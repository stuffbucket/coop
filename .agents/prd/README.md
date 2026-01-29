# Agent Sandbox Manager - PRD Index

**Overview:** Cross-platform Go tool to spin up/down Incus containers with guest agents reporting to a central controller.

---

## Phase Summary

| Phase | Focus | Timeline | Dependencies |
|-------|-------|----------|--------------|
| [Phase 1](PHASE_1_CORE_INFRASTRUCTURE.md) | Core Infrastructure | Week 1 | None |
| [Phase 2](PHASE_2_GUEST_AGENT.md) | Guest Agent | Week 2 | Phase 1 |
| [Phase 3](PHASE_3_SANDBOX_MANAGER.md) | Sandbox Manager | Week 3 | Phase 1, 2 |
| [Phase 4](PHASE_4_CLI_AND_POLISH.md) | CLI & Polish | Week 4 | Phase 1, 2, 3 |

## Supporting Specifications

| Document | Purpose |
|----------|--------|
| [Base Image Specification](BASE_IMAGE_SPECIFICATION.md) | Container image with full AI agent toolchain |

---

## Semantic Boundaries Principles Applied

Each PRD enforces:

1. **Impossible States** — Invariants that must never be violated, with enforcement strategies
2. **Boundary Conditions** — Explicit behavior at system edges (∂S ≠ ∅)
3. **Interface Anacoluthon Detection** — Watch for protocol discontinuities and semantic drift

---

## Architecture at a Glance

```
┌─────────────────────────────────────┐
│         CLI (Phase 4)               │
│   build | deploy | status | exec    │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│      SandboxManager (Phase 3)       │
│   Phase orchestration, registry,    │
│   status aggregation, exec routing  │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│      Incus Wrapper (Phase 1)        │
│   Platform detection, container     │
│   lifecycle, socket management      │
└──────────────┬──────────────────────┘
               │
    ┌──────────┴───────────┐
    ▼                      ▼
[Container 1]         [Container N]
    │                      │
    ▼                      ▼
[Guest Agent]         [Guest Agent]
 (Phase 2)             (Phase 2)
```

---

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| HTTP-based guest agent | Language-agnostic, extensible, debuggable |
| Phased container startup | Dependency ordering, parallelizable per phase |
| Platform abstraction at socket level | Incus API identical everywhere; setup varies |
| Single base image | Consistent state, faster startup |
| Stateless agents | Ephemeral containers, central state in controller |

---

## Cross-Phase Invariants

These invariants span multiple phases and must be maintained throughout:

| Invariant | Enforcement |
|-----------|-------------|
| Container ID format consistent | `agent-{phase}-{index:03d}` enforced at creation |
| Agent port fixed | 8888 hardcoded; configurable override in Phase 4 |
| Status always queryable | Never block status on startup operations |
| Cleanup always possible | StopAll works regardless of partial failure states |

---

## Risk Register

| Risk | Phases Affected | Mitigation |
|------|-----------------|------------|
| Lima socket path variability | 1 | Probe multiple known paths |
| Agent /proc unavailability | 2 | Graceful fallback to zeroed metrics |
| Phase dependency cycles | 3 | Cycle detection at config validation |
| Signal handling complexity | 4 | Use proven context cancellation patterns |

---

## Links

- [Architecture Document](../AGENT_SANDBOX_ARCHITECTURE.md)
- [MVP Specification](../AGENT_SANDBOX_MVP.md)
- [Base Image Specification](BASE_IMAGE_SPECIFICATION.md)
