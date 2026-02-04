# Contributing to coop

## Quick Start

```bash
# Clone the repo
git clone https://github.com/stuffbucket/coop.git
cd coop

# Set up development environment (installs hooks + tools)
make setup

# Build and test
make all
```

## Development Workflow

### Building

```bash
make build      # Build binary to ./coop
make install    # Install to $GOPATH/bin
```

### Testing

```bash
make test       # Full test suite with race detector
make test-short # Quick tests only (for iteration)
make coverage   # Generate HTML coverage report
```

### Linting

```bash
make lint       # Run golangci-lint
make fmt        # Format code with gofmt
make vuln       # Check for known vulnerabilities
```

## Git Hooks

We use versioned git hooks in `.githooks/`. The `make setup` command configures git to use them automatically:

```bash
git config core.hooksPath .githooks
```

### Hooks included:

- **commit-msg**: Enforces conventional commit format
- **pre-push**: Runs quick lint/build/test before pushing

### Commit Message Format

```
type(scope)?: description (50 chars max)

[optional body]

[optional footer]
```

Types: `feat`, `fix`, `refactor`, `test`, `build`, `chore`, `docs`, `perf`, `ci`

Examples:
```
feat(ui): add theme picker
fix: prevent path traversal in socket discovery
refactor(backend): rename vm package to backend
```

## Project Structure

```
cmd/coop/           # CLI entry point
internal/
  backend/          # VM backends (Colima, Lima)
  cloudinit/        # Cloud-init user data generation
  config/           # Configuration management
  doctor/           # Diagnostic commands
  incus/            # Incus container client
  logging/          # Structured logging
  names/            # Container name generator
  platform/         # Platform detection
  sandbox/          # Security sandboxing (macOS seatbelt)
  sshkeys/          # SSH key management
  state/            # Container state tracking
  ui/               # Terminal UI components
scripts/            # Build and release scripts
templates/          # Configuration templates
```

## Pull Requests

1. Create a feature branch from `main`
2. Make your changes with tests
3. Ensure `make all` passes
4. Open a PR with a clear description

CI will automatically:
- Run tests with race detector
- Lint with golangci-lint
- Scan for vulnerabilities (govulncheck + trivy)

## Release Process

Releases are automated via GitHub Actions when a tag is pushed:

```bash
make release TAG=v0.2.0
```

This creates and pushes the tag. GitHub Actions will:
1. Build binaries for all platforms
2. Create GitHub release with changelog
3. Update Homebrew tap formula
