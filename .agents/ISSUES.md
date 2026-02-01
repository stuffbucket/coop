# Code Smells & Anti-Patterns Assessment

## High Priority

### 1. God File: main.go (~800 lines)

**Location:** `cmd/coop/main.go`

All command implementations are inline in a single file. This creates tight coupling and makes testing difficult.

**Recommendation:** Extract commands to:
- `cmd/coop/container.go` (create, start, stop, delete, list, status)
- `cmd/coop/mount.go` (mount subcommands)
- `cmd/coop/snapshot.go` (snapshot subcommands)
- `cmd/coop/vm.go` (vm subcommands)
- `cmd/coop/image.go` (image subcommands)

Or adopt a command framework (cobra, urfave/cli) with proper subcommand registration.

---

### 2. Missing Context Propagation

**Location:** All packages

No `context.Context` usage anywhere. Operations can't be cancelled, timeouts aren't controllable externally.

```go
// Current
func (m *Manager) Create(cfg ContainerConfig) error

// Should be
func (m *Manager) Create(ctx context.Context, cfg ContainerConfig) error
```

**Impact:** Long-running operations (cloud-init wait, image build) cannot be interrupted gracefully.

---

### 3. Config Loading Called Multiple Times

**Locations:**
- `incus.Connect()` → calls `config.Load()`
- `sandbox.NewManager()` → calls `config.Load()`
- `main.go` vmCmd → calls `config.Load()`
- `main.go` imageListCmd → calls `config.Load()`

**Recommendation:** Load config once at startup in `main()`, pass via dependency injection:

```go
func main() {
    cfg, err := config.Load()
    // ...
    mgr, err := sandbox.NewManagerWithConfig(cfg)
}
```

---

### 4. Tight Coupling: incus ↔ vm

**Location:** `internal/incus/client.go` lines 55-65

```go
case PlatformMacOS:
    vmMgr, err := vm.NewManager(cfg)
    if err := vmMgr.EnsureRunning(); err != nil { ... }
```

The incus client implicitly starts the VM when connecting. This hidden side effect violates separation of concerns.

**Recommendation:** Make VM startup explicit at the call site:

```go
// In main.go or sandbox.NewManager()
if runtime.GOOS == "darwin" {
    vmMgr.EnsureRunning()
}
client, err := incus.Connect()
```

---

### 5. No Interface for incus.Client

**Location:** `internal/sandbox/manager.go`

```go
type Manager struct {
    client *incus.Client  // concrete type
    config *config.Config
}
```

Direct dependency on concrete `*incus.Client` makes unit testing impossible without a real Incus daemon.

**Recommendation:** Define an interface:

```go
type ContainerClient interface {
    CreateContainer(name, image string, config map[string]string, profiles []string) error
    StartContainer(name string) error
    StopContainer(name string, force bool) error
    // ...
}
```

---

## Medium Priority

### 6. Mixed Concerns in manager.go

**Location:** `internal/sandbox/manager.go`

The `Manager` type handles:
- Container CRUD operations
- SSH key/config management (`EnsureSSHKeys`, `UpdateSSHConfig`)
- Image building (`BuildBaseImage`)
- Mount validation/seatbelt logic (`IsSeatbelted`)
- Cloud-init waiting

