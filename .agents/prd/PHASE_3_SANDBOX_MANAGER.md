# Phase 3: Sandbox Manager PRD

**Product Requirements Document**  
**Timeline:** Week 3  
**Status:** Not Started  
**Dependencies:** Phase 1 (Core Infrastructure), Phase 2 (Guest Agent)

---

## Overview

Build the central SandboxManager that orchestrates container lifecycle, manages phased deployments, tracks agent connections, and provides unified status aggregation across all managed containers.

---

## Goals

1. Implement phased container startup with dependency ordering
2. Maintain registry of all managed containers and their agents
3. Provide unified status view across all agents
4. Support command execution broadcast and targeted dispatch
5. Clean teardown of all resources on stop

---

## Impossible States (Invariants)

| Invariant | Enforcement Strategy |
|-----------|---------------------|
| Container in registry but agent nil (after startup) | Startup transaction: either both registered or neither |
| Phase started before dependencies complete | Topological sort + barrier before each phase |
| Two containers with same ID | Registry uses map; reject duplicate creates |
| Agent client stored for non-existent container | Composite registration: container + agent atomic |
| Phase partially started (some containers, not others) | Rollback on phase failure—remove partial containers |
| StopAll called during StartPhase | Mutex protecting state transitions |

---

## Boundary Conditions (∂S ≠ ∅)

### Phase Management Boundaries

| Boundary | Expected Behavior |
|----------|------------------|
| Phase with 0 containers | No-op, mark phase complete |
| Phase depends on non-existent phase | Error at config validation time |
| Circular phase dependencies | Detect cycle, reject configuration |
| All containers in phase fail | Report phase failure, don't proceed to dependents |
| Single container in phase fails | Configurable: fail-fast or continue |
| Phase timeout exceeded | Cancel remaining, return partial success info |

### Container Registry Boundaries

| Boundary | Expected Behavior |
|----------|------------------|
| Get status for unknown container | Return error with "not found" context |
| Exec on container with dead agent | Return "agent unreachable" error |
| List containers when none exist | Return empty slice, no error |
| Container creation race condition | Mutex protection, second create fails |
| Agent becomes unreachable after registration | Status shows "unreachable" state |

### Resource Boundaries

| Boundary | Expected Behavior |
|----------|------------------|
| System runs out of memory for containers | Incus error propagated with context |
| Network exhaustion (no IPs available) | Clear error about network resources |
| Storage full | Clear error about storage resources |
| Max containers per phase (configurable) | Validate at config time |

---

## Interface Anacoluthon Detection

### Manager Interface Contract

```go
// Canonical interface
type SandboxManager interface {
    // Image building
    BuildBaseImage(ctx context.Context, dockerfile string) error
    
    // Phase lifecycle
    StartPhase(ctx context.Context, phaseName string) error
    StartAllPhases(ctx context.Context) error
    
    // Status and operations
    Status(ctx context.Context) (map[string]ContainerStatus, error)
    GetContainerStatus(ctx context.Context, id string) (*ContainerStatus, error)
    
    // Execution
    ExecOnAgent(ctx context.Context, containerID string, cmd string) (*ExecResult, error)
    ExecOnAll(ctx context.Context, cmd string) (map[string]*ExecResult, error)
    ExecOnPhase(ctx context.Context, phase string, cmd string) (map[string]*ExecResult, error)
    
    // Cleanup
    StopPhase(ctx context.Context, phaseName string) error
    StopAll(ctx context.Context) error
}

// Anacoluthon flags to avoid:
// - StartPhase returning success but containers not running
// - Status returning nil map instead of empty map
// - ExecOnAgent silently failing for unknown containers
// - StopAll not actually stopping (async without wait)
// - BuildBaseImage mutating manager state unexpectedly
```

### Phase Configuration Contract

```go
// Canonical config structure
type PhaseConfig struct {
    Name     string   // Unique identifier
    Count    int      // ≥ 0
    Image    string   // Valid image reference
    CPU      string   // Valid resource spec
    Memory   string   // Valid resource spec
    WaitFor  []string // Existing phase names
}

// Anacoluthon flags:
// - Count < 0 (impossible container count)
// - Empty Name (unlinkable phase)
// - WaitFor containing self (self-dependency)
// - Image empty string (un-pullable)
```

### Container Status Contract

```go
// Canonical status structure
type ContainerStatus struct {
    ID        string        // Never empty
    Phase     string        // Never empty
    State     ContainerState // Enum, not free string
    IP        string        // Empty until assigned
    CreatedAt time.Time
    Agent     *AgentStatus  // nil only if State != Running
}

type ContainerState int
const (
    StateUnknown ContainerState = iota
    StateCreating
    StateRunning
    StateStopping
    StateStopped
    StateError
)

// Anacoluthon flags:
// - State == Running but Agent == nil
// - State == Stopped but Agent != nil
// - ID empty (untraceable container)
```

---

## Functional Requirements

### FR-1: SandboxManager Structure

```go
type SandboxManager struct {
    client       *incus.IncusClient
    phases       []Phase
    containers   map[string]*ManagedContainer
    agentClients map[string]*agent.AgentClient
    mu           sync.RWMutex
    config       *ManagerConfig
}

type ManagerConfig struct {
    AgentPort         int           // Default 8888
    AgentReadyTimeout time.Duration // Default 30s
    DefaultCPU        string        // Default "2"
    DefaultMemory     string        // Default "2GB"
}

type ManagedContainer struct {
    ID        string
    Phase     string
    State     ContainerState
    IP        string
    CreatedAt time.Time
    Error     string
}
```

### FR-2: Phase Management

**Phase Loading:**
```go
func LoadPhases(configPath string) ([]Phase, error)
func ValidatePhases(phases []Phase) error  // Check for cycles, missing deps
```

