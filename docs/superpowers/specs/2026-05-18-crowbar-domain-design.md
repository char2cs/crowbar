# Crowbar — Domain & Feature Spec

**Date:** 2026-05-18
**Status:** Approved

---

## Overview

Crowbar is a general-purpose **agent orchestration platform** for software development teams. It runs AI agents through user-defined, version-controlled workflows (Flows), accumulates learned context over time (Memory), and progressively reduces the need for human review by encoding developer taste into every future agent run.

The feature development flow described in this spec is the **first built-in Flow** — not the only one. All domain entities and platform primitives are designed to be flow-agnostic.

---

## v0 Implementation Strategy

The Flow schema defined in this document is the authoritative design for how Crowbar orchestrates agent work. However, **the YAML-based flow engine is planned for a future version** — v0 ships the feature development flow hardcoded in Go.

**What this means for v0:**
- The Flow YAML schema is fully implemented — parser, translator, two-phase validator, and generic state machine engine ship in v0
- The feature development flow is defined in YAML and executed by the engine — not hardcoded
- The engine is built by porting the Manifold engine pattern from `quiver.core` — translator, ruleset validators, wizard execution loop, event drain, and reactions are all direct ports adapted to Crowbar's domain
- Reference: `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/engine/manifold/` and `/Users/char2cs/Projects/Rabbyte/quiver.core/internal/engine/wizard/`

**What the backend must be architected for:**
The Go code must treat the flow as data, not as logic. Concretely:
- States, transitions, and improvement agent triggers are represented as structs/interfaces — not as `switch` statements or hardcoded `if` chains
- The state machine engine (evaluate event → find matching transition → advance state) is a generic function that takes a flow definition as input
- Improvement agent trigger evaluation (event filter expressions) is a separate, pluggable component
- The `context` injection pipeline (file pointers, `all_runs`, per-step artifacts) is driven by a declarative config struct, not hardcoded per-agent

This ensures that when the YAML engine is built, it produces the same structs the hardcoded flow already uses — making the migration a parser addition, not a rewrite.

---

## Domain Entities

### Project

The top-level aggregate. A named collection of repositories with shared context across all of them.

| Field | Type | Notes |
|-------|------|-------|
| `id` | uuid | |
| `name` | string | |
| `description` | text | |
| `created_at` | timestamp | |

Relations: has many **Repositories**, has many **Memories** (project-scoped).

---

### Repository

A single code repository added by local path. Has its own isolated memory and task history.

| Field | Type | Notes |
|-------|------|-------|
| `id` | uuid | |
| `project_id` | fk → Project | |
| `name` | string | |
| `local_path` | string | Absolute path to the repo on disk |
| `default_flow` | string | Path to the default Flow YAML file |
| `created_at` | timestamp | |

Relations: belongs to **Project**, has many **Tasks**, has many **Memories** (repo-scoped).

---

### Task

One execution of a Flow on a Repository. Each Task gets its own dedicated git branch and worktree. The branch name is always provided by the user at creation time — Crowbar never generates it.

| Field | Type | Notes |
|-------|------|-------|
| `id` | uuid | |
| `repo_id` | fk → Repository | |
| `title` | string | |
| `flow_path` | string | Path to the Flow YAML file used |
| `current_state` | string | Name of the active state in the Flow |
| `status` | enum | `active`, `paused`, `complete` |
| `branch_name` | string | Git branch for this Task — required user input at creation |
| `base_branch` | string | Branch the Task branches off from — required user input, repo default branch recommended |
| `start_sha` | string | HEAD of `base_branch` at Task creation; used as the base for Task-level diffs |
| `end_sha` | string | nullable; Branch HEAD when the Task reaches a terminal state |
| `visited_states` | string[] | Ordered list of states the Task has passed through |
| `created_at` | timestamp | |

Relations: has one **Worktree**, has many **AgentRuns**, **KanbanItems**, **ReviewThreads**.

---

### Worktree

A git worktree managed by Crowbar. The relationship is **1:1 with Task** — each Task gets its own dedicated worktree created on demand when the Task starts. Crowbar runs `git worktree add` at Task creation and `git worktree remove` when the Task is cleaned up. Users never manage worktrees directly.

Because each Task has its own worktree, parallel Tasks work without conflict — they operate on separate branches in separate working directories.

| Field | Type | Notes |
|-------|------|-------|
| `id` | uuid | |
| `task_id` | fk → Task | 1:1 — one worktree per Task |
| `repo_id` | fk → Repository | |
| `path` | string | Absolute path to the worktree on disk |
| `branch` | string | Git branch checked out in this worktree — mirrors `Task.branch_name` |
| `created_at` | timestamp | |

Relations: belongs to **Task**, has many **AgentRuns**.

---

### AgentRun

One agent execution within a Task state. Each AgentRun gets its own Docker container and is assigned a Worktree from the pool by Crowbar.

