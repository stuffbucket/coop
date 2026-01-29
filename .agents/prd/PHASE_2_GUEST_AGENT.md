# Phase 2: Guest Agent PRD

**Product Requirements Document**  
**Timeline:** Week 2  
**Status:** Not Started  
**Dependencies:** Phase 1 (Core Infrastructure)

---

## Overview

Build the guest agent binary that runs inside each container, exposing an HTTP API for health checks, metrics collection, and command execution. Also create the controller-side client that communicates with these agents.

---

## Goals

1. Create a lightweight HTTP server that runs inside containers
2. Expose endpoints for readiness, status, and command execution
3. Collect and report system metrics (CPU, memory, disk, uptime)
4. Build a robust client for the controller to communicate with agents
5. Package the agent into a Docker image for Incus consumption

---

## Impossible States (Invariants)

| Invariant | Enforcement Strategy |
|-----------|---------------------|
| Agent accepting requests before server ready | Server blocks on listen before marking ready |
| Command execution without context deadline | Wrap all exec calls with context; reject if no deadline |
| Metrics endpoint returning partial/corrupt JSON | Serialize completely before writing response |
| Agent client connected to non-existent agent | WaitReady() required before any operation |
| HTTP response started but body incomplete | Use proper response writers; defer cleanup |

---

## Boundary Conditions (∂S ≠ ∅)

### Guest Agent Boundaries

| Boundary | Expected Behavior |
|----------|------------------|
| Port already in use | Exit with clear error, non-zero status |
| /proc filesystem unavailable | Return zeroed metrics with error flag |
| Command execution timeout | Kill process, return partial output + timeout error |
| Command produces >1MB output | Truncate with marker, return truncated flag |
| Memory pressure during request | Respond with 503, agent continues running |
| SIGTERM received | Graceful shutdown, drain connections |

### Agent Client Boundaries

| Boundary | Expected Behavior |
|----------|------------------|
| Agent unreachable | Return error immediately (no infinite retry) |
| Agent responds with non-JSON | Return parse error with raw response preview |
| Network timeout | Configurable timeout, default 10s |
| Agent returns 5xx | Propagate error with status code context |
| WaitReady exceeds max wait | Return timeout error, don't block forever |

---

## Interface Anacoluthon Detection

### Guest Agent HTTP Contract

```
// Canonical REST pattern
GET  /ready   → 200 {"ready": true} | 503 {"ready": false}
GET  /status  → 200 {metrics...}    | 500 {"error": "..."}
POST /execute → 200 {output, error} | 400 {error} | 500 {error}

// Anacoluthon flags to avoid:
// - /ready returning 200 with {"ready": false} (status code semantic mismatch)
// - /status returning 200 with error field populated (mixed signals)
// - /execute returning HTML on error (content-type violation)
// - POST /execute accepting GET (method semantic violation)
```

### Agent Client Contract

```go
// Canonical interface
type AgentClient interface {
    Ready(ctx context.Context) bool           // Never errors, just bool
    WaitReady(ctx context.Context, d time.Duration) error
    GetStatus(ctx context.Context) (*AgentStatus, error)
    Execute(ctx context.Context, cmd string) (*ExecResult, error)
}

// Anacoluthon flags:
// - Ready() throwing exception instead of returning false
// - GetStatus() returning nil, nil (impossible valid state)
// - Execute() modifying passed command string (parameter mutation)
```

---

## Functional Requirements

### FR-1: Guest Agent Binary

**Location:** `cmd/guest-agent/`

```go
type AgentServer struct {
    port      string
    name      string
    startTime time.Time
    executor  *Executor
}

func (s *AgentServer) Start() error
func (s *AgentServer) Shutdown(ctx context.Context) error
```

### FR-2: HTTP Endpoints

#### GET /ready
- **Purpose:** Health check for orchestration
- **Response:** `{"ready": true}`
- **Status Codes:** 200 (ready), 503 (not ready)

#### GET /status
- **Purpose:** Return current agent state and metrics
- **Response:**
```json
{
  "name": "agent-phase1-001",
  "uptime": "2h 15m",
  "uptime_seconds": 8100,
  "cpu_percent": 15.2,
  "memory_mb": 256,
  "memory_percent": 12.5,
  "disk_free_mb": 8192,
  "disk_percent": 20.0,
  "timestamp": "2024-01-15T10:30:00Z"
}
```

#### POST /execute
- **Purpose:** Execute command in container
- **Request:**
```json
{
  "command": "python3 --version",
  "timeout_seconds": 30
}
```
- **Response:**
```json
{
  "output": "Python 3.11.0\n",
  "exit_code": 0,
  "error": "",
  "truncated": false,
  "duration_ms": 45
}
```

### FR-3: Metrics Collection

```go
type Metrics struct {
    Uptime        string    `json:"uptime"`
    UptimeSecs    int64     `json:"uptime_seconds"`
    CPUPercent    float64   `json:"cpu_percent"`
    MemoryMB      int64     `json:"memory_mb"`
    MemoryPercent float64   `json:"memory_percent"`
    DiskFreeMB    int64     `json:"disk_free_mb"`
    DiskPercent   float64   `json:"disk_percent"`
    Timestamp     time.Time `json:"timestamp"`
}
```

**Collection Sources:**
| Metric | Source | Method |
|--------|--------|--------|
| CPU | `/proc/stat` | Parse user+system vs idle |
| Memory | `/proc/meminfo` | MemTotal - MemAvailable |
| Disk | `df /` | Available/Total from statfs |
| Uptime | Process start time | time.Since(startTime) |

