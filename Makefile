.PHONY: build test lint vuln vuln-go clean all check go-version ci ci-all act act-build act-lint push hooks

# Build settings
BINARY := coop
GO := go
GOFLAGS := -trimpath -ldflags="-s -w"

# Detect installed Go version (e.g., "1.25.6")
GO_VERSION := $(shell $(GO) version | sed -E 's/go version go([0-9]+\.[0-9]+(\.[0-9]+)?).*/\1/')

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

# Show detected Go version
go-version:
	@echo "Installed: go$(GO_VERSION)"
	@echo "go.mod:    $$(grep '^go ' go.mod | awk '{print $$2}')"

# Sync go.mod version to match installed Go
go-upgrade:
	@echo "Upgrading go.mod from $$(grep '^go ' go.mod | awk '{print $$2}') to $(GO_VERSION)"
	@sed -i '' 's/^go [0-9][0-9.]*$$/go $(GO_VERSION)/' go.mod
	$(GO) mod tidy
	@echo "Done. Run 'make test' to verify."

# Run CI checks via act (GitHub Actions local runner)
# Install: brew install act
# Supports git worktrees by mounting parent .git directory

# Detect git worktree and get parent .git path for container mount
ACT_WORKTREE_OPTS := $(shell \
	if [ -f .git ]; then \
		gitdir=$$(sed -n 's/^gitdir: //p' .git); \
		parent_git=$$(echo "$$gitdir" | sed 's|/worktrees/.*||'); \
		echo "--container-options \"-v $$parent_git:$$parent_git:ro\""; \
	fi)

# CI target - runs build job only (fast, always works)
ci:
	@command -v act >/dev/null 2>&1 || { echo "Install act: brew install act"; exit 1; }
	act push -j build $(ACT_WORKTREE_OPTS)

# Full CI - runs all jobs (lint/security are informational)
ci-all: act

act:
	@command -v act >/dev/null 2>&1 || { echo "Install act: brew install act"; exit 1; }
	act push $(ACT_WORKTREE_OPTS)

# Run only the build job
act-build:
	@command -v act >/dev/null 2>&1 || { echo "Install act: brew install act"; exit 1; }
	act push -j build $(ACT_WORKTREE_OPTS)

# Run only the lint job
act-lint:
	@command -v act >/dev/null 2>&1 || { echo "Install act: brew install act"; exit 1; }
	act push -j lint $(ACT_WORKTREE_OPTS)

# Push with CI verification (safe push)
push: act
	git push origin $$(git branch --show-current)

# Install git hooks
hooks:
	@mkdir -p .git/hooks
	@echo '#!/bin/sh' > .git/hooks/pre-push
	@echo 'make act' >> .git/hooks/pre-push
	@chmod +x .git/hooks/pre-push
	@echo "Installed pre-push hook"
