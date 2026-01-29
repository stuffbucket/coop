# Phase 4: CLI & Polish PRD

**Product Requirements Document**  
**Timeline:** Week 4  
**Status:** Not Started  
**Dependencies:** Phase 1 (Core Infrastructure), Phase 2 (Guest Agent), Phase 3 (Sandbox Manager)

---

## Overview

Create a polished command-line interface that exposes all SandboxManager functionality, with comprehensive error handling, configuration management, and documentation. This phase transforms the library into a usable product.

---

## Goals

1. Build intuitive CLI with subcommand structure
2. Implement robust error handling with actionable messages
3. Support configuration via files, environment, and flags
4. Add output formatting options (table, JSON, quiet)
5. Create comprehensive documentation and examples
6. Implement signal handling and graceful shutdown

---

## Impossible States (Invariants)

| Invariant | Enforcement Strategy |
|-----------|---------------------|
| CLI executing without valid manager | Manager init failure = immediate exit |
| Conflicting output formats selected | Mutual exclusion in flag parsing |
| Config from file + conflicting CLI flags | Explicit precedence: flag > env > file |
| Command requires containers but none exist | Check state before operation, clear error |
| Partial JSON output (pretty-print failure) | Buffer complete JSON before output |
| SIGINT during output | Flush buffers, then exit |

---

## Boundary Conditions (∂S ≠ ∅)

### Input Validation Boundaries

| Boundary | Expected Behavior |
|----------|------------------|
| Empty container ID | Error: "container ID required" |
| Container ID with invalid characters | Error: "invalid container ID format" |
| Negative count flag | Error: "count must be positive" |
| Non-existent config file | Error: "config file not found: {path}" |
| Config file with invalid YAML | Error: "invalid config at line {n}: {detail}" |
| Empty command for exec | Error: "command required" |

### Output Boundaries

| Boundary | Expected Behavior |
|----------|------------------|
| No containers to list | Empty table with headers / empty JSON array |
| Very long command output | Paginate or truncate with indicator |
| Terminal not TTY | Disable colors, use simple format |
| Redirect to file | Disable progress indicators |
| Unicode in container names | Proper UTF-8 handling |

### Signal Handling Boundaries

| Boundary | Expected Behavior |
|----------|------------------|
| SIGINT during `start` | Cancel pending ops, cleanup created containers |
| SIGINT during `stop` | Complete current stop, exit |
| SIGINT during `exec` | Cancel execution, return partial results |
| SIGTERM | Same as SIGINT (graceful) |
| SIGHUP | Reload config (if watching) |
| Double SIGINT | Force immediate exit |

---

## Interface Anacoluthon Detection

### CLI Contract

```
// Canonical command grammar
sandbox-manager <command> [options] [args]

Commands:
  build     Build base container image
  deploy    Start containers according to config
  start     Start specific phase
  status    Show container status
  exec      Execute command in container(s)
  stop      Stop and remove containers
  config    Show/validate configuration

// Anacoluthon flags to avoid:
// - `status` that modifies state (query should be read-only)
// - `deploy --dry-run` that actually deploys
// - `stop` that returns success when nothing was running
// - `exec` returning 0 when command failed (exit code masking)
```

### Output Format Contract

```
// Table format (default for TTY)
CONTAINER         PHASE     STATUS    IP           CPU     MEM
agent-phase1-001  phase1    running   10.0.0.101   15%     256MB
agent-phase1-002  phase1    running   10.0.0.102   12%     248MB

// JSON format (--output json)
[
  {"id": "agent-phase1-001", "phase": "phase1", ...},
  {"id": "agent-phase1-002", "phase": "phase1", ...}
]

// Quiet format (--quiet)
agent-phase1-001
agent-phase1-002

// Anacoluthon flags:
// - Table missing headers (uninterpretable)
// - JSON array containing mixed types
// - Quiet mode printing extra information
```

### Exit Code Contract

| Exit Code | Meaning | Example |
|-----------|---------|---------|
| 0 | Success | All operations completed |
| 1 | General error | Unknown command, bad args |
| 2 | Configuration error | Invalid config file |
| 3 | Connection error | Can't reach Incus |
| 4 | Container error | Container create failed |
| 5 | Agent error | Agent unreachable |
| 130 | SIGINT | User interrupted |

