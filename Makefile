.PHONY: fmt test test-race run

fmt:
	go fmt ./...

test:
	go test ./...

test-race:
	go test -race ./...

run:
	go run ./cmd/sentinel
