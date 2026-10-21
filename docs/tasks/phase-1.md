# Phase 1 — Concurrent core

Goal: an in-memory HTTP service that registers products and reserves units, plus a test that **proves** it sells more than it has when many buyers hit it at the same time. Then, the fix.

## Expected layout

```
cmd/api/main.go              ← starts the HTTP server
internal/inventory/          ← business rules (knows nothing about HTTP)
    inventory.go
    inventory_test.go
internal/httpapi/            ← handlers: translate HTTP ↔ inventory
    handlers.go
    handlers_test.go
```

Why `internal/`? Packages under `internal` cannot be imported by other modules. It is the idiomatic way to say "this is an implementation detail of this service".

---

## Day 1 — Domain and tests

In `internal/inventory/inventory.go`:

1. `Product` with `SKU string`, `Name string`, `Stock int`.
2. `Store` holding a `map[string]*Product` (key = SKU) and `NewStore() *Store`. **No mutex for now** — on purpose.
3. Sentinel errors with `errors.New`: `ErrProductNotFound`, `ErrProductAlreadyExists`, `ErrInsufficientStock`, `ErrInvalidQuantity`.
4. Methods:
   - `AddProduct(p Product) error` — fails if the SKU already exists or the stock is negative.
   - `GetProduct(sku string) (Product, error)` — returns a **copy**, not a pointer. Think about why.
   - `Reserve(sku string, qty int) error` — fails if `qty <= 0`, the product does not exist, or there is not enough stock.

Table-driven tests for `Reserve` and `AddProduct`, comparing errors with [`errors.Is`](https://pkg.go.dev/errors#Is). Cases: regular reservation, reserving the whole stock, quantity above stock, zero, negative, unknown SKU, duplicate SKU, negative stock.

## Day 2 — HTTP

In `internal/httpapi/handlers.go`, a `Handler` type that receives `*inventory.Store` and a `Routes() http.Handler` method using `http.NewServeMux()` with Go 1.22+ patterns:

- `POST /products` — `{"sku": "...", "name": "...", "stock": 10}` → `201`
- `GET /products/{sku}` — `r.PathValue("sku")` → `200`
- `POST /reservations` — `{"sku": "...", "quantity": 2}` → `201`

In `cmd/api/main.go`: create the store and the handler, call `http.ListenAndServe(":8080", ...)`.

Reading: [Routing enhancements in Go 1.22](https://go.dev/blog/routing-enhancements).

## Day 3 — HTTP errors and handler tests

A function that maps domain errors to status codes, always responding with JSON (`{"error": "insufficient stock"}`):

| Error | Status |
|---|---|
| `ErrProductNotFound` | 404 |
| `ErrProductAlreadyExists` | 409 |
| `ErrInsufficientStock` | 409 |
| `ErrInvalidQuantity` | 422 |
| Invalid JSON | 400 |
| anything else | 500 |

In `handlers_test.go`, test with [`httptest.NewRecorder`](https://pkg.go.dev/net/http/httptest) — no real server. At least: create a product, fetch an unknown one (404), reserve successfully, reserve without stock (409).

Manual test:

```bash
curl -i -X POST localhost:8080/products -d '{"sku":"PS5","name":"PlayStation 5","stock":3}'
curl -i -X POST localhost:8080/reservations -d '{"sku":"PS5","quantity":2}'
curl -i -X POST localhost:8080/reservations -d '{"sku":"PS5","quantity":2}'
curl -i localhost:8080/products/PS5
```

## Day 4 — Prove the bug

In `inventory_test.go`, a test named something like `TestReserve_ConcurrentBuyersNeverOversell`:

1. Product with stock 100.
2. 1000 goroutines, each trying `Reserve(sku, 1)`. Use `sync.WaitGroup` to wait for all of them.
3. Count how many reservations succeeded (careful: the counter is shared too — `sync/atomic` solves it).
4. Assert: successes == 100 and final stock == 0.

Run `go test ./...` several times (`-count=20`). Then run `make test-race`. Write down what each one shows — it goes into the README.

Question to think about: why does the test sometimes **pass** even with the bug? What does the race detector see that the regular test does not?

## Day 5 — Fix and understand

1. Protect the `Store` with `sync.Mutex`. The Day 4 test must pass with `-race -count=50`.
2. Try `sync.RWMutex`: which methods make sense with `RLock`? Does it gain anything in `Reserve`?
3. Write a `BenchmarkReserve` benchmark with `b.RunParallel` and compare both versions (`go test -bench=. -benchmem ./internal/inventory`).
4. Answer in one paragraph, in a "Phase 1" section of the README: what the mutex solves and why it does **not** solve the problem once two instances of the service are running. (Spoiler: that is why Phase 2 exists.)

`make check` must pass. Done? Call me for the phase review.