---

## Functional Requirements

### FR-1: Command Structure

```
sandbox-manager
├── build       Build base image
│   ├── --dockerfile, -f    Dockerfile path (default: ./Dockerfile)
│   ├── --name, -n          Image name (default: agent-base)
│   └── --push              Push to registry
│
├── deploy      Deploy all phases
│   ├── --config, -c        Config file (default: ./phases.yaml)
│   ├── --phase, -p         Start only specific phase
│   └── --dry-run           Show what would be created
│
├── status      Show status
│   ├── --phase, -p         Filter by phase
│   ├── --output, -o        Format: table|json|wide
│   ├── --watch, -w         Continuous refresh
│   └── --quiet, -q         IDs only
│
├── exec        Execute command
│   ├── <target>            Container ID, phase name, or "all"
│   ├── <command>           Command to execute
│   ├── --timeout, -t       Execution timeout
│   └── --parallel          Parallel execution (default for broadcast)
│
├── stop        Stop containers
│   ├── --phase, -p         Stop specific phase
│   ├── --all, -a           Stop all (default)
│   └── --force             Skip graceful shutdown
│
├── config      Configuration management
│   ├── show                Display current config
│   ├── validate            Validate config file
│   └── init                Create example config
│
└── version     Show version info
```

### FR-2: Build Command

```go
func handleBuild(ctx context.Context, args BuildArgs) error

type BuildArgs struct {
    Dockerfile string
    ImageName  string
    Push       bool
}
```

**Behavior:**
1. Validate Dockerfile exists
2. Build guest-agent binary
3. Build Docker image with agent embedded
4. Import into Incus (or push to registry)
5. Report success/failure

### FR-3: Deploy Command

```go
func handleDeploy(ctx context.Context, args DeployArgs) error

type DeployArgs struct {
    ConfigPath string
    PhaseOnly  string
    DryRun     bool
}
```

**Behavior:**
1. Load and validate configuration
2. If dry-run: print plan and exit
3. Start phases in order (or single phase if specified)
4. Report progress with spinner/status line
5. Report final status

### FR-4: Status Command

```go
func handleStatus(ctx context.Context, args StatusArgs) error

type StatusArgs struct {
    Phase  string
    Output string  // table, json, wide, quiet
    Watch  bool
    Quiet  bool
}
```

**Output Formats:**

**Table (default):**
```
CONTAINER         PHASE     STATUS    IP           UPTIME
agent-phase1-001  phase1    running   10.0.0.101   2h 15m
agent-phase1-002  phase1    running   10.0.0.102   2h 15m
```

**Wide:**
```
CONTAINER         PHASE     STATUS    IP           CPU     MEM      DISK    UPTIME
agent-phase1-001  phase1    running   10.0.0.101   15.2%   256MB    20%     2h 15m
```

**JSON:**
```json
[
  {
    "id": "agent-phase1-001",
    "phase": "phase1",
    "status": "running",
    "ip": "10.0.0.101",
    "metrics": {
      "cpu_percent": 15.2,
      "memory_mb": 256
    }
  }
]
```

**Watch mode:**
- Clear screen between refreshes
- Configurable interval (default 2s)
- Ctrl-C to exit

### FR-5: Exec Command

```go
func handleExec(ctx context.Context, args ExecArgs) error

type ExecArgs struct {
    Target   string   // container ID, phase name, or "all"
    Command  string
    Timeout  time.Duration
    Parallel bool
}
```

**Target Resolution:**
- Exact container ID: execute on that container
- Phase name: execute on all containers in phase
- "all": execute on all containers

**Output:**
```
=== agent-phase1-001 ===
Python 3.11.0

=== agent-phase1-002 ===
Python 3.11.0
```

### FR-6: Stop Command

```go
func handleStop(ctx context.Context, args StopArgs) error

type StopArgs struct {
    Phase string
    All   bool
    Force bool
}
```

**Behavior:**
1. If phase specified: stop only that phase
2. If all: stop all containers
3. Grace period unless --force
4. Report progress and final count

### FR-7: Config Command

```go
func handleConfig(ctx context.Context, cmd string, args ConfigArgs) error
```

**Subcommands:**
- `config show`: Display effective configuration
- `config validate`: Check config file for errors
- `config init`: Create example phases.yaml

