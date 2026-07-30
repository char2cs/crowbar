# Crowbar — Implementation Reference

This document details the technical architecture, stack decisions, and implementation strategy for Crowbar.

---

## Repository Structure

```
char2cs/crowbar                  # Main monorepo (AGPL-3.0-only)
├── api/                         # Go backend
├── web/                         # React frontend
├── flows/                       # Default YAML flow definitions
├── memory/                      # Memory schemas and defaults
├── docs/                        # This folder
├── docker/                      # Agent container definitions
└── desktop/          		 # Tauri wrapper (unites the front and back using Unix sockets, meant to be distribute as a app executable)
```

---

## Architecture Overview

```
crowbar/desktop (Tauri)
    └── bundles → crowbar binary + crowbar web

crowbar/api (Go)
    ├── Flow Engine
    ├── Agent Runner
    ├── Memory Layer
    ├── AI Provider Bridge
    └── REST + WebSocket API
            ↓
crowbar/web (React)
    └── Connects to wherever api is running
        (local binary or remote instance)
```

---

## Backend — crowbar/api

### Language & Runtime
- **Go** — single binary, runs anywhere, zero runtime dependencies
- Deployable locally via Tauri wrapper or remotely as a standalone server

### Event Sourcing — Asynx
- **[Asynx](https://github.com/char2cs/asynx)** — Event sourcing + CQRS framework (Go)
- Every phase transition, agent action, review comment, and memory entry is an event
- Full audit log and state replay out of the box
- Aggregate-per-project model

#### Core Aggregates

```go
// Project — the top level aggregate
type Project struct {
    ID          string
    WorkspaceID string    // multi-tenancy ready from day one
    Name        string
    CurrentPhase Phase
    FlowFile    string
}

// Workspace — scopes projects for future multi-user support
type Workspace struct {
    ID    string
    Owner string
}
```

#### Core Commands & Events

```
CreateProject       → ProjectCreated
StartBrainstorm     → BrainstormStarted
GenerateSpec        → SpecGenerated
StartImplementation → ImplementationStarted
SpawnAgent          → AgentSpawned
CompleteAgent       → AgentCompleted
AddReviewComment    → ReviewCommentAdded
ExtractMemory       → MemoryEntryCreated
CompletePhase       → PhaseCompleted
```

### Event Store
- **SQLite** (local, zero infra) via Asynx's pluggable Store interface
- **PostgreSQL** (remote deployments) — same interface, swap the driver
- Asynx enforces `(aggregateID, version)` uniqueness atomically

### Memory Layer

#### Storage
- **LanceDB** — embedded vector database, no server required, Rust native
- Each memory entry is embedded on write
- At review time, the PR diff is embedded and used to query top-K relevant entries
- Only relevant entries are injected into the reviewer agent — not the full memory file

#### Memory Entry Schema (Markdown + metadata)

```markdown
## Always use domain language for variable names

**Strength:** high
**Observed:** 7
**Last updated:** 2024-01-15
**Projects:** payments, auth

Agents should use bounded context language from the relevant
service domain. Never use generic names like `data`, `result`,
`response` when domain terms exist.

**Examples:**
- `userData` → `buyerProfile`
- `response` → `paymentConfirmation`
```

#### Memory Hierarchy

```
memory/
├── global.md              # Principles promoted across all projects
├── projects/
│   └── {project-id}/
│       ├── philosophy.md  # Architectural decisions
│       ├── patterns.md    # Recurring patterns
│       └── antipatterns.md
└── skills/
    └── {skill-name}.md    # Extracted reusable skills
```

#### Memory Subagent Behavior
- Triggered on every `ReviewCommentAdded` event via Asynx projection
- Reads existing relevant memory entries
- Rewrites (not appends) the relevant section with new signal folded in
- Tracks observation frequency — repeated signals are promoted up the hierarchy
- Projection failures are safe — events remain durable, replay on recovery

### AI Provider Bridge

```go
type Provider interface {
    Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
    Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error)
}
```

#### Supported Providers
- **Anthropic** (native SDK, day one)
- **OpenAI-compatible** (covers OpenAI, Gemini, Groq, Mistral, Together)
- **Ollama** (local models, air-gapped deployments)

Provider is configurable per-step in the flow YAML:

```yaml
steps:
  - name: ai-review
    agent: reviewer
    provider: anthropic
    model: claude-sonnet-4-20250514
```

### Agent Isolation
- Each implementation agent runs in its own **Docker container**
- Each agent gets its own **Git worktree** — no branch conflicts between parallel agents
- Containers are spun up at phase start and torn down on completion
- Tauri shell plugin handles Docker lifecycle on desktop

### API Layer
- **REST** — project management, flow control, memory inspection
- **WebSocket** — real-time agent status, streaming output, phase transitions
- Designed as if auth exists — every request carries a user/workspace context, hardcoded to single user today

---

## Flow Engine — crowbar/flows

Flows are YAML files. Every step, every agent, every prompt lives here — not in code.

### Flow File Schema

```yaml
flow: dev-workflow
version: 1
description: Standard development workflow with memory-backed review

steps:
  - name: brainstorm
    phase: brainstorm
    agent: orchestrator
    provider: anthropic
    model: claude-sonnet-4-20250514
    prompt: |
      You are a senior architect working alongside the developer.
      Help brainstorm the feature, ask clarifying questions, and
      shape the direction toward a concrete spec.

  - name: spec
    phase: spec
    agent: orchestrator
    depends_on: brainstorm
    prompt: |
      Given the brainstorm, produce a structured specification.
      Include: objectives, scope, technical approach, open questions.

  - name: implement
    phase: implementation
    agent: implementer
    depends_on: spec
    parallel: true
    prompt: |
      Implement the spec. Follow the patterns in the injected
      memory context. One agent per logical unit of work.

  - name: ai-review
    phase: ai_review
    agent: reviewer
    depends_on: implement
    memory_inject: true
    prompt: |
      Review the implementation against the injected memory context.
      Flag anything that violates established patterns or philosophy.

  - name: memory-extraction
    phase: improvement
    agent: memory
    trigger: on_human_review_comment
    prompt: |
      Given this human review comment, extract the generalizable
      principle. Fold it into the existing memory file. Do not append —
      rewrite the relevant section with the new signal integrated.

improvements:
  - step: ai-review
    agent: memory
    trigger: on_human_review_comment
```

### Flow Distribution
- Default flows ship with Crowbar in `crowbar/flows/`
- Community flows are distributable as Quiver packages
- No marketplace needed — Quiver handles discovery and installation

---

## Frontend — crowbar/web

### Stack
- **React + TypeScript**
- **Tailwind CSS** — utility-first, fast to build
- **shadcn/ui** — component library
- **Zustand** — lightweight state management
- **WebSocket** — real-time agent status from the Go backend

### Design Reference
GitHub's UI — calm, async-friendly, clear status, clear ownership, clear next action.

### Core Screens
- **Brainstorm** — chat interface with the orchestrator
- **Spec** — generated spec, editable before implementation starts
- **Implementation** — agent status board, per-agent output, real-time updates
- **Review** — diff view, comment interface, structured feedback
- **Memory Inspector** — view, edit, and correct memory entries per project
- **Projects** — project list, cognitive zone configuration

---

## Desktop Wrapper — crowbar-desktop

### Stack
- **Tauri v2** — Rust shell, WebView frontend
- Bundles the Go binary and React web app into a single installable
- Manages local instance lifecycle — starts/stops the Go server
- Uses Tauri `shell` plugin for Docker orchestration
- Uses Tauri `fs` plugin for local file access

### Distribution
- Packaged and distributed through **Quiver**
- Quiver handles Docker installation as a dependency if not present

---

## Multi-Tenancy Design (Future)

The data model is multi-tenancy ready from day one:

- Every record is scoped to a `WorkspaceID`
- Every API request carries a user context (hardcoded single user today)
- OAuth, user management, and RBAC are additive — no data model refactoring needed

### Planned Auth Providers
- GitHub OAuth (primary — developer audience)
- Google OAuth

---

## Deployment Targets

| Mode | Stack | Auth |
|------|-------|------|
| Local (Tauri) | SQLite + LanceDB local | None |
| Self-hosted | PostgreSQL + LanceDB | OAuth (future) |
| Hosted (future) | Managed PostgreSQL | OAuth |

---

## Dependencies Summary

| Dependency | Purpose | Where |
|------------|---------|-------|
| [Asynx](https://github.com/char2cs/asynx) | Event sourcing + CQRS | api/ |
| LanceDB | Vector store for memory retrieval | api/ |
| SQLite | Local event store | api/ |
| PostgreSQL | Remote event store | api/ |
| Anthropic Go SDK | AI provider (primary) | api/ |
| Docker | Agent isolation | api/ |
| React | Frontend framework | web/ |
| Tailwind CSS | Styling | web/ |
| shadcn/ui | Components | web/ |
| Zustand | State management | web/ |
| Tauri v2 | Desktop wrapper | desktop/ |

---

## License

AGPL-3.0-only. Free to use, modify, and self-host; modifications made available to others over a network must be published under the same licence (AGPL § 13). See LICENSE and LICENSING.md.