**Recommendation:** Extract to separate packages:
- `internal/sandbox/seatbelt/` — protected path validation
- `internal/sandbox/image/` — image building logic
- Keep SSH functions in `sshkeys` package (they're already there, just exported via manager)

---

### 7. Global State in ui.go

**Location:** `internal/ui/ui.go` line 45

```go
isTTY = term.IsTerminal(int(os.Stdout.Fd()))
```

Evaluated once at package load time. Breaks if stdout is redirected after import.

**Recommendation:** Use a function call or lazy initialization:

```go
func IsTTY() bool {
    return term.IsTerminal(int(os.Stdout.Fd()))
}
```

---

### 8. Stringly-Typed Container States

**Locations:**
- `internal/sandbox/manager.go` — `container.Status == "Running"`
- `internal/incus/client.go` — returns raw API strings
- `cmd/coop/main.go` — comparisons throughout

**Recommendation:** Define a typed constant:

```go
type ContainerState string

const (
    StateRunning ContainerState = "Running"
    StateStopped ContainerState = "Stopped"
    StateFrozen  ContainerState = "Frozen"
)
```

---

### 9. Repeated Error Handling Pattern

**Location:** `cmd/coop/main.go` — every command function

```go
mgr, err := sandbox.NewManager()
if err != nil {
    ui.Errorf("Error: %v", err)
    os.Exit(1)
}
```

Duplicated ~15 times.

**Recommendation:** Extract helper:

```go
func mustManager() *sandbox.Manager {
    mgr, err := sandbox.NewManager()
    if err != nil {
        ui.Errorf("Error: %v", err)
        os.Exit(1)
    }
    return mgr
}
```

Or better: use a command framework with middleware for common setup.

---

### 10. SSH Config Append Without Deduplication

**Location:** `internal/sshkeys/keys.go` `WriteSSHConfig()`

```go
newConfig := string(existingConfig) + hostEntry
```

Creates duplicate host blocks on every container start.

**Recommendation:** Parse existing config, update or append:

```go
func WriteSSHConfig(containerName, ip string) error {
    // Read, find existing Host block for containerName, update or append
}
```

---

## Low Priority

### 11. Magic Numbers

**Locations:**
- `internal/sandbox/manager.go:216` — `timeout := time.After(10 * time.Minute)`
- `internal/sandbox/manager.go:217` — `ticker := time.NewTicker(3 * time.Second)`
- `internal/sandbox/manager.go:97` — `"limits.processes": "500"`

**Recommendation:** Move to config constants:

```go
const (
    CloudInitTimeout     = 10 * time.Minute
    CloudInitPollInterval = 3 * time.Second
    DefaultProcessLimit   = 500
)
```

---

### 12. Inconsistent Error Wrapping

**Examples:**
```go
// No wrap - loses stack context
return fmt.Errorf("container %s not found", name)

// Wrapped - preserves cause
return fmt.Errorf("failed to create container: %w", err)
```

**Recommendation:** Always wrap with `%w` when there's an underlying error. Use sentinel errors for known conditions:

```go
var ErrContainerNotFound = errors.New("container not found")
```

---

### 13. Unused/Dead Code

| Item | Location | Issue |
|------|----------|-------|
| `ContainerInfo.FullName` | manager.go | Always equals `Name` |
| `Config.AgentPort` | cloudinit/userdata.go | Hardcoded to 8888, never configurable |
| `FallbackImage` | manager.go | Only used in edge error paths |

---

### 14. Cloud-init Template Embedded as String

**Location:** `internal/cloudinit/userdata.go`

The entire cloud-init YAML is a raw string constant (~80 lines). Hard to maintain and validate.

**Recommendation:** 
- Use `embed.FS` to load from `templates/cloud-init.yaml`
- Or use structured Go types that marshal to YAML

---

## Architectural Suggestions

### Dependency Injection Pattern

```go
// cmd/coop/app.go
type App struct {
    Config  *config.Config
    Manager *sandbox.Manager
    VMManager *vm.Manager
    Logger  *logging.Logger
}

func NewApp() (*App, error) {
    cfg, err := config.Load()
    if err != nil {
        return nil, err
    }
    // Wire up dependencies once
    return &App{...}, nil
}
```

### Command Interface

```go
type Command interface {
    Name() string
    Run(ctx context.Context, args []string) error
}

// Register commands
commands := map[string]Command{
    "create": &CreateCommand{app},
    "start":  &StartCommand{app},
    // ...
}
```

---

## Testing Gaps

Current test coverage appears limited to:
- `config/config_test.go`
- `cloudinit/userdata_test.go`
- `names/names_test.go`
- `ui/ui_test.go`

Missing tests for:
- `sandbox.Manager` (requires interface extraction)
- `incus.Client` (requires mock/interface)
- `vm.ColimaBackend` / `vm.LimaBackend`
- Integration tests for command flows

---

## Summary

| Priority | Count | Key Theme |
|----------|-------|-----------|
| High | 5 | Architecture/coupling issues |
| Medium | 5 | Code organization |
| Low | 4 | Style/maintenance |

**Recommended first steps:**
1. Extract interfaces for `incus.Client` and `sandbox.Manager`
2. Add `context.Context` to all I/O operations
3. Single config load with dependency injection
4. Make VM startup explicit (remove from `incus.Connect`)

---

## Active Workstreams (coordination)
- Phase 1 (other agent, worktree `coop-cleanup-phase1`): #7 isTTY fix, #10 SSH config dedup, #11 magic numbers → constants, #12 error wrapping, #13 dead code removal.
- Phase 2 Security Hardening (this agent, worktree `coop-sec-phase2`, branch `codex/phase2-security`): seatbelt auth code hardening (#S1), signed/pinned installers for base image and cloud-init (#S2), tighten firewall/SSH defaults (#S3), make nesting/idmap opt-in (#S4), add exec log redaction/quiet mode (#S5), pin remote fallback image (#S6).

Security issue IDs:
- #S1 Predictable seatbelt auth code
- #S2 Unsigned remote installers
- #S3 Broad firewall + permissive SSH defaults
- #S4 Nesting/raw.idmap default-on
- #S5 Exec output logging secrets
- #S6 Unpinned fallback image

---

## Work Dependency Tree

```
LAYER 0 - Safe First (no downstream impact, isolated changes)
├── #13 Remove dead code (ContainerInfo.FullName, unused fields)
├── #11 Extract magic numbers to constants
├── #12 Fix inconsistent error wrapping
├── #7  Fix global isTTY state → function call
├── #10 Fix SSH config append deduplication
└── #14 Extract cloud-init template to embed.FS

LAYER 1 - Additive Types (no signature changes, extends existing)
├── #8  Define ContainerState type constants
│       (use alongside strings initially, migrate incrementally)
└── #9  Extract mustManager() helper in main.go
        (localized refactor, reduces noise before split)

LAYER 2 - Config Foundation (enables DI pattern)
└── #3  Single config.Load() with dependency injection
        ├── blocks: #4, #5
        └── required before: splitting main.go cleanly

LAYER 3 - Decoupling (requires #3)
├── #4  Remove VM auto-start from incus.Connect()
│       (make explicit at call sites)
└── #5  Extract ContainerClient interface
        └── blocks: #2 (context requires stable interface)

LAYER 4 - Cross-Cutting (requires stable interfaces)
└── #2  Add context.Context to all I/O operations
        └── blocks: #1 (don't split while signatures changing)

LAYER 5 - Structural Refactor (requires layers 0-4 stable)
├── #1  Split main.go into command files
│       (benefits from: #9 helper, #3 DI, #2 context)
└── #6  Extract seatbelt/image packages from manager.go
        (cleaner after #5 interface extraction)
```

### Execution Order (Phases)

**Phase 1: Cleanup (no dependencies, parallelizable)**
| Issue | Effort | Risk | Files Touched |
|-------|--------|------|---------------|
| #13 Dead code | 10 min | None | manager.go, userdata.go |
| #11 Magic numbers | 15 min | None | manager.go |
| #12 Error wrapping | 20 min | None | Multiple (grep-replaceable) |
| #7 isTTY global | 5 min | None | ui/ui.go |
| #10 SSH dedup | 30 min | Low | sshkeys/keys.go |

**Phase 2: Type Safety (additive, non-breaking)**
| Issue | Effort | Risk | Files Touched |
|-------|--------|------|---------------|
| #8 State constants | 20 min | Low | New file + incremental migration |
| #9 mustManager | 10 min | None | main.go only |

**Phase 3: Foundation (sequential, blocking)**
| Issue | Effort | Risk | Depends On |
|-------|--------|------|------------|
| #3 Config DI | 1-2 hr | Medium | Phase 1-2 complete |
| #4 VM decoupling | 30 min | Medium | #3 |
| #5 Interface extraction | 1 hr | Medium | #3 |

**Phase 4: Context (wide impact)**
| Issue | Effort | Risk | Depends On |
|-------|--------|------|------------|
| #2 Context propagation | 2-3 hr | High | #5 (stable interface) |

**Phase 5: Reorganization**
| Issue | Effort | Risk | Depends On |
|-------|--------|------|------------|
| #1 Split main.go | 1-2 hr | Medium | #2, #3, #9 |
| #6 Extract packages | 1 hr | Medium | #5 |

### Anti-Patterns to Avoid

1. **Don't split main.go first** — signatures will change with context.Context, causing merge conflicts
2. **Don't add context.Context before interfaces** — you'll change signatures twice
3. **Don't extract packages before DI** — you'll wire dependencies incorrectly then rewire

### Recommended Immediate Actions

Start with **Phase 1** items — they're safe, quick wins that reduce noise:

```bash
# Suggested order
1. git checkout -b cleanup/dead-code
   → Remove ContainerInfo.FullName, unused AgentPort
   → PR + merge

2. git checkout -b cleanup/magic-numbers  
   → Extract constants
   → PR + merge

3. git checkout -b fix/ssh-config-dedup
   → Fix WriteSSHConfig to update existing entries
   → PR + merge
```

Each Phase 1 item is independently mergeable without affecting others

---

# Security Posture (2026-02-01)

## High Priority

1) Predictable seatbelt auth code  
**Location:** `internal/sandbox/authcode.go`  
**Issue:** 6-digit code is deterministic (hostname+HOME), so any local process can generate valid codes to mount protected paths.  
**Recommendation:** Derive from a random 32-byte secret stored at `~/.config/coop/seatbelt.key` (0600), TOTP ±1 window, rate-limit attempts, and consider per-session nonce.

2) Unsigned remote installers in base image build  
**Locations:** `scripts/build-base-image.sh`, `internal/cloudinit/userdata.go`  
**Issue:** Root installs from NodeSource, Claude install.sh, GitHub CLI key URL, Go tarballs without checksum/signature verification. High supply-chain risk.  
**Recommendation:** Pin versions and SHA256/GPG-verify downloads; favor distro packages or a vetted mirror/OCI image built in CI.

