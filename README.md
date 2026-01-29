# Coop

Coop runs AI coding agents in isolated Linux containers with pre-installed development tooling. Containers launch in under 30 seconds, provide full filesystem and process isolation from the host, and include Python 3.13, Go 1.24, Node.js 24, GitHub CLI, and Claude CLI out of the box.

## Why Isolation Matters

AI agents that write and execute code need boundaries. Without isolation, an agent has the same access you do: home directory, SSH keys, cloud credentials, browser sessions. Coop creates ephemeral containers where agents work in `/home/agent/workspace`—optionally mirroring a host folder—with their own user account (UID 1000), separate from any host identity. Containers can be snapshotted, rolled back, or destroyed without affecting the host.

Protected directories like `~/.ssh`, `~/Library`, and system paths under SIP are blocked from mounting by default. Mounting them requires `--force`, which triggers a 6-digit authorization code sent via macOS notification. The code must be entered interactively within 15 seconds. This prevents automated processes from silently exposing credentials.

## Quick Start

On macOS, Incus requires a Linux VM. Coop manages this through Colima (preferred) or Lima:

```bash
brew install colima incus
colima start --profile incus --vm-type vz --vz-rosetta --network-address
```

Build and initialize:

```bash
make build
./coop init
./coop image build   # One-time, ~10 minutes
```

Create and connect:

```bash
./coop create myagent
./coop shell myagent
```

Mount a project directory:

```bash
./coop mount add myagent ~/projects/myapp
./coop shell myagent
# Inside container: cd /home/agent/myapp
```

## Commands

### Containers

| Command | Description |
|---------|-------------|
| `coop create <name>` | Create container (`--cpus`, `--memory`, `--disk`, `--workdir`) |
| `coop start <name>` | Start stopped container |
| `coop stop <name>` | Stop running container (`--force`) |
| `coop lock <name>` | Freeze container (pause all processes) |
| `coop unlock <name>` | Unfreeze container |
| `coop delete <name>` | Remove container (`--force`) |
| `coop list` | List all containers |
| `coop status <name>` | Show container details |
| `coop logs <name>` | View logs (`-f` follow, `-n` lines) |
| `coop shell <name>` | SSH into container |
| `coop exec <name> <cmd>` | Run command in container |

### Mounts

| Command | Description |
|---------|-------------|
| `coop mount add <container> <path>` | Mount host directory (`--readonly`, `--force`) |
| `coop mount remove <container> <name>` | Remove mount |
| `coop mount list [container]` | List mounts (all containers if omitted) |

Mount listing uses visual indicators: `<--->` for read-write (bidirectional), `--->` for read-only (one-way). Protected paths require `--force` with interactive authorization.

### Snapshots

| Command | Description |
|---------|-------------|
| `coop snapshot create <container> <name>` | Create snapshot |
| `coop snapshot restore <container> <name>` | Restore to snapshot |
| `coop snapshot list <container>` | List snapshots |
| `coop snapshot delete <container> <name>` | Delete snapshot |

### Images & VM

| Command | Description |
|---------|-------------|
| `coop image build` | Build base image (~10 min) |
| `coop image list` | List local images |
| `coop vm status` | Show VM status (macOS only) |
| `coop vm start/stop/shell` | Manage VM |

## Architecture

Coop talks to Incus over its Unix socket. On macOS, Colima exposes this at `~/.colima/incus/sock`. Each container gets an `agent-sandbox` profile with CPU/memory limits, nesting enabled, and optional workspace mounts. UID mapping ensures files created inside containers appear owned by your host user.

The base image uses Ubuntu 22.04 cloud variant with the default ubuntu user reassigned to UID 2000, avoiding collision with the agent user at UID 1000.

## Configuration

Settings: `~/.config/coop/settings.json`

Environment overrides:
- `COOP_CONFIG_DIR`, `COOP_DATA_DIR`, `COOP_CACHE_DIR` — relocate directories
- `COOP_DEFAULT_IMAGE` — change base image
- `COOP_VM_BACKEND` — force colima or lima

Logs rotate automatically in `~/.local/share/coop/logs/`.

## Security

- **Isolation**: Containers run as unprivileged user with no host access by default
- **Protected paths**: `~/.ssh`, `~/Library`, `/System`, `/usr` blocked from mounting
- **Authorization**: Protected mounts require interactive 6-digit code (15s expiry, macOS notification)
- **Lock/Unlock**: Freeze running containers to pause agent activity instantly
