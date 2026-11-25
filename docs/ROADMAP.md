# Roadmap

Go fundamentals (types, structs, interfaces, tests, goroutines, channels, mutex) are already covered in [go-lessons](https://github.com/GusMartins499/go-lessons) and [go-exercises](https://github.com/GusMartins499/go-exercises).

| Phase | Duration | Deliverable | Go concepts |
|---|---|---|---|
| 1. Concurrent core | ~1 week | In-memory HTTP API + a test proving overselling with 1000 goroutines, then the fix | `net/http`, `httptest`, sentinel errors, `go test -race` |
| 2. Postgres | ~1 week | Persisted stock shared by many instances, docker-compose | `pgx`, atomic conditional updates, lost updates |
| 3. Expiring reservations | ~1 week | TTL, confirm/cancel, worker that returns stock | `context`, `time.Ticker`, channels, graceful shutdown |
| 4. Idempotency | ~4 days | `Idempotency-Key` header | HTTP middlewares, API design |
| 5. Redis on the hot path | ~1 week | Atomic decrement with Lua, Postgres as source of truth | external integration, consistency trade-offs |
| 6. Load testing | ~1 week | k6, Prometheus metrics, README with numbers | observability, `testing.B` |
| 7. Events (optional) | — | Outbox + Kafka/NATS | eventual consistency |

## Progress

- [x] Phase 1 — [tasks](tasks/phase-1.md)
- [x] Phase 2 — [tasks](tasks/phase-2.md)
- [x] Phase 3 — [tasks](tasks/phase-3.md)
- [ ] Phase 4 — [tasks](tasks/phase-4.md)
- [ ] Phase 5
- [ ] Phase 6
