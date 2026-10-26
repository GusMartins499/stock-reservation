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

## Endpoints

```bash
curl -i -X POST localhost:8080/products -d '{"sku":"PS5","name":"PlayStation 5","stock":3}'
```

```bash
curl -i localhost:8080/products/PS5
```

```bash
curl -i -X POST localhost:8080/reservations -d '{"sku":"PS5","quantity":2}'
```

```bash
curl -i localhost:8080/products/XBOX
```

## Mutation testing

Coverage shows which lines the tests run; mutation testing shows whether the tests catch bugs. [Gremlins](https://github.com/go-gremlins/gremlins) introduces small changes (mutants) into the code, such as flipping `<` to `<=`, and runs the tests against each one. A mutant is *killed* when a test fails and *lived* when every test still passes.

Efficacy is `killed / (killed + lived)`. The build fails when efficacy is not above the 80% threshold set in [.gremlins.yaml](.gremlins.yaml). In CI, only lines changed by the pull request are mutated, and the check is skipped when those lines have no mutants.

```bash
make mutation
```
