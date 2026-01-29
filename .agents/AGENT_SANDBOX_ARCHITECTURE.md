# Agent Sandbox Manager - Architecture & Implementation

**Goal:** Cross-platform Go tool to spin up/down system containers with guest agents that report back to a central controller.

---

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Go Application (Central Controller)                          │
│                                                              │
│  SandboxManager {                                           │
│    Build(baseImage)         → Create image with tools      │
│    StartPhase1()            → Spin up containers           │
│    StartPhase2()            → Start second batch           │
│    GetAgentStatus()         → Poll guest agents            │
│    StopAll()                → Clean up everything          │
│  }                                                          │
│                                                              │
│  GuestAgentController {                                    │
│    RegisterAgent()          → Agent checks in              │
│    SendCommand()            → Tell agent what to do        │
│    CollectMetrics()         → Get status from agent        │
│  }                                                          │
└────────────────┬─────────────────────────────────────────────┘
                 │
        ┌────────┴─────────┐
        │                  │
        ▼                  ▼
   ┌──────────┐      ┌──────────────┐
   │ macOS    │      │ WSL2/Windows │
   │ ↓        │      │ ↓            │
   │ Lima+    │      │ Ubuntu       │
   │ Incus    │      │ +Incus       │
   └────┬─────┘      └─────┬────────┘
        │                  │
        ▼                  ▼
    ┌──────────────────────────────┐
    │ Incus (identical API)         │
    │                              │
    │ containers: [               │
    │   agent-phase1-001,         │
    │   agent-phase1-002,         │
    │   agent-phase2-001,         │
    │   ...                       │
    │ ]                           │
    └───────────┬──────────────────┘
                │
    ┌───────────┴────────────────────────┐
    │                                    │
    ▼                                    ▼
Container 1                         Container 2
  ↓                                  ↓
Guest Agent (HTTP)        Guest Agent (HTTP)
  ↓                                  ↓
Listens on /agent-socket   Listens on /agent-socket
Reports status             Reports status
Executes commands          Executes commands
```

---

## Project Structure

```
sandbox-manager/
├── cmd/
│   ├── sandbox-manager/
│   │   ├── main.go              # CLI entry point
│   │   ├── build.go             # build-image command
│   │   ├── deploy.go            # deploy command
│   │   └── status.go            # status command
│   │
│   └── guest-agent/             # Runs inside containers
│       ├── main.go              # Agent entry point
│       ├── server.go            # HTTP server
│       ├── executor.go          # Command execution
│       └── reporter.go          # Status reporting
│
├── internal/
│   ├── sandbox/
│   │   ├── manager.go           # SandboxManager interface
│   │   ├── container.go         # Container lifecycle
│   │   ├── phase.go             # Phased startup logic
│   │   └── platform.go          # Platform detection
│   │
│   ├── image/
│   │   ├── builder.go           # Build base images
│   │   └── registry.go          # Image management
│   │
│   ├── agent/
│   │   ├── client.go            # Client to guest agents
│   │   ├── protocol.go          # Agent communication
│   │   └── messages.go          # Message types
│   │
│   └── config/
│       ├── config.go            # Configuration
│       └── phases.go            # Phase definitions
│
├── pkg/
│   └── incus/
│       ├── client_wrapper.go    # Incus client helpers
│       └── platform.go          # Platform-specific setup
│
├── Dockerfile                   # For base agent image
├── Makefile
├── go.mod
├── go.sum
├── README.md
└── LICENSE
```

---

## Core Types

```go
// internal/sandbox/manager.go

package sandbox

import (
    "context"
    "github.com/lxc/incus/v6/client"
)

// Phase represents a set of containers to start together
type Phase struct {
    Name       string
    Count      int           // How many containers to start
    ImageRef   string        // What image to use
    Resources  ResourceLimit // CPU, memory, etc.
    WaitFor    []string      // Phase dependencies
}

type ResourceLimit struct {
    CPUs   string // "2"
    Memory string // "2GB"
}

// SandboxManager controls all container lifecycle
type SandboxManager struct {
    client        client.InstanceServer
    baseImageRef  string
    phases        []Phase
    containers    map[string]*ManagedContainer
    agentClients  map[string]*AgentClient
}

// ManagedContainer wraps Incus container + guest agent
type ManagedContainer struct {
    ID         string        // agent-phase1-001
    Phase      string        // phase1
    Status     string        // running, stopped, error
    CreatedAt  time.Time
    Agent      *AgentClient  // Connection to guest agent
}

