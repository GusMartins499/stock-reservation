# stock-reservation

Inventory reservation service built for flash-sale traffic: thousands of buyers competing for a few hundred units, with zero overselling.

## The problem

During events like Black Friday, a single product can receive tens of thousands of purchase attempts per second for a limited stock. The service must:

- never sell more units than exist;
- keep latency low under heavy contention;
- release reservations from abandoned checkouts;
- tolerate client retries without reserving twice.

## Status

Work in progress — see [docs/ROADMAP.md](docs/ROADMAP.md).

## Phase 1

Concurrent in-memory core: HTTP API, a test that proves overselling under concurrent reservations, and the `sync.Mutex` fix. Notes: [English](docs/notes/phase-1.md) · [Português](docs/notes/phase-1.pt-BR.md).

## Phase 2

Stock in Postgres, shared by every instance of the service. A reservation is one conditional update, `UPDATE products SET stock = stock - $1 WHERE sku = $2 AND stock >= $1`: no lock held across round trips and no retries. A concurrency test runs 1000 buyers through two instances against 100 units and gets exactly 100 reservations.

## Phase 3

Reservations that expire. A reservation is `pending` until it is confirmed, cancelled or its `expires_at` passes; cancelling or expiring returns the units exactly once, and a confirmed reservation never expires. Every instance runs an expiry worker that returns stock in batches with `FOR UPDATE SKIP LOCKED`, and the service shuts down gracefully on `SIGTERM`. Notes: [English](docs/notes/phase-3.md) · [Português](docs/notes/phase-3.pt-BR.md).

## Phase 4

Idempotent reservations. `POST /reservations` requires an `Idempotency-Key` header: a retry with the same key gets the same reservation back, with `Idempotent-Replayed: true`, instead of taking stock again. The key is inserted in the same transaction as the stock decrement, so a crash at any point can never leave a reservation without its key. Concurrent retries wait on the primary key and replay. A test sends 50 concurrent retries and gets one reservation. Notes: [English](docs/notes/phase-4.md) · [Português](docs/notes/phase-4.pt-BR.md).

## Running

```bash
make db-up
make run
make test-race
```

`make run` connects to the Postgres from `docker-compose.yml`. Override `DATABASE_URL` or `ADDR` to point elsewhere, for example a second instance with `make run ADDR=:8081`.

| Variable | Default | |
|---|---|---|
| `DATABASE_URL` | required | Postgres connection string |
| `ADDR` | `:8080` | HTTP listen address |
| `RESERVATION_TTL` | `10m` | how long a reservation holds stock |
| `EXPIRY_INTERVAL` | `5s` | how often the worker looks for due reservations |
| `EXPIRY_BATCH_SIZE` | `100` | reservations expired, or keys deleted, per statement |
| `IDEMPOTENCY_KEY_TTL` | `24h` | how long an idempotency key is remembered |
| `IDEMPOTENCY_KEY_CLEANUP_INTERVAL` | `1m` | how often old keys are deleted |

To see a reservation expire, run `RESERVATION_TTL=30s make run`.

## Endpoints

```bash
curl -i -X POST localhost:8080/products -d '{"sku":"PS5","name":"PlayStation 5","stock":3}'
```

```bash
curl -i localhost:8080/products/PS5
```

```bash
curl -i -X POST localhost:8080/reservations -H "Idempotency-Key: $(uuidgen)" -d '{"sku":"PS5","quantity":2}'
```

Generate one key per purchase attempt and send the same key on every retry of it. A retry answers `201` with the same reservation and `Idempotent-Replayed: true`. The same key with a different body answers `422`, and a missing key answers `400`.

The response carries the reservation `id`, its `status` and `expires_at`. Confirm or cancel it with that id:

```bash
curl -i -X POST localhost:8080/reservations/<id>/confirm
```

```bash
curl -i -X POST localhost:8080/reservations/<id>/cancel
```

Confirming or cancelling a reservation that is no longer pending, or confirming one that has expired, answers `409`. An unknown id answers `404`.

```bash
curl -i localhost:8080/products/XBOX
```

## Mutation testing

Coverage shows which lines the tests run; mutation testing shows whether the tests catch bugs. [Gremlins](https://github.com/go-gremlins/gremlins) introduces small changes (mutants) into the code, such as flipping `<` to `<=`, and runs the tests against each one. A mutant is *killed* when a test fails and *lived* when every test still passes.

Efficacy is `killed / (killed + lived)`. The build fails when efficacy is not above the 80% threshold set in [.gremlins.yaml](.gremlins.yaml). In CI, only lines changed by the pull request are mutated, and the check is skipped when those lines have no mutants.

```bash
make mutation
```
