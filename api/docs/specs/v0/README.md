# Crowbar Backend — v0 Design Specs

> **Date:** 2026-06-03
> Complete backend design to serve the frontend capabilities in
> `docs/v0/ux-capabilities.md`. A **full rewrite** keeping the existing
> scaffold's architecture (layered containers, Asynx, GORM, topic-scoped
> WebSockets — modeled on `quiver.core`) and replacing its domain model.
>
> These are **design specs only**. Implementation happens in separate, focused
> sessions. Each spec is meant to be handed to one such session.

---

## Global Decisions

- **Local, single-user, no auth** desktop tool.
- **Architecture:** `quiver.core` patterns — hierarchical DI containers,
  `Broadcaster[T]`, `dispatch()` REST/WS dual-serve, Asynx event sourcing,
  hub projections.
- **All IDs are UUIDs.**
- **All routes prefixed `/v0/`** (REST + WS); uniform response envelope.
- **Storage:** Asynx for state machines (Workspace, AgentRun, ReviewThread,
  Chat); GORM for plain CRUD (Project, Repository, TerminalProfile); in-memory
  for TerminalSession.
- **External tools shelled out / required:** `git` (intrinsic), `gh`/`glab`
  (provider, graceful-disable), language servers (LSP, graceful-absence).
  Search is **pure-Go** (no `rg`).

---

## Index

| # | Spec | Status |
|---|------|--------|
| 00 | [Architecture & Domain Model](./00-architecture-and-domain.md) | Approved |
| 01 | [Chat Lifecycle](./01-chat-lifecycle.md) | Approved |
| 02 | [API Surface](./02-api-surface.md) | Approved |
| 03 | [Real-time / WebSocket Topology](./03-realtime-websockets.md) | Approved |
| 04 | [Git Subsystem](./04-git-subsystem.md) | Approved |
| 05 | [Filesystem & File Watcher](./05-filesystem-and-watcher.md) | Approved |
| 06 | [Terminal / PTY](./06-terminal-pty.md) | Approved |
| 07 | [Workspace & Worktree Hierarchy](./07-workspace-worktree-hierarchy.md) | Approved |
| 08 | [Git Provider Engine](./08-git-provider-engine.md) | Approved |
| 09 | [Branch Review](./09-branch-review.md) | Approved |
| 10 | [Language Server Protocol (LSP)](./10-lsp.md) | Approved |
| 11 | [Global Search](./11-global-search.md) | Approved |
| 12 | [Agentic Bridge](./12-agentic-bridge-spike.md) | ⚠️ Pending Spike |

---

## Signature Feature

The **workspace/worktree hierarchy** (07) + **git provider engine** (08) together
implement Crowbar's most powerful capability: local child→parent branch merges
that the git platform never sees, child re-parenting via `rebase --onto`, and
protected-branch awareness where the only path into a protected branch is a real
provider PR (which Crowbar reads but never creates).

## Deferred to the Agentic Bridge Spike (12)

Everything about what happens *inside* a chat — conversation content storage
(turns, tool calls, widgets, agent TODO lists), the streaming wire protocol,
fork content materialization, message-send endpoints, and the AI-generated branch
description (09 §5) — is **deliberately not designed yet**. It depends on
reverse-engineering multiple agent CLIs (Claude Code, Codex, Cursor CLI, Gemini
CLI, OpenCode) to find a common interface, exposed as pluggable addons.
