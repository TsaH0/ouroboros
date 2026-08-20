.PHONY: fmt test test-race build run

fmt:
	go fmt ./...

test:
	go test ./...

test-race:
	go test -race ./...

build:
	go build -o bin/ouroboros ./cmd/ouroboros

run:
	go run ./cmd/ouroboros