// Build base image with tools
func (m *SandboxManager) BuildBaseImage(ctx context.Context, dockerfile string) error {
    // 1. Build Docker image locally
    // 2. Convert to Incus image format or push to registry
    // 3. Store reference
}

// Start containers in phases
func (m *SandboxManager) StartPhase(ctx context.Context, phaseName string) error {
    phase := m.findPhase(phaseName)

    // For each container in phase
    for i := 0; i < phase.Count; i++ {
        containerID := fmt.Sprintf("agent-%s-%03d", phaseName, i)

        // Create container from base image
        if err := m.createContainer(ctx, containerID, phase); err != nil {
            return err
        }

        // Start container
        if err := m.startContainer(ctx, containerID); err != nil {
            return err
        }

        // Wait for guest agent to be ready
        agent, err := m.waitForAgent(ctx, containerID, 30*time.Second)
        if err != nil {
            return err
        }

        m.containers[containerID] = &ManagedContainer{
            ID:    containerID,
            Phase: phaseName,
            Status: "running",
            Agent: agent,
        }
    }

    return nil
}

// Get status from all agents
func (m *SandboxManager) Status(ctx context.Context) (map[string]interface{}, error) {
    status := make(map[string]interface{})

    for id, container := range m.containers {
        if container.Agent == nil {
            status[id] = map[string]string{"status": "no_agent"}
            continue
        }

        agentStatus, err := container.Agent.GetStatus(ctx)
        if err != nil {
            status[id] = map[string]string{"status": "unreachable", "error": err.Error()}
        } else {
            status[id] = agentStatus
        }
    }

    return status, nil
}

// Execute command on specific agent
func (m *SandboxManager) ExecOnAgent(ctx context.Context, containerID string, cmd string) (string, error) {
    container, ok := m.containers[containerID]
    if !ok {
        return "", fmt.Errorf("container not found: %s", containerID)
    }

    if container.Agent == nil {
        return "", fmt.Errorf("agent not connected for: %s", containerID)
    }

    return container.Agent.Execute(ctx, cmd)
}

// Stop and remove all containers
func (m *SandboxManager) StopAll(ctx context.Context) error {
    for id := range m.containers {
        if err := m.stopContainer(ctx, id); err != nil {
            // Log but continue
        }
    }

    m.containers = make(map[string]*ManagedContainer)
    m.agentClients = make(map[string]*AgentClient)

    return nil
}
```

---

## Guest Agent

```go
// cmd/guest-agent/main.go

package main

import (
    "flag"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
)

var (
    port = flag.String("port", "8888", "HTTP port for agent")
    name = flag.String("name", "", "Agent name (e.g., agent-phase1-001)")
)

func main() {
    flag.Parse()

    // Create HTTP server for controller to communicate with
    server := &http.Server{
        Addr: ":" + *port,
    }

    executor := NewExecutor()

    http.HandleFunc("/status", handleStatus(executor))
    http.HandleFunc("/execute", handleExecute(executor))
    http.HandleFunc("/ready", handleReady())

    // Start server
    go server.ListenAndServe()
    log.Printf("Guest agent %s listening on :%s\n", *name, *port)

    // Wait for shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
    <-sigChan

    server.Close()
}

// GET /ready - health check
func handleReady() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{"ready": true}`))
    }
}

// GET /status - return agent status
func handleStatus(executor *Executor) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        status := executor.GetStatus()
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(status)
    }
}

// POST /execute - run command
func handleExecute(executor *Executor) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req ExecuteRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }

        output, err := executor.Execute(r.Context(), req.Command)

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(ExecuteResponse{
            Output: output,
            Error:  errStr(err),
        })
    }
}

type ExecuteRequest struct {
    Command string `json:"command"`
}

type ExecuteResponse struct {
    Output string `json:"output"`
    Error  string `json:"error,omitempty"`
}

// Executor runs commands inside the container
type Executor struct {
    startTime time.Time
}

func NewExecutor() *Executor {
    return &Executor{
        startTime: time.Now(),
    }
}

func (e *Executor) GetStatus() map[string]interface{} {
    return map[string]interface{}{
        "running_since": e.startTime,
        "uptime":        time.Since(e.startTime).String(),
    }
}

func (e *Executor) Execute(ctx context.Context, command string) (string, error) {
    cmd := exec.CommandContext(ctx, "sh", "-c", command)

    output, err := cmd.CombinedOutput()
    return string(output), err
}

func errStr(err error) string {
    if err == nil {
        return ""
    }
    return err.Error()
}
```

---

## Agent Client (Communicates with Guest Agents)