| Field | Type | Notes |
|-------|------|-------|
| `id` | uuid | |
| `task_id` | fk → Task | |
| `worktree_id` | fk → Worktree | the Task's dedicated worktree |
| `state_name` | string | Flow state this run belongs to |
| `intelligence` | string | Intelligence tier declared in the Flow (`low`, `medium`, `high`, `ultrahigh`) |
| `model` | string | Model name resolved from user config at the time of the run |
| `container_id` | string | Docker container ID |
| `status` | enum | `running`, `complete`, `failed`, `interrupted` |
| `output` | text | nullable; the handoff summary the agent passed as the second argument to `crowbar_signal()`. Injected into subsequent states' context in order. |
| `started_at` | timestamp | |
| `completed_at` | timestamp | nullable |

When an AgentRun completes, Crowbar writes two artifacts to `{data_dir}/runs/{agent_run_id}/`:

- `events.jsonl` — the full ACP event stream, written asynchronously via a buffered channel + logger goroutine as events arrive
- `diff.patch` — the git diff between the task branch and base, generated at completion

These artifacts are never injected directly into any agent's context window. Their file paths are passed as pointers — agents use `fs.read` to pull exactly what they need.

**Drain goroutine** — runs for the lifetime of the AgentRun. Receives `SessionUpdate` notifications from the ACP SDK and simultaneously:
1. Writes the raw event to `events.jsonl`
2. Fans out to `Broadcaster[ConversationMessage]` for real-time frontend streaming
3. On agent turn end: assembles the full message and writes a `ConversationMessage` to SQLite
4. On `crowbar_signal()` tool call: sends the corresponding asynx command to advance Task state

Pattern ported from `quiver.core/internal/app/repositories/runtime/internal/reactions.go` (`drainExecution`).

---

### KanbanItem

A unit of implementation work within a Task. Created by the agent in a state that declares `items: true`. Status values are free-form strings defined by the Flow YAML — Crowbar stores and displays them without interpreting them.

| Field | Type | Notes |
|-------|------|-------|
| `id` | uuid | |
| `task_id` | fk → Task | |
| `title` | string | |
| `status` | string | Free-form; valid values defined by the Flow |
| `agent_run_id` | fk → AgentRun | nullable; the AgentRun currently working on this item |
| `created_at` | timestamp | |
| `updated_at` | timestamp | |

---

### ConversationMessage

A single turn in a chat conversation within a Flow state. Stored in SQLite so the full conversation history survives tab closes, restarts, and state transitions. Both user messages and agent messages are stored here — user messages arrive via HTTP POST, agent messages are assembled from the ACP event stream by the drain goroutine.

| Field | Type | Notes |
|-------|------|-------|
| `id` | uuid | |
| `task_id` | fk → Task | |
| `agent_run_id` | fk → AgentRun | nullable; null for user messages posted before an AgentRun starts |
| `state_name` | string | Flow state this message belongs to |
| `role` | enum | `user` — typed by the human; `agent` — produced by the ACP agent |
| `type` | enum | `text` — prose message; `tool_call` — agent invoked a tool; `tool_result` — tool response |
| `content` | text | Full message content. For `tool_call`: JSON-encoded tool name + args. For `tool_result`: JSON-encoded result. |
| `created_at` | timestamp | |

Chat reload: `SELECT * FROM conversation_messages WHERE task_id = ? ORDER BY created_at`. Full conversation history across all states and all AgentRuns for a Task is the complete ordered list.

---

### ReviewThread

A conversation thread anchored to a specific file and line, opened during a review state. Modelled like a Slack thread — full back-and-forth between reviewers and the Implementation agent. Threads exist in two distinct phases corresponding to who initiated the review round.

| Field | Type | Notes |
|-------|------|-------|
| `id` | uuid | |
| `task_id` | fk → Task | |
| `kanban_item_id` | fk → KanbanItem | nullable; the item this thread is associated with |
| `file_path` | string | |
| `line_number` | int | |
| `phase` | enum | `ai_review` — opened by the AI reviewer improvement agent; `human_review` — opened by the human |
| `opened_by` | enum | `reviewer` or `human` |
| `status` | enum | `open`, `agreed`, `force_approved` |
| `created_at` | timestamp | |

Relations: has many **ReviewMessages**.

---

### ReviewMessage

A single message within a ReviewThread. Supports full multi-turn conversations between reviewers (human or AI) and the implementer agent.

| Field | Type | Notes |
|-------|------|-------|
| `id` | uuid | |
| `thread_id` | fk → ReviewThread | |
| `role` | enum | `human` — the human reviewer; `reviewer` — the AI reviewer improvement agent; `implementer` — the Implementation agent responding to the thread |
| `content` | text | |
| `created_at` | timestamp | |

---

### Memory

A learned principle or pattern, either extracted automatically by an Improvement Agent or manually written by the user. Stored in SQLite (source of truth, auditable) and indexed in LanceDB (vector embeddings for semantic retrieval).