### FR-8: Configuration Loading

**Precedence (highest to lowest):**
1. Command-line flags
2. Environment variables (`SANDBOX_*`)
3. Config file
4. Defaults

**Environment Variables:**
```
SANDBOX_CONFIG_PATH   Config file location
SANDBOX_INCUS_SOCKET  Incus socket path override
SANDBOX_AGENT_PORT    Agent HTTP port
SANDBOX_LOG_LEVEL     Logging verbosity
```

### FR-9: Error Handling

**Error Message Format:**
```
Error: <operation> failed: <cause>

  <detailed explanation>

Suggestions:
  - <actionable fix 1>
  - <actionable fix 2>

For more information: sandbox-manager help <command>
```

**Example:**
```
Error: deploy failed: unable to connect to Incus

  The Incus socket was not found at /var/lib/incus/unix.socket

Suggestions:
  - Verify Incus is installed: incus --version
  - On macOS, ensure Lima VM is running: limactl list
  - Check socket path: SANDBOX_INCUS_SOCKET=/path/to/socket

For more information: sandbox-manager help deploy
```

---

## Non-Functional Requirements

### NFR-1: Usability
- Commands follow POSIX conventions
- Help text for all commands and flags
- Sensible defaults for all options
- Tab completion support (bash, zsh, fish)

### NFR-2: Performance
- CLI startup: < 100ms
- Status with 50 containers: < 5s
- No unnecessary network calls

### NFR-3: Reliability
- Graceful handling of SIGINT
- No zombie processes
- Cleanup on error paths

### NFR-4: Observability
- Verbose mode (`-v`, `-vv`) for debugging
- Timing information in verbose mode
- Structured logs to stderr

---

## Acceptance Criteria

### AC-1: Build Command
- [ ] Builds image from Dockerfile
- [ ] Embeds guest-agent binary
- [ ] Imports into Incus
- [ ] Reports success/failure clearly

### AC-2: Deploy Command
- [ ] Loads configuration file
- [ ] Starts containers in dependency order
- [ ] Shows progress during deployment
- [ ] Dry-run shows plan without executing

### AC-3: Status Command
- [ ] Table format is readable
- [ ] JSON format is valid JSON
- [ ] Wide format includes metrics
- [ ] Quiet format outputs only IDs
- [ ] Watch mode refreshes

### AC-4: Exec Command
- [ ] Executes on single container
- [ ] Executes on phase
- [ ] Executes on all
- [ ] Respects timeout
- [ ] Shows output per-container

### AC-5: Stop Command
- [ ] Stops specific phase
- [ ] Stops all containers
- [ ] Force skips grace period
- [ ] Reports containers stopped

### AC-6: Config Command
- [ ] Shows effective configuration
- [ ] Validates config with helpful errors
- [ ] Generates example config

### AC-7: Error Handling
- [ ] All errors have actionable messages
- [ ] Exit codes are consistent
- [ ] SIGINT handled gracefully

### AC-8: Documentation
- [ ] README with quick start
- [ ] All commands have --help
- [ ] Example configurations
- [ ] Troubleshooting guide

---

## Technical Risks

| Risk | Impact | Mitigation |
|------|--------|-----------|
| Terminal detection unreliable | Color/format issues | Use proven library (github.com/mattn/go-isatty) |
| Shell completion varies by shell | User confusion | Support top 3: bash, zsh, fish |
| Long-running commands unresponsive | Poor UX | Progress indicators, allow interrupt |

---

## Dependencies

**External:**
- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration management
- `github.com/olekukonko/tablewriter` - Table formatting
- `github.com/fatih/color` - Terminal colors
- `github.com/mattn/go-isatty` - TTY detection

---

## Definition of Done

- [ ] All commands implemented and tested
- [ ] Help text for every command and flag
- [ ] Tab completion for bash/zsh
- [ ] README with installation and quick start
- [ ] Configuration file format documented
- [ ] Example configurations provided
- [ ] Troubleshooting section
- [ ] All error paths have helpful messages
- [ ] Binary builds for Linux, macOS (arm64, amd64)
- [ ] Release automation (goreleaser)
- [ ] Integration test: full workflow (build → deploy → status → exec → stop)
