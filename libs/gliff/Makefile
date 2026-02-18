.PHONY: build test bench lint

build:
	CGO_ENABLED=0 go build -trimpath -o bin/ ./cmd/...

test:
	go test ./...

bench:
	go test -bench=. -benchmem ./...

lint:
	golangci-lint run

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out
