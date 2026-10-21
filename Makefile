.PHONY: run test test-race cover fmt vet check

run:
	go run ./cmd/api

test:
	go test ./...

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
