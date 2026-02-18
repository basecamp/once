.PHONY: build test integration lint coverage dist test-release

PLATFORMS = linux darwin
ARCHITECTURES = amd64 arm64
VERSION := $(shell git describe --tags --always)
LDFLAGS := -ldflags "-X 'github.com/basecamp/once/internal/version.Version=$(VERSION)'"

TEST_RELEASE_TAG = v0.0.1-test

build:
	CGO_ENABLED=0 go build -trimpath $(LDFLAGS) -o bin/ ./cmd/...

build-all:
	@for os in $(PLATFORMS); do \
		for arch in $(ARCHITECTURES); do \
			echo "Building for $$os/$$arch..."; \
			CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath $(LDFLAGS) -o bin/$$os-$$arch/ ./cmd/...; \
		done; \
	done

test:
	go test ./internal/...

integration:
	go test -v -count=1 ./integration/...

lint:
	golangci-lint run

coverage:
	go test -coverprofile=coverage.out ./internal/...
	go tool cover -html=coverage.out

dist: build-all
	mkdir -p dist
	@for os in $(PLATFORMS); do \
		for arch in $(ARCHITECTURES); do \
			cp bin/$$os-$$arch/once dist/once-$$os-$$arch; \
		done; \
	done
	cd dist && sha256sum once-* > checksums.txt

test-release:
	-gh release delete $(TEST_RELEASE_TAG) --yes --cleanup-tag
	git tag -f $(TEST_RELEASE_TAG)
	git push origin $(TEST_RELEASE_TAG) --force
