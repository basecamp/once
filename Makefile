.PHONY: build test integration lint lint-actions coverage dist test-release

PLATFORMS = linux darwin
ARCHITECTURES = amd64 arm64
VERSION := $(shell git describe --tags --always)
LDFLAGS := -ldflags "-s -w -X 'github.com/basecamp/once/internal/version.Version=$(VERSION)'"

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
	cd installer && go test ./...

integration:
	go test -v -count=1 -parallel 16 -timeout 30m ./integration/...

check:
	go vet ./...
	govulncheck ./...

lint:
	golangci-lint run

lint-actions:
	actionlint
	zizmor .

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
	cd dist && { \
		echo "once $(VERSION)"; \
		echo "commit $$(git rev-parse HEAD)"; \
		sha256sum once-*; \
	} > release.txt

# Runs the real release workflow on main as a draft: signed like any release,
# but never published, never tagged, and never "latest". Signing waits for a
# reviewer to approve the release environment, as it does for every release.
test-release:
	-gh release delete $(TEST_RELEASE_TAG) --yes --cleanup-tag
	gh workflow run release.yml --ref main -f version=$(TEST_RELEASE_TAG) -f draft=true
