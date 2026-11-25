# Phase 3 notes

[Versão em português](phase-3.pt-BR.md)

A reservation holds stock for a limited time. The buyer confirms it after paying or cancels it. If neither happens before `expires_at`, the units go back on sale. Without this, every abandoned checkout keeps units off the shelf for good, and a flash sale sells out to people who never paid.

## The state machine

| From | Event | To | Stock |
|---|---|---|---|
| — | reserve | `pending` | decremented |
| `pending` | confirm before `expires_at` | `confirmed` | unchanged |
| `pending` | cancel | `cancelled` | returned |
| `pending` | `expires_at` passes | `expired` | returned |

Every other transition is refused with `409`. A confirmed reservation keeps its units: `confirmed` means sold, so the stock was already right when the reservation was made.

Each transition is one SQL statement whose `WHERE status = 'pending'` is the guard. When two requests race on the same reservation, Postgres locks the row for the first `UPDATE`; the second one waits, re-checks the `WHERE` against the committed row, finds it no longer pending and changes nothing. There is no lock in Go and no read-then-write in the application. The tests run 50 concurrent cancels on one reservation (exactly one succeeds and the stock comes back once) and confirm racing cancel twenty times (always exactly one winner).

Reserve and cancel touch two tables, so each one is a data-modifying CTE: the stock update and the reservation change commit together or not at all, in one round trip.

## Why the database clock

Confirm checks `expires_at > now()` and the expirer checks `expires_at <= now()`, both in SQL. With the clock of each instance, two instances a few seconds apart could disagree: one accepts a confirmation while the other has already returned the units. With `now()` there is one clock, and confirm and expire can never both win. A test fires a confirm and `ExpireDue` on the same reservation from two pools, around its `expires_at`, twenty times: either the confirm wins and nothing expires, or the confirm is refused and the units come back exactly once.

## The expirer

`ExpireDue` takes one batch of due reservations, marks them `expired` and returns their units, in one statement:

1. `FOR UPDATE SKIP LOCKED` picks up to `limit` due rows. Every instance runs the expirer; `SKIP LOCKED` makes each one take rows the others have not locked instead of waiting for them, so they split the work instead of queuing behind each other. In the two-instance run, each expired reservation shows up in the log of only one instance.
2. The batch limit keeps every transaction short, even after an outage leaves thousands of reservations due. The worker calls it again right away while full batches come back, then waits for the next tick.
3. The returned quantities are summed per SKU and the product rows are locked in SKU order before the update.

The last point came from a test. Eight expirers running at once over five SKUs deadlocked: one batch held `PS5` and waited for `XBOX`, another held `XBOX` and waited for `PS5`. Across 30 runs Postgres reported 192 `deadlock detected`. Locking products with `ORDER BY sku FOR UPDATE` makes every transaction take locks in the same order, and the count went to zero.

## The worker and shutdown

The worker calls `ExpireDue` at startup, to catch up after downtime, and then on every `EXPIRY_INTERVAL`. A failed batch is logged and tried again on the next tick; it does not kill the process. Its tests use `testing/synctest`, so a ticker of minutes runs instantly and without timing flakiness.

On `SIGINT` or `SIGTERM`, `http.Server.Shutdown` lets in-flight requests finish, the worker stops, and only then is the pool closed. Closing the pool first would make those requests and the running batch fail halfway. A batch that is cut off is not a problem: its transaction rolls back and the rows are still due for the next instance.