```go
// internal/agent/client.go

package agent

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

type AgentClient struct {
    baseURL string
    httpClient *http.Client
    timeout time.Duration
}

// NewAgentClient creates a client to a guest agent
// socketPath: /var/lib/incus/unix.socket or TCP address
func NewAgentClient(hostPort string, timeout time.Duration) *AgentClient {
    return &AgentClient{
        baseURL: fmt.Sprintf("http://%s", hostPort),
        httpClient: &http.Client{
            Timeout: timeout,
        },
        timeout: timeout,
    }
}

// Ready checks if agent is ready
func (ac *AgentClient) Ready(ctx context.Context) bool {
    _, err := ac.do(ctx, "GET", "/ready", nil, nil)
    return err == nil
}

// WaitReady waits for agent to become ready
func (ac *AgentClient) WaitReady(ctx context.Context, maxWait time.Duration) error {
    deadline := time.Now().Add(maxWait)
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()

    for {
        if ac.Ready(ctx) {
            return nil
        }

        select {
        case <-ticker.C:
            if time.Now().After(deadline) {
                return fmt.Errorf("agent not ready after %v", maxWait)
            }
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

// GetStatus retrieves agent status
func (ac *AgentClient) GetStatus(ctx context.Context) (map[string]interface{}, error) {
    var status map[string]interface{}
    _, err := ac.do(ctx, "GET", "/status", nil, &status)
    return status, err
}

// Execute runs a command on the agent
func (ac *AgentClient) Execute(ctx context.Context, command string) (string, error) {
    reqBody := map[string]string{"command": command}

    var resp struct {
        Output string `json:"output"`
        Error  string `json:"error"`
    }

    _, err := ac.do(ctx, "POST", "/execute", reqBody, &resp)
    if err != nil {
        return "", err
    }

    if resp.Error != "" {
        return resp.Output, fmt.Errorf(resp.Error)
    }

    return resp.Output, nil
}

// Helper to make HTTP requests
func (ac *AgentClient) do(ctx context.Context, method, path string, reqBody interface{}, respBody interface{}) (*http.Response, error) {
    url := ac.baseURL + path

    var body io.Reader
    if reqBody != nil {
        data, err := json.Marshal(reqBody)
        if err != nil {
            return nil, err
        }
        body = bytes.NewReader(data)
    }

    req, err := http.NewRequestWithContext(ctx, method, url, body)
    if err != nil {
        return nil, err
    }

    req.Header.Set("Content-Type", "application/json")

    resp, err := ac.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return nil, fmt.Errorf("agent returned %d", resp.StatusCode)
    }

    if respBody != nil {
        if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
            return nil, err
        }
    }

    return resp, nil
}
```

---

## Container Lifecycle

```go
// internal/sandbox/container.go

package sandbox

import (
    "context"
    "fmt"
    "github.com/lxc/incus/v6/client"
)

func (m *SandboxManager) createContainer(ctx context.Context, containerID string, phase Phase) error {
    // Container config
    req := client.InstanceCreateRequest{
        Name: containerID,
        Type: "container",
        Source: client.InstanceSource{
            Type: "image",
            Alias: phase.ImageRef,
        },
        Profiles: []string{"default"},
        Config: map[string]string{
            "limits.cpu":    phase.Resources.CPUs,
            "limits.memory": phase.Resources.Memory,
        },
    }

    // Create (but don't start yet)
    op, err := m.client.CreateInstance(req)
    if err != nil {
        return fmt.Errorf("create container %s: %w", containerID, err)
    }

    if err := op.Wait(); err != nil {
        return fmt.Errorf("wait for create %s: %w", containerID, err)
    }

    return nil
}

func (m *SandboxManager) startContainer(ctx context.Context, containerID string) error {
    // Start the container
    op, err := m.client.UpdateInstanceState(containerID, client.InstanceStatePut{
        Action:  "start",
        Timeout: -1,
    }, "")
    if err != nil {
        return fmt.Errorf("start container %s: %w", containerID, err)
    }

    return op.Wait()
}

func (m *SandboxManager) stopContainer(ctx context.Context, containerID string) error {
    // Graceful stop, then delete
    m.client.UpdateInstanceState(containerID, client.InstanceStatePut{
        Action:  "stop",
        Timeout: 30,
    }, "")

    op, err := m.client.DeleteInstance(containerID)
    if err != nil {
        return err
    }

    return op.Wait()
}

// Get container IP address
func (m *SandboxManager) getContainerIP(ctx context.Context, containerID string) (string, error) {
    container, _, err := m.client.GetInstance(containerID)
    if err != nil {
        return "", err
    }

    // Find the primary IP
    if len(container.ExpandedDevices) == 0 {
        return "", fmt.Errorf("no network devices for %s", containerID)
    }

    // In Incus, network info is in StateGet
    state, err := m.client.GetInstanceState(containerID)
    if err != nil {
        return "", err
    }

    // Parse state.Network to find IP
    for _, nic := range state.Network {
        for _, addr := range nic.Addresses {
            if addr.Family == "inet" {
                return addr.Address, nil
            }
        }
    }

    return "", fmt.Errorf("no IP found for %s", containerID)
}

// Wait for guest agent to be ready
func (m *SandboxManager) waitForAgent(ctx context.Context, containerID string, maxWait time.Duration) (*AgentClient, error) {
    // Get container IP
    ip, err := m.getContainerIP(ctx, containerID)
    if err != nil {
        return nil, err
    }

    // Create agent client
    agentClient := agent.NewAgentClient(ip+":8888", 5*time.Second)

    // Wait for it to be ready
    if err := agentClient.WaitReady(ctx, maxWait); err != nil {
        return nil, fmt.Errorf("agent not ready: %w", err)
    }

    return agentClient, nil
}
```

