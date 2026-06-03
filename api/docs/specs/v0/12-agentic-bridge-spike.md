# Crowbar Backend — Agentic Bridge

> **Status:** ⚠️ **PENDING SPIKE — NOT YET DESIGNED**
> **Date:** 2026-06-03
> **Depends on:** `00-architecture-and-domain.md`, `01-chat-lifecycle.md`,
> `03-realtime-websockets.md`
> **Scope:** This document is **not a design**. It is a placeholder that records
> the goal, the constraints, the open questions, and everything decided so far,
> so the spike can start from a known baseline. **No interfaces are locked here.**

---

## 1. Why This Is a Spike

The Agentic Bridge wraps the **JSON output of TUI agent CLIs** and maps it onto
Crowbar's own domain entities. We must support a **common interface** across
multiple, independently-designed agent CLIs:

- Claude Code
- Codex
- Cursor CLI
- Gemini CLI
- OpenCode
- (and others over time)

Each speaks its **own** streaming JSON/event format, with different framing for
text deltas, tool calls, tool results, todo/plan updates, and lifecycle. **Before
committing to any interface, we must reverse-engineer these protocols** and find
the common shape. Locking a domain model prematurely would almost certainly be
wrong. Hence: **spike first, design second.**

---

## 2. Goal

A pluggable subsystem that, for any supported agent CLI:

1. **Spawns** the CLI as a subprocess in a workspace.
2. **Reads** its streaming JSON output.
3. **Maps** that output to Crowbar domain entities (turns, tool calls, widgets,
   plan/TODO items, …).
4. **Drives** the `AgentRun` lifecycle and, through it, the live chat status.
5. Is **pluggable via the addons system** — each agent CLI is an addon declaring
   how to spawn it and how to parse its output. New agents = new addons, no core
   changes.

```
agent CLI subprocess  ──JSON stream──►  Bridge addon (per CLI)
                                            │ normalize
                                            ▼
                                     common ChatFrame / domain events
                                            │
                                            ▼
                              AgentRun (Asynx) + conversation content store
                                            │
                                            ▼
                                  WS /v0/ws/chats/:chatId/stream
```

---

## 3. What Is Already Decided (baseline for the spike)

These are settled in the approved specs and the spike must respect them:

- **Chat lifecycle** (`01-chat-lifecycle.md`) is fixed: Chat is an Asynx
  aggregate; status is `idle ↔ agent-running`; the aggregate reflects AgentRun
  state via `AgentRunStarted` / `AgentRunCompleted` commands issued by the
  AgentRun subscription. **The bridge must drive the AgentRun, not the Chat
  directly.**
- **AgentRun aggregate** (`00` §5.5, §6.2): `pending → running → done | error |
  interrupted`, with crash recovery (`running` → `error` on restart).
- **Status projection** (`03` §2 Class A, `01` §5): chat sidebar spinner is
  driven entirely by AgentRun → Chat → hub → `/v0/ws/chats?wsId=`.
- **Agents run their own shell** (`06-terminal-pty.md` §7): the agent's
  `run_command` uses the **agent's own** terminal/shell, **not** Crowbar's PTY
  subsystem. The bridge owns whatever the agent does with commands.
- **The content stream endpoint exists** but its frames are post-spike:
  `WS /v0/ws/chats/:chatId/stream`, namespace `chatId` (`02` §3, `03` §3).

---

## 4. Open Questions (the spike must answer these)

### 4.1 Conversation content storage — **DEFERRED**

Where and how do we persist what happens *inside* a chat? Candidates and their
natures were discussed but **not decided**:

- **Turns** (user/agent messages) — append-only log.
- **Tool calls** — have a lifecycle (`pending → done | error`).
- **Widgets** (Excalidraw / Mermaid / code) — append-only, embedded in a turn.
- **Agent TODO / plan items** — *unclear what these even are yet* (see 4.2).

GORM vs Asynx for each is **open**. (An earlier draft proposed GORM tables; that
proposal is **withdrawn** and must be re-decided after the spike — do not treat
it as settled.)

### 4.2 What is the agent "TODO list"?

Unresolved. Could be:
- an agent's internal task plan (Claude-Code-style step list), or
- a shared workspace task board, or
- something else.

Each agent CLI may express this differently — part of what the reverse
engineering must clarify before we model it.

### 4.3 Streaming wire protocol (`ChatFrame`)

The UX spec (§5, §28) sketches a frame shape (`text` / `widget` / `tool_call` /
`done`), but the **real** frames must be derived from the actual CLI outputs and
normalized into one common protocol. Not locked.

### 4.4 Fork content materialization

`01-chat-lifecycle.md` §4 fixes the *aggregate* relationship (`parentId` +
`fromTurnId`) but explicitly defers **how inherited context is materialized**
(copy rows vs. parent-pointer vs. replay). Depends on 4.1.

### 4.5 Message-send + run endpoints

The endpoint(s) to send a user message and start an agent run are **not**
specified yet (only the chat *lifecycle* CRUD is). They belong to this spike.

### 4.6 AI-generated branch description

`09-branch-review.md` §5: the "About" tab description and PR pre-fill text are
agent-generated and therefore depend on this bridge. Deferred until it exists.

### 4.7 Addon contract

The shape of a bridge addon — how it declares spawn config, parses output, and
maps to the common frames — is the **core deliverable** of the post-spike design.

---

## 5. Suggested Spike Approach (non-binding)

1. **Capture real output** from each target CLI (Claude Code, Codex, Cursor CLI,
   Gemini CLI, OpenCode) on identical tasks; record raw JSON streams.
2. **Diff the protocols** — catalog frame types, tool-call framing, plan/TODO
   representation, lifecycle signals, error reporting.
3. **Find the common denominator** → a candidate normalized `ChatFrame` set and
   a candidate domain mapping.
4. **Prototype one addon** (likely Claude Code) end-to-end against the existing
   AgentRun aggregate and the `/v0/ws/chats/:chatId/stream` endpoint.
5. **Validate** the common interface against a second CLI before locking it.
6. Only then write the real design spec(s) and re-open 4.1–4.7.

---

## 6. Dependencies Map

```
Agentic Bridge (this spike)
  ├─ consumes: AgentRun aggregate (00 §5.5)         [exists]
  ├─ drives:   Chat status via AgentRun (01 §5)      [exists]
  ├─ emits to: WS /v0/ws/chats/:chatId/stream (03)   [endpoint exists, frames TBD]
  ├─ defines:  conversation content storage (4.1)    [DEFERRED]
  ├─ defines:  message-send / run endpoints (4.5)    [DEFERRED]
  ├─ unblocks: AI branch description (09 §5)          [DEFERRED]
  └─ plugs into: addons system                        [contract TBD — core deliverable]
```

---

## 7. Status

**Do not implement against this document.** It exists to (a) prevent premature
interface decisions, (b) record the baseline the approved specs already fix, and
(c) enumerate exactly what the spike must resolve. When the spike concludes, it
is replaced by one or more real design specs.