| Field | Type | Notes |
|-------|------|-------|
| `id` | uuid | |
| `scope` | enum | `repo`, `project` |
| `repo_id` | fk → Repository | nullable; set when scope is `repo` |
| `project_id` | fk → Project | nullable; set when scope is `project` |
| `content` | text | The principle, written as a concise actionable rule |
| `type` | string | Free-form category label (e.g. "code-style", "debug-pattern") |
| `source` | string | Agent name that created it, or `"human"` if manually added |
| `strength_score` | int | Incremented each time the entry is reinforced by an agent |
| `task_id` | fk → Task | nullable; the Task session this was extracted from |
| `updated_at` | timestamp | |

The vector embedding is stored in LanceDB, keyed by `id`. All writes go through a single `MemoryService` that keeps SQLite and LanceDB in sync: write to SQLite first, compute embedding, upsert to LanceDB. User edits trigger a re-embed. User deletes remove from both stores.

Crowbar injects relevant Memory entries into every agent call automatically via semantic search — agents do not need to query it directly.

---

### Flow

**Not a database entity.** A YAML file on disk, referenced by `Task.flow_path` and `Repository.default_flow`. Lives in the repository, version-controlled alongside code. Defines the complete behavior of a Task: states, transitions, agent system prompts, tool access, UI declarations, and improvement agent triggers.

---

## The Flow Schema

Flows are state machines. Every decision about what happens next is encoded in the YAML — no AI orchestrator. The state machine engine reads the current state, listens for events (emitted by agents via `crowbar_signal()` or fired by Crowbar as system events), and deterministically moves to the next state.

### Top-level structure

```yaml
name: string
version: string
description: string

# Free-form status labels for KanbanItems in this Flow.
# Crowbar stores and displays these; it does not interpret them.
item_statuses:
  - string

states:
  - ...

improvement_agents:
  - ...
```

### State definition

```yaml
- name: string           # unique identifier within this Flow
  description: string    # optional, for human readers
  terminal: bool         # if true, reaching this state completes the Task
  ui: chat | kanban | diff | background
                         # The view foregrounded when this state is entered.
                         # ALL views (chat, kanban, diff, file explorer, git, terminal)
                         # remain accessible as tabs at all times — the user can switch
                         # freely regardless of the current state. When activity happens
                         # in a non-active view (a thread opens, a commit lands), that
                         # tab is badged to draw attention.
                         # `background` = no primary view; the task has no foreground UI.
  items: bool            # if true, this state creates and manages KanbanItems
  agent:
    intelligence: low | medium | high | ultrahigh
                         # resolved to a model + CLI via crowbar.yaml; see Agent Runtime Model
    tools:               # list of tools available to this agent
      - crowbar.signal
      - crowbar.create_item
      - crowbar.update_item_status
      - crowbar.get_items
      - crowbar.open_thread
      - crowbar.reply_thread
      - fs.read
      - fs.write
      - terminal
    system_prompt: |
      ...
  transitions:
    - to: state_name     # move Task to this state
      on: event_name     # when this event is signalled
    - emit: event_name   # emit an event without changing the main Task state
      on: event_name     # (used by concurrent improvement agents)
```

### Improvement agent definition

```yaml
- name: string
  description: string
  trigger:
    event: string        # system event or custom event name
    filter: string       # optional boolean expression against event payload fields.
                         # Operators: ==, !=, includes, not includes.
                         # Example: "new_status == 'ai_review'"
  scope: repo | project  # where extracted Memory entries are written
  context:               # optional — additional AgentRun artifacts to expose.
                         # Default: triggering step's events.jsonl + diff.patch
                         # are always available as file pointers. Declare additional
                         # steps to also expose their artifacts.
    - step: state_name   # expose this step's events.jsonl and diff.patch as readable paths
  agent:
    intelligence: low | medium | high | ultrahigh
    tools:
      - crowbar.write_memory
      - crowbar.open_thread
      - fs.read
      - ...
    system_prompt: |
      ...
```

### System events

Crowbar fires these automatically — improvement agents can subscribe to them:

| Event | Payload | Description |
|-------|---------|-------------|
| `item_status_changed` | `item_id`, `old_status`, `new_status` | Any KanbanItem status change |
| `thread_resolved` | `thread_id`, `resolution` (`agreed` or `force_approved`) | A ReviewThread is closed |
| `thread_reply_posted` | `thread_id`, `role`, `phase` | A new message was posted in a ReviewThread. Used to trigger improvement agents that respond to thread activity. |
| `task_complete` | `task_id`, `visited_states` | Task reaches a terminal state |

### Agent tool API (MCP)

Crowbar exposes these tools to agents via MCP. Agents call them like any Claude tool — no string parsing, no special output formats.