---

## Base Image (Dockerfile)

```dockerfile
# Dockerfile for agent sandbox

FROM ubuntu:24.04

# Install base tools + guest agent
RUN apt-get update && apt-get install -y \
    curl \
    wget \
    git \
    build-essential \
    python3 \
    python3-pip \
    nodejs \
    npm \
    # ... add whatever tools agents might need
    && rm -rf /var/lib/apt/lists/*

# Copy guest agent binary
COPY guest-agent /opt/agent/guest-agent
RUN chmod +x /opt/agent/guest-agent

# Create agent user
RUN useradd -m -s /bin/bash agent

# Set up entrypoint
ENTRYPOINT ["/opt/agent/guest-agent", "--name", "$(hostname)"]
```

---

## Configuration & Phases

```go
// internal/config/phases.go

package config

type PhaseConfig struct {
    Phases []Phase `yaml:"phases"`
}

type Phase struct {
    Name      string `yaml:"name"`      // "phase1", "phase2"
    Count     int    `yaml:"count"`     // How many containers
    Image     string `yaml:"image"`     // Image ref
    CPU       string `yaml:"cpu"`       // "2"
    Memory    string `yaml:"memory"`    // "2GB"
    WaitFor   []string `yaml:"wait_for"` // Dependencies
}

// phases.yaml example:
/*
phases:
  - name: phase1
    count: 3
    image: images:claude-agent-ubuntu
    cpu: "2"
    memory: "2GB"
    wait_for: []

  - name: phase2
    count: 5
    image: images:claude-agent-ubuntu
    cpu: "1"
    memory: "1GB"
    wait_for:
      - phase1

  - name: phase3-gpu
    count: 1
    image: images:claude-agent-ubuntu-gpu
    cpu: "4"
    memory: "4GB"
    wait_for:
      - phase2
*/
```

---

## CLI Interface

```go
// cmd/sandbox-manager/main.go

package main

import (
    "flag"
    "fmt"
    "log"
    "os"
)

func main() {
    if len(os.Args) < 2 {
        printUsage()
        os.Exit(1)
    }

    command := os.Args[1]
    args := os.Args[2:]

    switch command {
    case "build":
        handleBuild(args)
    case "deploy":
        handleDeploy(args)
    case "status":
        handleStatus(args)
    case "stop":
        handleStop(args)
    case "exec":
        handleExec(args)
    default:
        fmt.Printf("Unknown command: %s\n", command)
        printUsage()
    }
}

func handleBuild(args []string) {
    fs := flag.NewFlagSet("build", flag.ExitOnError)
    dockerfile := fs.String("dockerfile", "Dockerfile", "Dockerfile path")
    imageName := fs.String("name", "claude-agent-ubuntu", "Image name")
    fs.Parse(args)

    mgr, err := sandbox.NewManager()
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Building base image: %s\n", *imageName)
    if err := mgr.BuildBaseImage(context.Background(), *dockerfile); err != nil {
        log.Fatal(err)
    }
    fmt.Println("✓ Base image built")
}

func handleDeploy(args []string) {
    fs := flag.NewFlagSet("deploy", flag.ExitOnError)
    configPath := fs.String("config", "phases.yaml", "Phase config file")
    fs.Parse(args)

    mgr, err := sandbox.NewManager()
    if err != nil {
        log.Fatal(err)
    }

    // Load phase config
    config, err := config.LoadPhases(*configPath)
    if err != nil {
        log.Fatal(err)
    }

    // Start each phase
    for _, phase := range config.Phases {
        fmt.Printf("Starting phase: %s (count: %d)\n", phase.Name, phase.Count)
        if err := mgr.StartPhase(context.Background(), phase); err != nil {
            log.Fatalf("Failed to start phase %s: %v\n", phase.Name, err)
        }
        fmt.Printf("✓ Phase %s ready (%d containers)\n", phase.Name, phase.Count)
    }
}

func handleStatus(args []string) {
    mgr, err := sandbox.NewManager()
    if err != nil {
        log.Fatal(err)
    }

    status, err := mgr.Status(context.Background())
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Agent Status:")
    for id, s := range status {
        fmt.Printf("  %s: %v\n", id, s)
    }
}

func handleStop(args []string) {
    mgr, err := sandbox.NewManager()
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Stopping all containers...")
    if err := mgr.StopAll(context.Background()); err != nil {
        log.Fatal(err)
    }
    fmt.Println("✓ All containers stopped and removed")
}

func handleExec(args []string) {
    if len(args) < 2 {
        fmt.Println("Usage: sandbox-manager exec <container-id> <command>")
        os.Exit(1)
    }

    containerID := args[0]
    command := args[1]

    mgr, err := sandbox.NewManager()
    if err != nil {
        log.Fatal(err)
    }

    output, err := mgr.ExecOnAgent(context.Background(), containerID, command)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(output)
}

func printUsage() {
    fmt.Println(`sandbox-manager - Agent sandbox lifecycle manager

