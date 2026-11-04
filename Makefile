-include .env
export

BIN := bin/api
GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

.PHONY: build run test vet lint check clean

build:
	go build -o $(BIN) ./cmd/api

run: build
	./$(BIN)

test:
	go test -race ./...

vet:
	go vet ./...

lint:
	$(GOLANGCI_LINT) run

check: vet test lint

name ?= 
migration:
	goose -dir internal/store/migrations create $(name) sql

clean:
	rm -rf $(BIN)
