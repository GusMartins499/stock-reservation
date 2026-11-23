# Phase 2 — Postgres

Goal: move the stock out of each process and into one shared place, so that two instances of the service cannot oversell. The rule from Phase 1 still holds: a concurrency test with many buyers on one SKU is the proof that the stock never goes past what exists. This time the buyers are spread across two instances.

## Expected layout

```
cmd/api/main.go              ← connects to Postgres and starts the HTTP server
db/schema.sql                ← table definitions
docker-compose.yml           ← local Postgres
internal/inventory/          ← domain and the in-memory store
internal/postgres/           ← the same operations, in SQL
    store.go
    store_test.go
internal/httpapi/            ← depends on an interface, not on a concrete store
```

Why a separate `postgres` package? The same reason `inventory` knows nothing about HTTP: the domain stays free of SQL. `postgres` imports `inventory` to reuse `Product` and the sentinel errors, never the other way around.

---

## Day 1 — Postgres running

1. `docker-compose.yml` with one Postgres service, and `Makefile` targets `db-up`, `db-down` and `db-psql`.
2. In `db/schema.sql`, a `products` table: `sku` (primary key), `name`, `stock`. Add `CHECK (stock >= 0)`.
3. Mount the file in `/docker-entrypoint-initdb.d` so Postgres applies it on first start. It runs only on an empty volume, so a schema change needs `docker compose down -v`.

Then, in `make db-psql`, insert a product by hand and reproduce the problem with two terminals:

```sql
BEGIN;
SELECT stock FROM products WHERE sku = 'PS5';
UPDATE products SET stock = 0 WHERE sku = 'PS5';
COMMIT;
```

Run it in both terminals, with the second `SELECT` happening before the first `COMMIT`. Question to think about: both read the same stock, so what is the database actually protecting here, and what is it not?

Reading: [Transaction isolation](https://www.postgresql.org/docs/current/transaction-iso.html), up to and including *Read Committed*.

## Day 2 — An interface between HTTP and storage

No SQL today. Every test must stay green at the end.

1. Add `ctx context.Context` as the first parameter of `AddProduct`, `GetProduct` and `Reserve`. Handlers pass `r.Context()`. Database calls need it to stop working when the client disconnects. The in-memory store ignores it.
2. In `httpapi`, declare the interface that the API needs and make `API` hold it instead of `*inventory.Store`:

```go
type Store interface {
	AddProduct(ctx context.Context, p inventory.Product) error
	GetProduct(ctx context.Context, sku string) (inventory.Product, error)
	Reserve(ctx context.Context, sku string, qty int) error
}
```

Question to think about: why is the interface declared in `httpapi`, where it is used, and not in `inventory`? What did `*inventory.Store` have to change to satisfy it?

## Day 3 — `postgres.Store`, without `Reserve`

Reading: [`pgxpool`](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool) and [`pgconn.PgError`](https://pkg.go.dev/github.com/jackc/pgx/v5/pgconn#PgError).

In `internal/postgres/store.go`:

1. `Store` holding a `*pgxpool.Pool`, and `NewStore(pool *pgxpool.Pool) *Store`.
2. `AddProduct`: validates like the in-memory store, then `INSERT`. A duplicate SKU arrives as a `*pgconn.PgError` with code `23505`: use [`errors.As`](https://pkg.go.dev/errors#As) and return `inventory.ErrProductAlreadyExists`.
3. `GetProduct`: `SELECT`, and `pgx.ErrNoRows` becomes `inventory.ErrProductNotFound`.

Always pass values as `$1`, `$2`. Never build SQL by concatenating strings: that is SQL injection.

Question to think about: `AddProduct` no longer needs to check whether the SKU exists before inserting. Why is letting the database refuse better than `SELECT` followed by `INSERT`? (Hint: you have seen this pattern on Day 4 of Phase 1.)

## Day 4 — Integration tests

`internal/postgres/store_test.go`, table-driven, against the real database:

1. Read the connection string from `TEST_DATABASE_URL`. When it is empty, call `t.Skip`, so `go test ./...` keeps working without Docker.
2. Clean the table at the start of each case (`TRUNCATE products`). Same idea as a new store per case.
3. Same cases as the `inventory` tests for `AddProduct` and `GetProduct`, comparing errors with `errors.Is`.
4. In `.github/workflows/ci.yml`, a Postgres service so that these tests also run in CI.

Because every case uses the same database, these tests cannot use `t.Parallel`. Packages also run in parallel by default, so keep every database test in this one package.

## Day 5 — Two instances, one stock

1. Port `TestReserve_ConcurrentBuyersNeverOversell` to `internal/postgres`, with one difference: create **two pools and two stores** on the same database, and alternate the goroutines between them. That is two instances of the service. Split the test helper so that opening a pool and cleaning the table are separate steps.
2. Port the `TestReserve` table from `inventory`: the rules do not change with the storage.
3. Write `Reserve` as a single conditional update:

```sql
UPDATE products SET stock = stock - $1 WHERE sku = $2 AND stock >= $1
```

4. `RowsAffected() == 1` is a successful reservation. `0` means the SKU does not exist or the stock does not cover the quantity: only on that path, read the product to tell `ErrProductNotFound` from `ErrInsufficientStock`.

The concurrency test must pass with `-count=20 -race`.

Questions to think about:

- A `SELECT` followed by an `UPDATE` with the value computed in Go loses updates under concurrency. Why does `stock = stock - $1` not?
- Why is this better than `SELECT ... FOR UPDATE` inside a transaction, or than optimistic locking with a `version` column and retries? Think about round trips, how long the row stays locked, and what happens to retries when a thousand buyers want the same row.
- `-race` found the Phase 1 bug. Would it find a lost update in Postgres? Why?

## Day 6 — Serve from Postgres

1. `cmd/api/main.go` requires `DATABASE_URL` and listens on `ADDR` (default `:8080`). It pings the database before serving, so a database that is down fails at startup instead of on the first request.
2. `make run` points at the local Postgres from `docker-compose.yml`.
3. Run two instances (`make run` and `make run ADDR=:8081`), reserve on both with `curl`, and see the same stock. The reservation that goes past it gets `409`.

`make check` must pass, and CI must be green.
