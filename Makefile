-include .env
export

BIN := bin/server

.PHONY: build run clean

build:
	go build -o $(BIN) ./cmd/server

run: build
	./$(BIN)

clean:
	rm -rf $(BIN)