### FR-4: Command Executor

```go
type Executor struct {
    maxOutputSize int           // Default 1MB
    defaultTimeout time.Duration // Default 30s
}

type ExecResult struct {
    Output     string `json:"output"`
    ExitCode   int    `json:"exit_code"`
    Error      string `json:"error,omitempty"`
    Truncated  bool   `json:"truncated"`
    DurationMs int64  `json:"duration_ms"`
}

func (e *Executor) Execute(ctx context.Context, cmd string, timeout time.Duration) (*ExecResult, error)
```

**Execution Rules:**
- Commands run via `sh -c "command"`
- Stdout and stderr combined
- Process killed on context cancellation
- Output truncated at maxOutputSize

### FR-5: Agent Client

**Location:** `internal/agent/client.go`

```go
type AgentClient struct {
    baseURL    string
    httpClient *http.Client
    timeout    time.Duration
}

func NewAgentClient(hostPort string, timeout time.Duration) *AgentClient
func (c *AgentClient) Ready(ctx context.Context) bool
func (c *AgentClient) WaitReady(ctx context.Context, maxWait time.Duration) error
func (c *AgentClient) GetStatus(ctx context.Context) (*AgentStatus, error)
func (c *AgentClient) Execute(ctx context.Context, command string) (*ExecResult, error)
```

### FR-6: Base Image

> **Full specification:** See [BASE_IMAGE_SPECIFICATION.md](BASE_IMAGE_SPECIFICATION.md) for complete toolchain requirements.

The base image must include:
- **Languages:** Node.js 22.x, Python 3.12.x, Go 1.22.x, Rust stable, Ruby, Java 21, PHP
- **Package managers:** npm/yarn/pnpm, pip/uv/pipx, cargo, gem, composer, maven/gradle
- **Build tools:** make, cmake, gcc, clang
- **CLI tools:** git, curl, jq, yq, ripgrep, fd, fzf
- **Language servers:** typescript-language-server, pyright, gopls, rust-analyzer
- **Linters:** shellcheck, hadolint, eslint, ruff, golangci-lint

**Minimal Dockerfile (see full spec for production):**
```dockerfile
FROM ubuntu:24.04

# See BASE_IMAGE_SPECIFICATION.md for full package list
RUN apt-get update && apt-get install -y \
    build-essential curl wget git \
    python3 python3-pip nodejs npm golang \
    && rm -rf /var/lib/apt/lists/*

COPY guest-agent /opt/agent/guest-agent
RUN chmod +x /opt/agent/guest-agent

RUN useradd -m -s /bin/bash -u 1000 agent
USER agent
WORKDIR /home/agent/workspace

ENTRYPOINT ["/opt/agent/guest-agent"]
CMD ["--port", "8888"]
```

---

## Non-Functional Requirements

### NFR-1: Performance
- `/ready` response: < 5ms
- `/status` response: < 50ms
- `/execute` overhead: < 10ms (excluding command runtime)
- Agent binary size: < 10MB

### NFR-2: Reliability
- Agent survives container resource pressure
- Graceful degradation when metrics unavailable
- Clean shutdown on SIGTERM (10s drain)

### NFR-3: Security
- No shell injection (commands are sanitized context)
- Bind to all interfaces only (container internal)
- No credentials in metrics output

### NFR-4: Observability
- Request logging with method, path, duration, status
- Structured JSON logs to stdout
- Debug flag for verbose logging

---

## Acceptance Criteria

### AC-1: Agent Server
- [ ] Starts and binds to specified port
- [ ] Logs startup with port and name
- [ ] Handles SIGTERM gracefully
- [ ] Returns 503 before ready

### AC-2: /ready Endpoint
- [ ] Returns 200 + `{"ready": true}` when healthy
- [ ] Response time < 5ms

### AC-3: /status Endpoint
- [ ] Returns valid JSON with all metric fields
- [ ] CPU percentage is reasonable (0-100+)
- [ ] Memory values match `free -m` output
- [ ] Disk values match `df /` output

### AC-4: /execute Endpoint
- [ ] Executes simple command: `echo hello`
- [ ] Returns exit code for failed commands
- [ ] Respects timeout parameter
- [ ] Truncates output > 1MB

### AC-5: Agent Client
- [ ] Connect to running agent
- [ ] WaitReady succeeds when agent is up
- [ ] WaitReady times out when agent is down
- [ ] GetStatus returns parsed metrics
- [ ] Execute returns command output

### AC-6: Docker Image
- [ ] Builds successfully
- [ ] Runs and responds to /ready
- [ ] Can be imported into Incus

---

## Technical Risks

| Risk | Impact | Mitigation |
|------|--------|-----------|
| /proc not available in some containers | Metrics collection fails | Graceful fallback to zero values |
| Command injection via /execute | Security vulnerability | Document as trusted controller only |
| Agent crashes leave container orphaned | Resource leak | Controller monitors agent health |

---

## Dependencies

**Build:**
- Go 1.21+
- Docker for image building

**Runtime:**
- Linux container environment
- /proc filesystem (for metrics)

---

## Definition of Done

- [ ] Guest agent builds as static binary
- [ ] All three endpoints functional
- [ ] Unit tests for metrics collection
- [ ] Integration test: agent in container responds
- [ ] Docker image builds and runs
- [ ] Agent client communicates successfully
- [ ] Documentation for protocol
