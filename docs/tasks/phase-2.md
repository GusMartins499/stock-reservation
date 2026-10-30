# Phase 2 — Postgres

Goal: move the stock out of each process and into one shared place, so that two instances of the service cannot oversell. Same method as Phase 1: first a test that **proves** the bug, then the fix. This time the fix is a choice between two locking strategies, measured.

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

## Day 5 — Prove the bug again

1. Write `Reserve` the naive way: `SELECT` the stock, check it in Go, `UPDATE` with the new value. No transaction.
2. Port `TestReserve_ConcurrentBuyersNeverOversell` to `internal/postgres`, with one difference: create **two pools and two stores** on the same database, and alternate the goroutines between them. That is two instances of the service.
3. Run it several times and write down the results. It goes into the notes.

The test must be red. Commit it red, like in Phase 1: it is the history that tells the story.

Question to think about: `-race` found this bug in Phase 1. Will it find it now? Why?

## Day 6 — Fix 1: pessimistic locking

Reading: [Explicit locking](https://www.postgresql.org/docs/current/explicit-locking.html), the *Row-Level Locks* section.

Rewrite `Reserve` with a transaction:

1. `pool.Begin(ctx)`, and `defer tx.Rollback(ctx)` right after it. Same idea as `defer Unlock`: it covers every `return`. After a `Commit`, the rollback does nothing.
2. `SELECT stock FROM products WHERE sku = $1 FOR UPDATE`.
3. Check the stock, `UPDATE`, `tx.Commit(ctx)`.

The Day 5 test must pass with `-count=20`.

Question to think about: compare `FOR UPDATE` with the `sync.Mutex` from Phase 1. What is locked, where does the lock live, and why does it work across instances?

## Day 7 — Fix 2: optimistic locking

Reading: [Optimistic concurrency control](https://en.wikipedia.org/wiki/Optimistic_concurrency_control), the overview only.

1. Add a `version` column to `products`.
2. Write a second version of `Reserve`, with no `FOR UPDATE`: read `stock` and `version`, then `UPDATE ... WHERE sku = $1 AND version = $2`, incrementing `version`.
3. Use `RowsAffected()`: `0` means another buyer got there first. Read again and retry, with a limit. When the limit runs out, return a new `inventory.ErrConflict` and map it to a status in `writeError`.

Keep both versions available, for example as `Reserve` and `ReserveOptimistic`, so the benchmark can compare them. The same test must pass for both.

Question to think about: `UPDATE products SET stock = stock - $1 WHERE sku = $2 AND stock >= $1` does everything in one statement. Which of the two strategies is it closer to, and what does it give up?

## Day 8 — Measure and close

1. `BenchmarkReserve` for both versions, with `b.RunParallel`. Compare with low contention (many SKUs) and high contention (one SKU, the flash-sale case).
2. Choose the strategy that `Reserve` keeps, based on the numbers, and remove the other one.
3. `cmd/api/main.go` connects with `DATABASE_URL`. Run two instances (`8080` and `8081`), reserve on both with `curl`, and see the same stock.
4. Add to the Phase 2 notes, in `docs/notes/`: the results from Day 5, the benchmark table, the strategy you chose and why.

`make check` must pass, and CI must be green. Done? Call me for the phase review.
