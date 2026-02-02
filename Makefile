.PHONY: all build test lint vuln clean release

BINARY := coop
GO := go
GOFLAGS := -trimpath -ldflags="-s -w"

# Default: lint, test, build
all: lint test build

# Build the binary
build:
	$(GO) build $(GOFLAGS) -o $(BINARY) ./cmd/coop

# Run tests with race detector
test:
	$(GO) test -race -cover ./...

# Lint with staticcheck
lint:
	@command -v staticcheck >/dev/null 2>&1 || { echo "Install: brew install staticcheck"; exit 1; }
	staticcheck ./...

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
