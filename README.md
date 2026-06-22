EDITOR_RERUN_MARKER_88
# Crowbar

Crowbar is a self-improving agentic development platform. It orchestrates AI agents through structured development workflows, learns from human feedback over time, and progressively reduces the need for human review by encoding a developer's — or a team's — taste, philosophy, and patterns into every future agent run.

---

TEST # This is a change of a child branch

## The Problem

Modern AI coding tools are stateless. Every agent run starts from zero. The AI reviewer today catches the same things it caught yesterday, and misses the same things too. Human developers end up reviewing the same categories of mistakes over and over, with no mechanism for the AI to learn from that signal.

At scale — across multiple services, multiple contributors, multiple codebases — this compounds. There is no shared cognitive context. There is no memory of what the team has already decided. Every agent is a new hire with no onboarding.

---

## The Solution

Crowbar introduces a **self-improving review loop**:

1. A human reviews agent-generated code and leaves comments
2. A background agent reads those comments and extrapolates generalizable principles
3. Those principles are written to a per-project memory file
4. Every future AI review is injected with the accumulated memory
5. Over time, the AI reviewer starts catching what the human catches — in the human's voice

The human stays in the loop. But the loop gets shorter with every iteration.

---

## Core Concepts

### Phases

Every project in Crowbar moves through structured phases:

- **Brainstorming** — The orchestrator agent works alongside the developer to explore ideas and shape direction
- **Spec Writing** — The orchestrator generates a structured specification from the brainstorm
- **Implementation** — Subagents execute the spec, each isolated in their own environment
- **AI Review** — A reviewer agent evaluates the implementation against the memory file
- **Human Review** — The developer reviews, comments, and closes the loop

### Flow Engine

Phases are defined in YAML. Every step, every agent, every prompt lives in a flow file — not in code. This means:

- Flows are versioned, shareable, and distributable
- Prompts can be iterated without touching the codebase
- Anyone can write a new flow for a new use case
- Improvement agents are wired into steps as first-class citizens

### Memory Layer

Each project maintains a memory file — a structured, human-readable record of the developer's coding philosophy, architectural decisions, recurring patterns, and explicit antipatterns. It is:

- Written by the memory subagent from human review signal
- Queried semantically at review time — only relevant entries are injected
- Fully inspectable and editable by the developer
- Scoped per project, with global principles promoted over time

### Agent Isolation

Each implementation agent runs in its own Docker container against its own Git worktree. Agents cannot affect each other's environments. Containers are spun up for the duration of a phase and torn down cleanly on completion.

### Cognitive Zones

Projects can be grouped into cognitive zones — scoped contexts that give the orchestrator awareness of multiple codebases simultaneously. This enables brainstorming features that span services, with the full topology of a system as context.

---

## Deployment Modes

### Local

A single binary runs the full Crowbar stack locally. Distributed through Quiver. Ideal for solo developers who want a local-first experience on their development machine.

### Self-Hosted

The Crowbar backend can be deployed on a remote server. Team members connect through the web UI with no local installation required. The project maintainer hosts one instance, controls the AI tokens, and every contributor works within the same environment — the same memory files, the same flows, the same coding philosophy.

### Hosted

A managed hosted tier for teams who do not want to self-host.

---

## Who It's For

- **Solo developers** who want their agents to sound like them
- **Open source maintainers** who want contributors to work within established patterns without manual onboarding
- **Engineering teams** who want shared agentic infrastructure without each developer managing their own AI setup

---

## The Compounding Effect

The longer Crowbar is used on a project, the more the memory layer reflects the real decisions made on that codebase. New contributors inherit the project's philosophy immediately. The AI reviewer catches more with every human review cycle. The human reviews less over time.

This is the core thesis: **a development workflow that gets measurably better the more you use it.**

---

## Relationship to Quiver

Crowbar is distributed through [Quiver](https://github.com/rabbytesoftware/quiver.desktop) — a multi-platform package manager built by the same author. Quiver handles installation, dependency management, and environment setup for Crowbar's local deployment. Default flow files can be distributed as Quiver packages, enabling a community-driven ecosystem of shareable workflows.

---

## Naming

Crowbar follows the Valve-universe naming schema established across the rabbytesoftware ecosystem. The crowbar is Gordon Freeman's iconic tool — simple, brutally effective, works in any environment, and is the first thing you reach for. That's the right metaphor for a development tool.
CROWBAR_RERUN_MARKER_77
