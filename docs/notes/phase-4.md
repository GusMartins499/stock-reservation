# Phase 4 notes

[Versão em português](phase-4.pt-BR.md)

Clients retry. A load balancer gives up after 5 s while the database answers at 6 s, a phone drops off the network, a queue delivers a message twice. The client can't tell whether the first request took the stock, so the only safe thing it can do is send the request again. Without idempotency, every retry during a flash sale holds units for a reservation nobody will pay for.

`POST /reservations` now requires an `Idempotency-Key` header, following the Stripe model and [`draft-ietf-httpapi-idempotency-key-header`](https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/).

| Request | Answer |
|---|---|
| no key, blank, or over 255 characters | `400` |
| new key | `201`, or the usual `404` / `409` / `422` |
| known key, same `sku` and `quantity` | `201`, same reservation, `Idempotent-Replayed: true` |
| known key, different `sku` or `quantity` | `422` |

## The key commits with the stock

`ReserveOnce` runs as one transaction:

1. `INSERT INTO idempotency_keys (key) ... ON CONFLICT DO NOTHING`.
2. If the key already exists, read its reservation and replay it, or refuse with `422` if the body differs.
3. If the key is new, run the reservation CTE from Phase 3 and point the key at the new reservation.

The key and the stock decrement commit together or not at all. If the instance dies after the reservation but before the answer, the key is already there, so the retry gets the reservation back. If the instance dies before the commit, there is no reservation and no key, and the retry starts over. A key in a separate store such as Redis, written after the reservation, would leave a gap between the two writes where a retry reserves a second time.

## What a concurrent retry waits on

Two requests that insert the same key at the same moment both reach the primary key index. Postgres makes the second one wait for the first transaction to finish:

- if the first one commits, the second one's `ON CONFLICT DO NOTHING` inserts nothing, and it replays the reservation;
- if the first one rolls back, for example because stock ran out, the second one's insert succeeds and it reserves on its own.

There is no lock in Go, and no `SELECT` followed by an `INSERT`. The test sends the same key 50 times at once through two pools and gets one reservation, one decrement and 49 replays.

The key is inserted **before** the reservation because of the last unit. Suppose stock is 1 and two requests carry the same key, but the reservation runs first and the key is written after it. The first request takes the unit. The second one waits on the product row, then finds stock 0 and answers `409`, even though its reservation exists. A test covers this case.

## Where this differs from Stripe

- **A failed request leaves no key.** A `409 insufficient stock` rolls back the key together with everything else, so a retry is evaluated again and can succeed if stock came back. This is safe because the first attempt took nothing. Stripe replays errors as well, since a request that charged money has to show the client its original outcome. The property this service needs is *at most one reservation per key*, and it holds either way.
- **A replay returns the reservation as it is now.** If the client confirmed in the meantime, the replay shows `confirmed`. Stripe stores the original response and replays it byte for byte. Doing that here would mean the database layer storing HTTP bodies, and a client that is retrying is better served by the current state.

## Keeping keys for 24 hours

Keys live for `IDEMPOTENCY_KEY_TTL`, which defaults to `24h`, the window Stripe documents. After that a second worker deletes them in batches, with the same `FOR UPDATE SKIP LOCKED` pattern as the expirer, so every instance can run it. Both workers now log with a `worker` field, so a log line says which job produced it.