| Tool | Signature | Description |
|------|-----------|-------------|
| `crowbar_signal` | `(event: string, output?: string)` | Trigger a state transition or emit an event. `output` is a markdown summary of the step's decisions — stored on the AgentRun and injected into all subsequent states' context in order. |
| `crowbar_create_item` | `(title: string, description?: string) → item_id` | Create a KanbanItem |
| `crowbar_update_item_status` | `(item_id: string, status: string)` | Move an item to a new status |
| `crowbar_get_items` | `() → Item[]` | List all KanbanItems for this Task |
| `crowbar_open_thread` | `(file: string, line: int, content: string) → thread_id` | Open a ReviewThread. Crowbar automatically sets `phase` and `opened_by` based on which agent is calling. |
| `crowbar_reply_thread` | `(thread_id: string, content: string)` | Append a message to a thread. Role is set automatically from the calling agent's identity. |
| `crowbar_get_threads` | `(status?: string, phase?: string) → Thread[]` | List ReviewThreads for this Task. Filter by `status` (`open`, `agreed`, `force_approved`) and/or `phase` (`ai_review`, `human_review`). Used by the implementer to poll for threads that need a response. |
| `crowbar_resolve_thread` | `(thread_id: string, emoji?: string)` | Mark a thread `agreed`. Only callable by agents (human resolution is done via the UI). If `emoji` is provided (e.g. `"👍"`), the UI renders it as an emoji react bubble instead of a plain checkmark — used when the implementer agrees with a human concern without needing to elaborate. |
| `crowbar_write_memory` | `(content: string, type: string)` | Write a Memory entry (improvement agents only) |

---

## Reference Flow: Feature Development

This is the first Flow shipped with Crowbar. It illustrates all platform primitives.

```yaml
name: feature-development
version: "1.0"
description: Full feature development — brainstorm to reviewed implementation

item_statuses:
  - todo
  - implementing
  - done

states:
  - name: brainstorming
    ui: chat
    agent:
      intelligence: medium
      tools: [crowbar.signal, fs.read, terminal]
      system_prompt: |
        You are a senior engineer partnering with the developer to explore
        a feature idea. Ask clarifying questions, explore edge cases, and
        help arrive at a clear implementation intent.
        When aligned, call crowbar_signal("user_approved").
        If a bug is uncovered instead, call crowbar_signal("bug_identified").
    transitions:
      - to: spec
        on: user_approved
      - to: debugging
        on: bug_identified

  - name: spec
    ui: chat
    agent:
      intelligence: medium
      tools: [crowbar.signal, fs.read]
      system_prompt: |
        Based on the brainstorming conversation, produce a structured spec
        with acceptance criteria and a list of implementation tasks.
        Present it to the developer. On approval, call crowbar_signal("user_approved").
        If they want to revisit, call crowbar_signal("revision_requested").
    transitions:
      - to: implementation
        on: user_approved
      - to: brainstorming
        on: revision_requested

  - name: implementation
    ui: kanban
    items: true
    agent:
      intelligence: high
      tools: [crowbar.signal, crowbar.create_item, crowbar.update_item_status, crowbar.get_items, crowbar.get_threads, crowbar.reply_thread, fs.read, fs.write, terminal]
      system_prompt: |
        You may be entering this state for the first time (fresh implementation) or
        returning from AI review with open threads to address.

        On first entry: create an item for each task, implement them one at a time,
        marking each crowbar_update_item_status(item_id, "implementing") when starting
        and crowbar_update_item_status(item_id, "done") when complete.

        On re-entry after review: check open AI review threads with
        crowbar_get_threads(status="open", phase="ai_review"). Address each one, make
        the fix, and reply documenting what you changed using crowbar_reply_thread.

        When all items are "done" and no open threads remain, call
        crowbar_signal("implementation_complete").
    transitions:
      - to: ai_review
        on: implementation_complete
      - to: spec
        on: scope_changed

  - name: ai_review
    ui: diff
    agent:
      intelligence: high
      tools: [crowbar.signal, crowbar.open_thread, crowbar.get_threads, crowbar.resolve_thread, crowbar.reply_thread, fs.read]
      system_prompt: |
        Review the full diff against the spec, repository memory, and coding standards.

        First, load all existing threads with crowbar_get_threads() and read their full
        history. Do NOT re-raise issues that are already agreed or force-approved.

        For each prior thread: verify in the current diff whether the fix was correctly
        implemented. If fixed: call crowbar_resolve_thread(thread_id). If incomplete:
        reply with crowbar_reply_thread noting what is still missing.

        For any new issues not raised in prior rounds: call crowbar_open_thread(file, line, concern).

        When your review is complete:
        - No open threads: call crowbar_signal("review_passed")
        - Open threads remain: call crowbar_signal("review_failed")
    transitions:
      - to: implementation
        on: review_failed
      - to: human_review
        on: review_passed

  - name: human_review
    ui: diff
    # No agent. The human reviews at their own pace — no agent is running or polling.
    # The human reads the full diff, the ai_review thread history, and opens their own threads.
    # Transitions are fired by the human clicking UI buttons, not by an agent.
    # When the human clicks "Request Changes": task returns to implementation; the implementer
    # picks up all open human_review threads in bulk and addresses them.
    # When the human clicks "Approve": task completes.
    transitions:
      - to: implementation
        on: changes_requested
      - to: complete
        on: approved

  - name: debugging
    ui: chat
    agent:
      intelligence: high
      tools: [crowbar.signal, fs.read, fs.write, terminal]
      system_prompt: |
        Help the developer identify and resolve the bug. Call
        crowbar_signal("bug_resolved") when fixed. If investigation reveals
        broader scope, call crowbar_signal("scope_expanded").
    transitions:
      - to: implementation
        on: bug_resolved
      - to: brainstorming
        on: scope_expanded

  - name: complete
    terminal: true

improvement_agents:
  - name: review_learner
    description: Analyzes the completed review cycle after human approval and writes memories so all future agents learn from this experience
    trigger:
      event: task_complete
      filter: "visited_states includes 'ai_review'"
    scope: repo
    context:
      all_runs: true    # expose ALL AgentRun artifacts across every state and every run
                        # in this task — events.jsonl + diff.patch for each one.
                        # Needed because implementation and ai_review run multiple times;
                        # the learner needs the full chronological arc, not just one run.
    agent:
      intelligence: medium
      tools: [crowbar.get_threads, crowbar.write_memory, fs.read]
      system_prompt: |
        A task just completed the full review cycle and was approved by the human.
        You have access to the event logs and diffs for every step, plus the complete
        thread history across both the ai_review and human_review phases.

        Think like a senior engineer reflecting on what happened: what patterns emerged,
        what the AI reviewer flagged repeatedly, what the human caught that the AI missed,
        what coding habits caused friction, what approaches worked well.

        Extract each insight as a concise, actionable principle. Call
        crowbar_write_memory(content, type) for each one. These memories are injected
        into every future agent run — this is how the system gets smarter over time and
        tailors itself to the team's standards.

  - name: debug_pattern_extractor
    description: Captures reusable debugging patterns after debug sessions
    trigger:
      event: task_complete
      filter: "visited_states includes 'debugging'"
    scope: repo
    agent:
      intelligence: medium
      tools: [crowbar.write_memory]
      system_prompt: |
        A task with a debugging session just completed. Extract any patterns,
        root causes, or diagnostic steps worth remembering. Call
        crowbar_write_memory(content, type) to save each one.
```