**Phase Ordering:**
- Build dependency graph from `WaitFor` fields
- Topological sort to determine startup order
- Detect and reject cycles

**Phase Startup Sequence:**
```
For each phase in sorted order:
  1. Wait for all WaitFor phases to be complete
  2. Create all containers for this phase (parallel)
  3. Start all containers for this phase (parallel)
  4. Wait for all agents to be ready (parallel with timeout)
  5. Mark phase complete
```

### FR-3: Container Lifecycle Integration

```go
func (m *SandboxManager) createContainerForPhase(ctx context.Context, phase Phase, index int) (*ManagedContainer, error)
func (m *SandboxManager) startContainer(ctx context.Context, id string) error
func (m *SandboxManager) waitForAgent(ctx context.Context, id string) error
func (m *SandboxManager) stopAndRemoveContainer(ctx context.Context, id string) error
```

**Container Naming:**
- Format: `agent-{phase}-{index:03d}`
- Example: `agent-phase1-001`, `agent-workers-042`

### FR-4: Status Aggregation

```go
func (m *SandboxManager) Status(ctx context.Context) (map[string]ContainerStatus, error)
```

**Aggregation Logic:**
1. Iterate all registered containers
2. For each with Running state, query agent status
3. Merge container metadata with agent metrics
4. Handle unreachable agents gracefully

**Status Response:**
```json
{
  "agent-phase1-001": {
    "id": "agent-phase1-001",
    "phase": "phase1",
    "state": "running",
    "ip": "10.0.0.101",
    "agent": {
      "uptime": "2h 15m",
      "cpu_percent": 15.2,
      "memory_mb": 256
    }
  },
  "agent-phase1-002": {
    "id": "agent-phase1-002",
    "phase": "phase1", 
    "state": "running",
    "ip": "10.0.0.102",
    "agent": null,
    "error": "agent unreachable"
  }
}
```

### FR-5: Command Execution

**Single Container:**
```go
func (m *SandboxManager) ExecOnAgent(ctx context.Context, containerID string, cmd string) (*ExecResult, error)
```

**Broadcast:**
```go
func (m *SandboxManager) ExecOnAll(ctx context.Context, cmd string) (map[string]*ExecResult, error)
func (m *SandboxManager) ExecOnPhase(ctx context.Context, phase string, cmd string) (map[string]*ExecResult, error)
```

**Execution Semantics:**
- Single: fail if container not found or agent unreachable
- Broadcast: best-effort, collect all results (including errors)
- Respect context cancellation

### FR-6: Cleanup

```go
func (m *SandboxManager) StopPhase(ctx context.Context, phaseName string) error
func (m *SandboxManager) StopAll(ctx context.Context) error
```

**Cleanup Sequence:**
1. Stop all containers (parallel, with timeout)
2. Delete all containers (parallel)
3. Remove from registry
4. Clear agent clients

---

## Non-Functional Requirements

### NFR-1: Performance
- Phase with 10 containers: < 30s total startup
- Status for 50 containers: < 5s
- Broadcast exec to 20 containers: < 2s overhead

### NFR-2: Concurrency
- Parallel container creation within phase
- Parallel agent readiness waiting
- Thread-safe registry access

### NFR-3: Reliability
- Partial failure handling (some containers succeed)
- Rollback on phase failure (configurable)
- Idempotent StopAll

### NFR-4: Observability
- Phase start/complete logging
- Container state transition logging
- Agent connection logging
- Execution timing

---

## Acceptance Criteria

### AC-1: Phase Management
- [ ] Load phases from YAML configuration
- [ ] Validate phase dependencies (no cycles)
- [ ] Start phases in dependency order
- [ ] Wait for dependencies before proceeding

### AC-2: Container Lifecycle
- [ ] Create containers with correct naming
- [ ] Apply resource limits from phase config
- [ ] Start containers and get IPs
- [ ] Connect agent clients
- [ ] Register in manager state

### AC-3: Status Aggregation
- [ ] Return status for all containers
- [ ] Include agent metrics for running containers
- [ ] Handle unreachable agents gracefully
- [ ] Show container state accurately

### AC-4: Command Execution
- [ ] Execute on single container by ID
- [ ] Execute on all containers
- [ ] Execute on phase subset
- [ ] Handle partial failures in broadcast

### AC-5: Cleanup
- [ ] Stop all containers gracefully
- [ ] Remove all containers
- [ ] Clear manager state
- [ ] Handle already-stopped containers

### AC-6: Concurrency
- [ ] Parallel container creation is safe
- [ ] Status queries don't block startup
- [ ] StopAll during startup is safe

---

## Technical Risks

| Risk | Impact | Mitigation |
|------|--------|-----------|
| Race between status query and shutdown | Inconsistent state returned | RWMutex protection |
| Phase dependency deadlock | System hangs | Cycle detection at config time |
| Agent ready timeout too short | False failures | Configurable, generous defaults |
| Too many parallel API calls to Incus | Rate limiting errors | Configurable concurrency limit |

---

## Dependencies

**Internal:**
- Phase 1: `pkg/incus` client wrapper
- Phase 2: `internal/agent` client

**External:**
- `gopkg.in/yaml.v3` for config parsing
- `golang.org/x/sync/errgroup` for parallel operations

---

## Definition of Done

- [ ] SandboxManager interface fully implemented
- [ ] Phase configuration loading and validation
- [ ] Integration test: multi-phase startup sequence
- [ ] Integration test: status aggregation with agents
- [ ] Integration test: broadcast execution
- [ ] Integration test: clean shutdown
- [ ] Unit tests for phase dependency sorting
- [ ] Documentation for phase configuration format
