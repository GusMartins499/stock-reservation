# Phase 4 — Idempotency

Goal: a client that sends `POST /reservations` again gets the reservation it already has, not a second one. Retries are normal: a mobile client loses its connection, a load balancer times out at 5 s while the database answers at 6 s, a queue delivers a message twice. In each case the client cannot know whether the first request took the stock, so the only safe move is to send it again. Without idempotency, every retry during a flash sale takes units for a reservation nobody will pay for.

The design follows what Stripe has run in production for years (the `Idempotency-Key` header, keys kept for 24 hours) and the IETF draft that standardises it, [`draft-ietf-httpapi-idempotency-key-header`](https://datatracker.ietf.org/doc/draft-ietf-httpapi-idempotency-key-header/). Where this phase does something different from them, it says so.

## Rules

`POST /reservations` requires an `Idempotency-Key` header. The client generates it, usually a UUID v4, once per purchase attempt, and sends the same value on every retry of that attempt.

| Request | Answer |
|---|---|
| no `Idempotency-Key`, or empty, or longer than 255 characters | `400` |
| new key | the reservation runs as today: `201`, or `404` / `409` / `422` |
| key already used, same `sku` and `quantity` | `201` with that reservation and `Idempotent-Replayed: true` |
| key already used, different `sku` or `quantity` | `422` |
| key in use by a request that has not finished | waits for it, then answers as one of the two rows above |

A key is stored **only when the reservation succeeds**, and **in the same transaction** as the stock decrement. Two consequences:

- A retry can never take stock twice, even if the instance dies between the reservation and the answer: either the transaction committed and the key points to the reservation, or nothing happened and the retry starts from scratch.
- A request that failed (`409 insufficient stock`, for example) leaves no key, so its retry is evaluated again and can succeed if stock came back. That is safe: the first attempt took nothing. Stripe does it differently and replays errors too, because its requests charge money and the client must see the original outcome. Here the property that matters is *at most one reservation per key*, and it holds either way.

Replays return the reservation **as it is now**, so a retry after a confirm shows `confirmed`. Stripe returns the stored original response byte for byte instead. Storing the response body would mean the database layer keeping HTTP responses; this service only needs the reservation, and its current state is more useful to a client that is retrying.

Confirm and cancel stay as they are: the `WHERE status = 'pending'` from Phase 3 already makes a repeated confirm harmless, it just answers `409` the second time.

## Expected layout

```
db/schema.sql                  ← idempotency_keys table
internal/inventory/            ← ErrIdempotencyKeyReused
internal/postgres/
    idempotency.go             ← ReserveOnce: key + reservation in one transaction
    idempotency_test.go
internal/httpapi/
    idempotency.go             ← middleware that reads and validates the header
    idempotency_test.go
cmd/api/main.go                ← a second worker that deletes old keys
```

---

## Day 1 — Contract, schema and domain

1. In `db/schema.sql`, an `idempotency_keys` table: `key` (`TEXT PRIMARY KEY`), `reservation_id` (`uuid REFERENCES reservations (id)`, nullable for the moment between the key insert and the reservation, never null once committed), `created_at` (`timestamptz NOT NULL DEFAULT now()`).
2. An index on `created_at`, for the cleanup on Day 4.
3. In `inventory`: `ErrIdempotencyKeyReused`.
4. Decide the signature of the new store method. It now takes five values (key, SKU, quantity, TTL and the context) and returns three (the reservation, whether it was a replay, an error). A request struct, `inventory.ReservationRequest{Key, SKU, Quantity, TTL}`, keeps call sites readable and lets a field be added later without touching every caller. This is the same reason `http.Request` and `pgx.ConnConfig` are structs.

No `sku`/`quantity` columns on the key: the reservation it points to already has them, and comparing with them is how a reused key is detected.

## Day 2 — Reserve once per key

`ReserveOnce(ctx, req) (inventory.Reservation, bool, error)` in `internal/postgres/idempotency.go`, in **one transaction** (`pgx.BeginFunc`):

1. `INSERT INTO idempotency_keys (key) VALUES ($1) ON CONFLICT (key) DO NOTHING RETURNING key`.
2. No row returned: the key exists. Read its reservation. Different `sku` or `quantity` → `ErrIdempotencyKeyReused`. Same → return it with `replayed = true`.
3. Row returned: the key is new. Run the reservation CTE from Phase 3 inside the same transaction, then `UPDATE idempotency_keys SET reservation_id = ...`. Any error rolls back the key with the reservation.

Step 1 is what makes concurrent retries safe. When two requests insert the same key, Postgres makes the second one wait on the primary key until the first commits or rolls back. After a commit, the second sees the key and replays. After a rollback, its insert goes through and it reserves on its own. There is no lock in Go, and no `SELECT` followed by an `INSERT`.

Integration tests:

- a new key reserves and decrements stock; the same key again returns the same reservation id, `replayed = true`, and the stock is unchanged;
- same key with a different `quantity`, and with a different `sku`: `ErrIdempotencyKeyReused`, stock unchanged;
- a reservation that fails with `ErrInsufficientStock` leaves no row in `idempotency_keys`;
- the last unit: stock 1, reserve with a key, then the same key again returns the reservation, not `ErrInsufficientStock`;
- 50 goroutines on two pools send the same key at once: exactly one reservation row, the stock went down once, and every goroutine got the same id.

Question to think about: why insert the key *before* the reservation? Try the other order on paper with the "last unit" case: two requests with the same key, stock 1.

## Day 3 — The middleware and the route

1. `requireIdempotencyKey(next http.Handler) http.Handler` in `internal/httpapi/idempotency.go`: reads `Idempotency-Key`, answers `400` if it is missing, blank or over 255 characters, and otherwise puts it in the request context under an unexported key type. This is how `chi/middleware.RequestID` and the OpenTelemetry HTTP middleware pass values to handlers.
2. Wrap only `POST /reservations` with it in `Routes()`.
3. The handler builds the `ReservationRequest`, calls `ReserveOnce`, and on a replay sets `Idempotent-Replayed: true` before writing `201`. `ErrIdempotencyKeyReused` maps to `422`.
4. The `Store` interface and the fake store change with it.

Unit tests with the fake store: missing, blank and too-long key (`400`, and the store is never called); a new key (`201`, no replay header); a replay (`201` with the header); a reused key (`422`); the key the handler passed to the store is the one in the header.

## Day 4 — Keeping keys for 24 hours, and close

1. `DeleteExpiredKeys(ctx, limit) (int, error)`: deletes up to `limit` keys older than `IDEMPOTENCY_KEY_TTL` (default `24h`, the window Stripe documents), with the same `FOR UPDATE SKIP LOCKED` batch shape as `ExpireDue`. A key older than that is free to be used again.
2. In `main.go`, a second `expiry.Worker` running it. Its interval can be long (`1m`): nothing waits on it. The worker's log line says "expired reservations"; it needs to say what each worker actually removed.
3. Two instances: send `POST /reservations` with the same key to each, and see one reservation, one decrement and `Idempotent-Replayed: true` on the second.
4. Phase 4 notes in `docs/notes/`: why the key lives in the same transaction as the stock, what the second concurrent request waits on, and the two places this phase differs from Stripe (failed requests leave no key, replays return the current state).
5. README section for Phase 4, and the `curl` example for `POST /reservations` with the header.

`make check` must pass, and CI must be green.