---

## Key Capabilities

### Project & Repository Management

- Create a Project with a name and description
- Add a Repository to a Project by providing a local directory path
- View all repositories in a Project with their task counts
- Remove a repository from a Project

### Task Management

- Create a Task on a Repository by providing a title, branch name, base branch (repo default branch is pre-filled as the recommended value), and Flow YAML file (defaults to `Repository.default_flow`). Crowbar runs `git worktree add -b {branch_name} {path} {base_branch}` immediately and records `start_sha` from the base branch HEAD.
- View the Task list for a Repository; Tasks waiting for user input are prominently highlighted
- Interact with the current Task state through the appropriate UI (chat, kanban, or diff)
- View full Task history — which states were visited and in what order
- Archive a Task — worktree is removed from disk, branch is left untouched for the user to manage

**Task lifecycle controls:**

| Action | When available | Effect |
|--------|---------------|--------|
| **Stop agent** | AgentRun is `running` | Terminates the ACP session; marks AgentRun `interrupted`; Task moves to `paused` |
| **Retry** | AgentRun is `failed` or `interrupted` | Creates a new AgentRun for the same state with the same context injection; prior failed run's `output` is not carried forward |
| **Force transition** | Task is `paused` | User manually selects which state to move to, bypassing agent execution |
| **Resume** | Task is `paused` | Equivalent to Retry — creates a new AgentRun for the current state |
| **Skip to Human Review** | Task is in `ai_review` state | Escape hatch: stops the running reviewer AgentRun (`interrupted`) and immediately fires `review_passed`, advancing the task to `human_review`. Use when the AI reviewer is too strict or stuck. |
| **Archive** | Any non-running Task | Permanent; removes worktree from disk |

**How AgentRuns fail:** the ACP subprocess crashes, the Docker container dies, the MCP connection drops, or the agent exits without calling `crowbar_signal()`. All of these produce `status: failed`. User-initiated stops produce `status: interrupted`. Recovery is identical for both — Retry creates a fresh AgentRun for the same state.

### UI Navigation

The `ui` field in a state definition sets the *foregrounded* view when the task enters that state — it is not a lock. The user can switch freely between all available views at any time: chat, kanban, diff, file explorer, git, terminal. When activity happens in a non-active view (a thread opens on the diff, a commit lands in git, an item status changes in kanban), that tab is badged to surface the change without forcing navigation.

### Real-time Protocol

All real-time communication uses **WebSocket** following the same architecture as `quiver.core` — type-safe `Broadcaster[T]` per event type, dispatch pattern (single route checks `Upgrade` header), hub above API versions, non-blocking fan-out.

**Broadcasters:**

