.PHONY: all build test lint vuln clean release

BINARY := coop
GO := go
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
GOFLAGS := -trimpath -ldflags="$(LDFLAGS)"

# Default: lint, test, build
all: lint test build

# Build the binary
build:
	$(GO) build $(GOFLAGS) -o $(BINARY) ./cmd/coop

# Run tests with race detector
test:
	$(GO) test -race -cover ./...

# Lint with golangci-lint (matches CI)
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "Install: brew install golangci-lint"; exit 1; }
	golangci-lint run

# Vulnerability scan (govulncheck)
vuln:
	@command -v govulncheck >/dev/null 2>&1 || { echo "Install: brew install govulncheck"; exit 1; }
	govulncheck ./...

# Clean build artifacts
clean:
	rm -f $(BINARY)

# Create a release tag (GitHub Actions handles the rest)
# Usage: make release VERSION=v0.2.0
release:
	@test -n "$(VERSION)" || { echo "Usage: make release VERSION=v0.2.0"; exit 1; }
	git tag -a $(VERSION) -m "Release $(VERSION)"
	git push stuffbucket $(VERSION)
	@echo "Tagged $(VERSION) - GitHub Actions will build and publish."
