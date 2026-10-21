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

## Running

```bash
make run
make test-race
```