| Broadcaster | Event type | Triggered by |
|-------------|-----------|--------------|
| `Broadcaster[Task]` | Task state transitions, status changes | asynx `Task.*` subscription |
| `Broadcaster[AgentRun]` | Run started, completed, failed | asynx `AgentRun.*` subscription |
| `Broadcaster[ConversationMessage]` | Chat messages + agent text chunks | drain goroutine |
| `Broadcaster[KanbanItem]` | Item status changes | asynx `KanbanItem.*` subscription |
| `Broadcaster[ReviewThread]` | Thread opened, replied, resolved | asynx `ReviewThread.*` subscription |

**Agent text streaming:** The drain goroutine receives `SessionUpdate` notifications from the ACP SDK and fans them out immediately via `Broadcaster[ConversationMessage]` as partial chunks. The frontend assembles chunks into a message in the browser. When the agent turn ends, the drain goroutine writes the assembled full message to SQLite.

**User messages:** Sent via HTTP POST to `/v0/tasks/:id/messages`. Backend stores the `ConversationMessage`, then forwards to the ACP session via `conn.Prompt()` on the live session. No WebSocket upload path for chat — same pattern as Quiver's runtime execute method.

**PTY terminal:** Uses a separate WebSocket endpoint (`/v0/tasks/:id/terminal/:tab_id`). Binary frames carry raw PTY bytes server→client; text frames carry keystrokes client→server and resize control messages.

**Reference:** `quiver.core/internal/api/ws/` for broadcaster, client, and filter implementations to port directly.

### Chat UI

Real-time chat interface between the user and the agent. The agent can read the codebase and signal transitions. `ConversationMessage` records are loaded on mount and new messages arrive via the `Broadcaster[ConversationMessage]` WebSocket stream. Chat is accessible as a tab in every state, including states whose primary view is kanban or diff.

### Kanban UI (implementation state)

Displays KanbanItems as columns: `todo`, `implementing`, `done`. Items move across columns as the agent updates their status. The user can observe progress in real time but does not interact with individual items directly. A chat panel is always available alongside the kanban for the user to talk to the implementer while it works.

### File Explorer

Read-only file tree and viewer for the Task's worktree. The backend serves directory listings and raw file contents on demand via REST — no watching, purely on-demand reads from the worktree path on disk.

**Frontend dependency:** CodeMirror (read-only mode) for file viewing with syntax highlighting across all languages.

### Markdown Rendering

Markdown rendering is entirely a frontend concern — the backend serves raw content, the browser renders it. Used in two places: agent chat messages in brainstorming and spec states (which produce markdown), and `.md` files opened in the file explorer.

