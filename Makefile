.PHONY: build test lint vuln vuln-go vuln-strict clean all check

# Build settings
BINARY := coop
GO := go
GOFLAGS := -trimpath -ldflags="-s -w"

# Default target
all: check build

# Build the binary
build:
	$(GO) build $(GOFLAGS) -o $(BINARY) ./cmd/coop

# Run tests
test:
	$(GO) test -race -cover ./...

# Lint with staticcheck (install: brew install staticcheck)
lint:
	@command -v staticcheck >/dev/null 2>&1 || { echo "Install staticcheck: brew install staticcheck"; exit 1; }
	staticcheck ./...

# Go-specific vulnerability scan with call graph analysis (install: brew install govulncheck)
# This is more precise - only reports CVEs in code paths you actually use
vuln-go:
	@command -v govulncheck >/dev/null 2>&1 || { echo "Install govulncheck: brew install govulncheck"; exit 1; }
	govulncheck ./...

# Broad vulnerability scan (install: brew install trivy)
# Covers Go deps, containers, IaC, secrets, etc
vuln-trivy:
	@command -v trivy >/dev/null 2>&1 || { echo "Install trivy: brew install trivy"; exit 1; }
	trivy fs --scanners vuln --exit-code 0 .

# Combined vulnerability scan - run both tools
vuln: vuln-go vuln-trivy

# Strict vulnerability scan - fails on any CVE (for CI)
vuln-strict:
	@command -v govulncheck >/dev/null 2>&1 || { echo "Install govulncheck: brew install govulncheck"; exit 1; }
	@command -v trivy >/dev/null 2>&1 || { echo "Install trivy: brew install trivy"; exit 1; }
	govulncheck ./...
	trivy fs --scanners vuln --exit-code 1 --severity HIGH,CRITICAL .

# Update dependencies and tidy
deps:
	$(GO) get -u ./...
	$(GO) mod tidy

# Verify dependencies haven't been tampered with
verify:
	$(GO) mod verify

# Full security check: verify + scan
check: verify vuln

# Clean build artifacts
clean:
	rm -f $(BINARY)
	$(GO) clean -cache

# CI target: strict checks
ci: verify vuln-strict lint test build