## Medium Priority

3) Broad firewall opening and permissive SSH defaults  
**Location:** `internal/cloudinit/userdata.go`  
**Issue:** ufw allows TCP 8888 from 10.0.0.0/8; SSH enables TCP forwarding and passwordless sudo for `agent`. Potential lateral movement inside bridged network.  
**Recommendation:** Drop the 8888 allow unless required; disable forwarding and NOPASSWD by default or gate behind explicit flag.

4) Nesting + raw.idmap enabled by default  
**Location:** `internal/sandbox/manager.go` (`ensureAgentProfile`)  
**Issue:** `security.nesting=true` and `raw.idmap` map host UID into container while allowing arbitrary mounts; increases breakout/lateral risk if container is compromised.  
**Recommendation:** Make nesting/idmap opt-in flags on `coop create`; default to off and document trade-offs.

5) Exec output logged without redaction  
**Location:** `internal/logging/logging.go` via `ExecCommand`/`ExecCommandWithOutput` usage  
**Issue:** Secrets printed by `coop exec` can persist in rotated logs.  
**Recommendation:** Add `--no-log-output` or redaction mode; document logging behavior.

## Low Priority

6) Host image pull not pinned  
**Location:** `internal/sandbox/manager.go` fallback to `ubuntu/22.04/cloud`  
**Issue:** No fingerprint check on remote image; tamper window exists.  
**Recommendation:** Pin alias fingerprint or require verified local alias before use.
