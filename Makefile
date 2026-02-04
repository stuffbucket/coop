.PHONY: all build test lint vuln clean release install fmt coverage help setup

BINARY := coop
GO := go
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
GOFLAGS := -trimpath -ldflags="$(LDFLAGS)"

# Default: show help
.DEFAULT_GOAL := help

# Full build pipeline: lint, test, build
all: lint test build

# First-time setup for contributors
setup:
	@echo "Setting up development environment..."
	git config core.hooksPath .githooks
	@command -v golangci-lint >/dev/null 2>&1 || { echo "Installing golangci-lint..."; brew install golangci-lint; }
	@command -v govulncheck >/dev/null 2>&1 || { echo "Installing govulncheck..."; go install golang.org/x/vuln/cmd/govulncheck@latest; }
	@echo "✓ Setup complete"

# Build the binary
build:
	$(GO) build $(GOFLAGS) -o $(BINARY) ./cmd/coop

# Install to GOPATH/bin
install:
	$(GO) install $(GOFLAGS) ./cmd/coop

# Run tests with race detector
test:
	$(GO) test -race -cover ./...

# Run short tests only (for quick iteration)
test-short:
	$(GO) test -short ./...

# Lint with golangci-lint (matches CI)
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "Install: brew install golangci-lint"; exit 1; }
	golangci-lint run

# Format code with gofmt
fmt:
	$(GO) fmt ./...

# Generate coverage report
coverage:
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Vulnerability scan (govulncheck)
vuln:
	@command -v govulncheck >/dev/null 2>&1 || { echo "Install: go install golang.org/x/vuln/cmd/govulncheck@latest"; exit 1; }
	govulncheck ./...

# Clean build artifacts
clean:
	rm -f $(BINARY) coverage.out coverage.html

# Create a release tag (GitHub Actions handles the rest)
# Usage: make release TAG=v0.2.0
release:
	@test -n "$(TAG)" || { echo "Usage: make release TAG=v0.2.0"; exit 1; }
	git tag -a $(TAG) -m "Release $(TAG)"
	git push stuffbucket $(TAG)
	@echo "Tagged $(TAG) - GitHub Actions will build and publish."

# Show available targets
help:
	@echo "coop development commands:"
	@echo ""
	@echo "  make setup      - First-time dev environment setup"
	@echo "  make build      - Build the binary"
	@echo "  make install    - Install to GOPATH/bin"
	@echo "  make test       - Run all tests with race detector"
	@echo "  make test-short - Run short tests only (fast)"
	@echo "  make lint       - Run golangci-lint"
	@echo "  make fmt        - Format code with gofmt"
	@echo "  make coverage   - Generate HTML coverage report"
	@echo "  make vuln       - Run vulnerability scan"
	@echo "  make clean      - Remove build artifacts"
	@echo "  make all        - Run lint, test, build"
	@echo ""
	@echo "  make release TAG=v0.2.0 - Create and push release tag"