Usage:
  sandbox-manager build [--dockerfile Dockerfile] [--name image-name]
  sandbox-manager deploy [--config phases.yaml]
  sandbox-manager status
  sandbox-manager stop
  sandbox-manager exec <container-id> <command>
`)
}
```

---

## Usage Example

```bash
# Build base image with tools and agent
sandbox-manager build --dockerfile Dockerfile --name claude-agent-ubuntu

# Deploy: start all phases sequentially
sandbox-manager deploy --config phases.yaml

# Check status
sandbox-manager status

# Execute command on specific agent
sandbox-manager exec agent-phase1-001 "python3 --version"

# Clean up
sandbox-manager stop
```

---

## Implementation Roadmap

### Week 1: Core Infrastructure
- [ ] Project setup, Go module
- [ ] Incus client wrapper
- [ ] Container lifecycle (create, start, stop, delete)
- [ ] Platform detection (macOS/WSL2/Linux)

### Week 2: Guest Agent
- [ ] Guest agent HTTP server
- [ ] Status and execute endpoints
- [ ] Docker image build
- [ ] Agent client (controller side)

### Week 3: Sandbox Manager
- [ ] Phase management
- [ ] Phased startup with dependencies
- [ ] Agent readiness polling
- [ ] Status aggregation

### Week 4: CLI & Polish
- [ ] CLI commands (build, deploy, status, stop, exec)
- [ ] Configuration loading
- [ ] Error handling
- [ ] Documentation

---

## Key Design Decisions

1. **Guest Agent over Socket Communication**
   - Simpler than Unix socket forwarding
   - HTTP is language-agnostic (easy to add agents in other languages)
   - Can be extended with callbacks, webhooks, etc.

2. **Phases for Startup**
   - Allows dependencies between container groups
   - Some agents need others to be ready first
   - Phased approach = parallelizable per phase

3. **Platform Abstraction**
   - Incus API is identical everywhere
   - Only setup is platform-specific (Lima on macOS, distro on WSL2)
   - Agent code sees no platform differences

4. **Image Building**
   - Single base image with tools + agent
   - All containers start from same known state
   - Faster startup than provisioning per-container

5. **Stateless Agent Design**
   - Each container is ephemeral
   - No persistent state in containers (or minimal)
   - Central controller maintains state

---

## What You Build On Top

Once you have the sandbox manager, you use it from Claude Code:

```go
// In Claude Code agent execution

// Prepare phase config
phaseCfg := &config.PhaseConfig{
    Phases: []config.Phase{
        {
            Name:  "tool-agents",
            Count: 3,
            Image: "images:claude-agent-ubuntu",
            CPU:   "2",
            Memory: "2GB",
        },
    },
}

// Start sandboxes
mgr, _ := sandbox.NewManager()
mgr.StartPhase(ctx, "tool-agents")

// Get agents
status, _ := mgr.Status(ctx)

// Run code in agent
output, _ := mgr.ExecOnAgent(ctx, "agent-tool-agents-001", "/opt/agent-binary")

// Clean up
mgr.StopAll(ctx)
```

Everything is fast, efficient, and consistent across platforms.