**Frontend dependencies:**
- **markdown-it** — markdown parsing and rendering
- **mermaid.js** — renders ```` ```mermaid ```` fenced blocks as diagrams (state machines, flowcharts, ER diagrams)
- **highlight.js** — syntax highlighting inside code blocks, as a markdown-it plugin

### AI Review UI (ai_review state)

The `ai_review` state uses the same diff view as `human_review` — the human can watch the AI reviewer's threads appear in real time as the reviewer works through the diff. The chat panel is available alongside the diff so the human can observe (or interrupt) the AI-to-AI exchange.

The reviewer's threads have `phase=ai_review` and are visually distinguished from human threads. When the reviewer signals `review_passed`, the task transitions to `human_review`. When it signals `review_failed`, the task returns to `implementation` and the implementer addresses the open threads.

### Diff UI (human_review state)

The backend parses `git diff {base_branch}...{branch_name}` into structured JSON — file by file, hunk by hunk, line by line — including both old and new file line numbers for every line. The frontend renders this as a GitHub-style diff.

ReviewThreads are anchored to `file_path + line_number` (new-file line number from the structured diff). When the diff view loads, Crowbar fetches all ReviewThreads for the Task and renders them inline on their respective lines — collapsed by default, expanding on click, showing the full ReviewMessage history. Threads are visually distinguished by `phase`: `ai_review` threads (AI-opened, from the item review loop) appear with a different badge than `human_review` threads.

**Opening a thread:**
- **Human** — clicks any line in the diff; frontend reads `file_path` and `line_number` from the structured diff data and creates a ReviewThread with `phase=human_review, opened_by=human`
- **AI reviewer agent** — calls `crowbar_open_thread(file, line, content)` via MCP during an item review run; Crowbar sets `phase=ai_review, opened_by=reviewer` automatically

**Thread interactions:**
- **Open or continue a thread** → the human leaves a comment on a file/line; the Implementation agent responds via `crowbar_reply_thread`
- **Agent emoji react** → when the agent agrees with a human comment, it reacts with an emoji (👍, 🚀, etc.) instead of a text reply. This signals agreement without noise.
- **Agent challenge** → when the agent disagrees, it replies in the thread with its reasoning. The human can respond, and the conversation continues until both agree or the human force-approves.
- **Force-change button** → available on any thread at any time; closes it with `status: force_approved`. The implementer agent is required to acknowledge with a brief note stating what it will implement — it may not skip or defer.
- **Approve button** → available once the human is satisfied (all threads are `agreed` or `force_approved`). Crowbar fires the `approved` event, moving the Task to the next state. If the human is not satisfied with the agent's responses, they click **Request Changes** instead — Crowbar fires `changes_requested` and the Task moves back to the `implementation` state for broader rework.

**Recursive review loops:**

*AI review loop (item-level):*
Each KanbanItem cycles through `ai_review → needs_revision → ai_review` until the AI reviewer is satisfied, at which point the item advances to `human_review`. The AI reviewer runs once per cycle; on each subsequent run it sees the full thread history and skips threads already resolved.

*Human review loop (task-level):*
All items must reach `human_review` before the task-level diff view opens. From there, the human and implementer exchange on threads. If the human clicks **Request Changes**, the task returns to `implementation` — the implementer addresses the broader concern and re-submits all items through the full AI review → human review pipeline again.

The Implementation agent reads the full message history of each thread when forming its response — not just the last message.

> **Outdated threads:** if the agent makes new commits after a thread is opened, line numbers in the file may shift. Threads remain anchored to the line number at the time they were opened and may be marked `outdated` in a future iteration — the same behaviour as GitHub PR comments.

### Memory Management

Per-Repository and per-Project Memory tabs where users can:

- Browse all Memory entries (filterable by type, scope, and source)
- Read the full content of any entry
- Edit an entry (triggers a re-embed in LanceDB)
- Delete an entry (removes from SQLite and LanceDB)
- Add an entry manually (user-authored, `source: "human"`)
- See the originating Task for agent-extracted entries
- See the strength score (reinforcement count) for each entry

### Flow Authoring

- Point a Repository to any Flow YAML file on disk
- Crowbar validates the YAML on load: checks that all `transitions.to` reference defined states, all `tools` are known, and the schema is well-formed
- Validation errors are surfaced before a Task can be created with that Flow

### Git Integration

Each Task owns its branch entirely — every commit on that branch was made during that Task's agent runs. This makes git history unambiguous at the Task level.

**Task Git tab:**
- **Commits view** — full log of the Task's branch, in chronological order
- **Diff view** — `git diff {start_sha}..{end_sha}`: the complete diff of everything the Task changed, from branch creation to completion. Functions as a per-Task PR diff.

**Repository Git view:**
- Lists all Task branches across the Repository with their status (active, paused, complete)
- Allows navigating to any Task's Git tab from a single place

The agent commits freely during its runs using its terminal tool — Crowbar does not manage commits. `start_sha` is recorded when the Task is created; `end_sha` is recorded when the Task reaches a terminal state.

### Terminal Integration

Crowbar embeds a multi-tab PTY terminal panel available at all times alongside any task view. This is the user's own shell — entirely independent of any running agent session. Agent sessions always render as a chat interface; the terminal panel is strictly for the user's own use (running dev servers, git commands, inspecting the filesystem, etc.).

**Layout — responsive to window orientation:**
- Portrait window → terminal panel anchored at the bottom, resizable height
- Landscape window → terminal panel anchored on the left, resizable width

The panel is always visible; it does not need to be toggled open.

**Tabs:**
- Each task starts with one terminal tab, opened at the task's worktree root
- Users can open additional tabs with a `+` button; each tab is an independent shell session
- Tabs display a running-process indicator (green dot) when a process is active
- Switching away from a tab with a running process keeps it alive in the background — the process is not interrupted
- Tabs are per-task — switching tasks in the sidebar shows that task's own terminal tabs

**Protocol:**
Terminal output flows from the PTY process in the Go backend to the frontend over a WebSocket connection, rendered by xterm.js. User keystrokes travel the reverse path. Resize events are sent as structured control messages. This is the same approach used by VS Code's integrated terminal and similar tools.

### Activity Graph

A repository-level knowledge graph showing the relationships between Tasks and Memory entries — inspired by Obsidian's graph view. Gives the user a visual overview of the project's accumulated intelligence.

**Nodes:**
- **Task** — colored by status (`active` = blue, `paused` = grey, `complete` = green). Clicking navigates to the Task.
- **Memory** — sized by `strength_score`; frequently reinforced memories appear visually larger. Clicking navigates to the Memory entry.

**Edges:**
- Task → Memory: this Task's agent runs produced this Memory entry
- Memory → Task: this Memory was semantically retrieved and injected into this Task's context
- Memory → Memory: reinforcement link — a later Task validated the same pattern, increasing the strength score

**Frontend dependency:** **Cytoscape.js** — purpose-built for relationship graphs, handles large node counts, supports interactive force-directed layouts.

---

## Agent Runtime Model

All agents in Crowbar run as **ACP-compatible CLI subprocesses**. Crowbar is an ACP client — it never calls an AI provider API directly. Authentication, model selection, and inference are delegated entirely to the agent process.

### Intelligence Levels

Every Flow state declares an `intelligence` tier. This is the only AI configuration in a Flow — no model names, no provider references. Flows are fully agent-agnostic.

| Level | Intended use |
|-------|-------------|
| `low` | Fast, cheap tasks: lightweight extraction, simple summaries |
| `medium` | Standard reasoning: brainstorming, spec writing, pattern extraction |
| `high` | Complex reasoning: implementation, detailed code review, debugging |
| `ultrahigh` | Most demanding: large-context analysis, critical architecture decisions |

### User Configuration

In `crowbar.yaml`, the user maps each intelligence level to a model name. The model name implicitly determines which agent CLI Crowbar will spawn — no explicit agent field needed.

```yaml
intelligence:
  low: claude-haiku-4-5
  medium: claude-sonnet-4-6
  high: claude-sonnet-4-6
  ultrahigh: claude-opus-4-7
