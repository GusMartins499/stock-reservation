.PHONY: run test test-race cover fmt vet check test-v mutation db-up db-down db-psql

run:
	go run ./cmd/api

test:
	go test ./...

test-v:
	go test ./... -v

test-race:
	go test -race ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

fmt:
	gofmt -s -w .

vet:
	go vet ./...

check: fmt vet test-race

mutation:
	gremlins unleash

db-up:
	docker compose up -d --wait

db-down:
	docker compose down

db-psql:
	docker compose exec postgres psql -U stock -d stock_reservation
