# Phase 1: Core Infrastructure PRD

**Product Requirements Document**  
**Timeline:** Week 1  
**Status:** Not Started

---

## Overview

Establish the foundational Go project and platform-agnostic Incus integration layer that all subsequent phases depend upon.

---

## Goals

1. Create a well-structured Go module with proper dependency management
2. Build an Incus client wrapper that abstracts platform-specific socket connections
3. Implement core container lifecycle operations (create, start, stop, delete)
4. Detect and adapt to host platform (macOS via Lima, WSL2/Ubuntu direct, native Linux)

---

## Impossible States (Invariants)

These conditions must never occur—design to make them unrepresentable:

| Invariant | Enforcement Strategy |
|-----------|---------------------|
| Container exists but no tracking record | Atomic create-then-register; rollback on failure |
| Container deleted but still in registry | Delete from registry first, then from Incus |
| Platform detected as both macOS and Linux | Exhaustive switch on detected platform enum |
| Incus client created without valid socket | Constructor returns error, no partial state |
| Container started before successful create | State machine enforcing `created → started` transition |

---

## Boundary Conditions (∂S ≠ ∅)

| Boundary | Expected Behavior |
|----------|------------------|
| Incus not installed | Graceful error: "Incus not found. See installation guide." |
| Socket path doesn't exist | Graceful error with platform-specific troubleshooting |
| Zero containers requested | No-op, return empty success |
| Max container limit reached | Propagate Incus error with context |
| Network unavailable during create | Timeout with retry guidance |
| Lima VM not running (macOS) | Detect and prompt: "Run `limactl start incus`" |

---

## Interface Anacoluthon Detection

Watch for these protocol discontinuities:

```go
// Canonical interface pattern
type ContainerLifecycle interface {
    Create(ctx context.Context, spec ContainerSpec) (*Container, error)
    Start(ctx context.Context, id string) error
    Stop(ctx context.Context, id string) error
    Delete(ctx context.Context, id string) error
}

// Anacoluthon flags to avoid:
// - Create() returning void but container exists (side effect leak)
// - Stop() silently succeeding on non-existent container
// - Delete() throwing exception vs returning error inconsistently
// - Platform detection changing behavior mid-session
```

---

## Functional Requirements

### FR-1: Project Structure

```
sandbox-manager/
├── cmd/
│   └── sandbox-manager/
│       └── main.go              # Entry point stub
├── internal/
│   ├── sandbox/
│   │   ├── manager.go           # SandboxManager interface
│   │   ├── container.go         # Container lifecycle
│   │   └── platform.go          # Platform detection
│   └── config/
│       └── config.go            # Configuration structures
├── pkg/
│   └── incus/
│       ├── client.go            # Incus client wrapper
│       └── platform.go          # Platform-specific socket resolution
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### FR-2: Platform Detection

```go
type Platform int

const (
    PlatformUnknown Platform = iota
    PlatformLinux           // Native Linux with Incus
    PlatformMacOS           // macOS with Lima + Incus
    PlatformWSL2            // WSL2 Ubuntu with Incus
)

func DetectPlatform() (Platform, error)
func GetIncusSocketPath(p Platform) (string, error)
```

**Detection Logic:**
1. Check `runtime.GOOS`
2. For macOS: verify Lima presence, check for Incus VM
3. For Linux: check `/etc/os-release` for WSL indicators
4. Validate Incus socket accessibility

### FR-3: Incus Client Wrapper

```go
type IncusClient struct {
    conn client.InstanceServer
    platform Platform
}

func NewIncusClient() (*IncusClient, error)
func (c *IncusClient) CreateContainer(ctx context.Context, req CreateRequest) error
func (c *IncusClient) StartContainer(ctx context.Context, name string) error
func (c *IncusClient) StopContainer(ctx context.Context, name string, timeout int) error
func (c *IncusClient) DeleteContainer(ctx context.Context, name string) error
func (c *IncusClient) GetContainerState(ctx context.Context, name string) (*ContainerState, error)
func (c *IncusClient) ListContainers(ctx context.Context, prefix string) ([]string, error)
```

### FR-4: Container Lifecycle

**State Machine:**
```
[None] --create--> [Created] --start--> [Running] --stop--> [Stopped] --delete--> [None]
                                              |                  ^
                                              +------------------+
                                                   (stop)
```

**Operations:**

| Operation | Precondition | Postcondition | Timeout |
|-----------|--------------|---------------|---------|
| Create | Container doesn't exist | Container in `Created` state | 60s |
| Start | Container in `Created` or `Stopped` | Container in `Running` | 30s |
| Stop | Container in `Running` | Container in `Stopped` | 30s (graceful) + 10s (force) |
| Delete | Container exists | Container removed | 30s |

---

## Non-Functional Requirements

### NFR-1: Performance
- Container create: < 5s (cached image)
- Container start: < 3s
- Platform detection: < 100ms

### NFR-2: Reliability
- All operations idempotent where semantically valid
- Graceful degradation with informative errors
- No orphaned resources on partial failure

### NFR-3: Observability
- Structured logging (slog) at debug/info/error levels
- Context propagation for tracing
- Operation timing metrics

---

## Acceptance Criteria

### AC-1: Platform Detection
- [ ] Correctly identifies macOS with Lima
- [ ] Correctly identifies native Linux
- [ ] Correctly identifies WSL2
- [ ] Returns clear error for unsupported platforms

### AC-2: Incus Connection
- [ ] Connects to Incus socket on all supported platforms
- [ ] Handles missing Incus installation gracefully
- [ ] Handles Lima VM not running (macOS)

### AC-3: Container Lifecycle
- [ ] Create container from `images:ubuntu/22.04`
- [ ] Start container successfully
- [ ] Stop container gracefully
- [ ] Delete container and confirm removal
- [ ] List containers with prefix filter

### AC-4: Error Handling
- [ ] All errors include operation context
- [ ] Platform-specific troubleshooting in error messages
- [ ] No panics on expected failure modes

---

## Technical Risks

| Risk | Impact | Mitigation |
|------|--------|-----------|
| Lima socket path varies by version | Connection failures on macOS | Probe multiple known paths |
| Incus API changes | Compile failures | Pin to specific lxc/incus version |
| WSL2 networking complexity | Container unreachable | Document required WSL2 network config |

---

## Dependencies

- `github.com/lxc/incus/v6/client` - Incus Go client
- `github.com/lxc/incus/v6/shared/api` - API types

---

## Definition of Done

- [ ] All code compiles with `go build ./...`
- [ ] Unit tests pass for platform detection
- [ ] Integration test: create → start → stop → delete cycle
- [ ] README documents platform-specific setup
- [ ] No linter errors (`golangci-lint run`)