```

### Agent Registry

Crowbar ships with an internal registry mapping model name prefixes to the CLI command and ACP flags for each supported agent:

| Model prefix | CLI | ACP invocation |
|-------------|-----|----------------|
| `claude-*` | `claude` | `claude --acp --model <model>` |
| `codex-*` | `codex` | `codex acp --model <model>` |
| `gemini-*` | `gemini` | `gemini --acp --model <model>` |

Resolution at runtime: `intelligence: high` → user config `high: claude-sonnet-4-6` → registry prefix `claude-*` → spawn `claude --acp --model claude-sonnet-4-6`.

### Context Injection

Crowbar injects context into each agent session via two mechanisms:

1. **MCP config passthrough** — At session start, Crowbar passes its MCP server config to the agent. The agent connects directly to the Crowbar MCP server and gains access to all declared tools (`crowbar_signal`, `crowbar_create_item`, etc.). Crowbar never implements custom tool dispatch.

2. **`session/prompt`** — Crowbar sends a structured prompt containing:
   - The state's `system_prompt`
   - All prior `AgentRun.output` values in chronological order (context handoff between states)
   - Relevant Memory entries (retrieved via semantic search against repo + project memory)

**Credentials are never stored or forwarded by Crowbar.** Each agent CLI reads its own configuration (e.g. `~/.claude/` for Claude Code). The user brings their own agent and their own auth.

> **Cloud deployment note:** ACP remote transport (HTTP/WebSocket) is currently a work in progress in the ACP spec. Crowbar's current model assumes the agent process runs on the same host as the engine. Remote agent deployment will be revisited when ACP remote transport stabilizes.

---

## Execution Model

When a Task enters a new state that has an `agent`:

1. Crowbar creates an **AgentRun** record, recording the intelligence tier and the model name resolved from `crowbar.yaml`
2. Uses the **Worktree** dedicated to this Task (created when the Task was first started)
3. Spins up a **Docker container** pointed at the assigned worktree
4. Resolves the state's `intelligence` → model name (from `crowbar.yaml`) → CLI spawn command (from the agent registry)
5. Spawns the ACP agent process inside the container and establishes an ACP session (`initialize` + `session/new`)
6. Passes the Crowbar MCP server config to the agent so it can connect directly to Crowbar's tool API
7. Sends a `session/prompt` containing:
   - The state's `system_prompt`
   - **Previous step outputs** — the `output` field from every completed AgentRun in this Task, in chronological order. Each agent produces a markdown summary when it calls `crowbar_signal()`, and every subsequent agent receives all prior summaries.
   - Relevant Memory entries (semantic search against repo + project memory)
8. The agent runs, calling Crowbar tools via MCP, until it calls `crowbar_signal()`
9. Crowbar evaluates the signal against the state's `transitions` and moves the Task

When an **improvement agent** trigger fires:

1. Crowbar evaluates the `filter` expression against the event payload
2. If it matches, creates a background AgentRun (no UI)
3. Container + worktree provisioned as above, but with the improvement agent's restricted tool set
4. Crowbar constructs a `session/prompt` containing:
   - The event payload (structured data: IDs, status values, etc.)
   - Resolved DB entities referenced by the event (e.g. full ReviewThread + all ReviewMessages; KanbanItem details)
   - The triggering AgentRun's `output` summary
   - **File pointers to on-disk artifacts** — the agent reads these with `fs.read` as needed, pulling only what is relevant rather than consuming the full content upfront:
     - `{data_dir}/runs/{agent_run_id}/events.jsonl` — full ACP event log of the triggering step
     - `{data_dir}/runs/{agent_run_id}/diff.patch` — git diff produced at AgentRun completion
   - File pointers for any additional steps declared in `context.step`
   - Relevant Memory entries (semantic search, same as any AgentRun)
5. Agent runs, writes Memory or opens threads, then exits
6. No state transition — the main Task is unaffected

---

## Memory Sync Invariant

Every write to Memory (by any agent or by the user) goes through `MemoryService`:

1. Write to SQLite
2. Compute vector embedding
3. Upsert to LanceDB

Reads for UI (browsing, editing) come from SQLite. Reads for agent context (semantic search) come from LanceDB. They are always consistent because all writes are synchronous through the service.
