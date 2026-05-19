# Crowbar — Domain & Feature Spec

**Date:** 2026-05-18
**Status:** Approved

---

## Overview

Crowbar is a general-purpose **agent orchestration platform** for software development teams. It runs AI agents through user-defined, version-controlled workflows (Flows), accumulates learned context over time (Memory), and progressively reduces the need for human review by encoding developer taste into every future agent run.

The feature development flow described in this spec is the **first built-in Flow** — not the only one. All domain entities and platform primitives are designed to be flow-agnostic.

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

One execution of a Flow on a Repository. Moves through states as defined by the Flow YAML. Each Task gets its own git branch.

| Field | Type | Notes |
|-------|------|-------|
| `id` | uuid | |
| `repo_id` | fk → Repository | |
| `title` | string | |
| `flow_path` | string | Path to the Flow YAML file used |
| `current_state` | string | Name of the active state in the Flow |
| `status` | enum | `active`, `paused`, `complete` |
| `branch_name` | string | Git branch created for this Task |
| `visited_states` | string[] | Ordered list of states the Task has passed through |
| `created_at` | timestamp | |

Relations: has many **AgentRuns**, **KanbanItems**, **ReviewThreads**.

---

### AgentRun

One agent execution within a Task state. Each AgentRun gets its own Docker container and git worktree.

| Field | Type | Notes |
|-------|------|-------|
| `id` | uuid | |
| `task_id` | fk → Task | |
| `state_name` | string | Flow state this run belongs to |
| `agent_type` | string | Label from the Flow YAML |
| `container_id` | string | Docker container ID |
| `worktree_path` | string | Absolute path to the git worktree |
| `status` | enum | `running`, `complete`, `failed` |
| `started_at` | timestamp | |
| `completed_at` | timestamp | nullable |

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

### ReviewThread

A conversation thread anchored to a specific file and line, opened during a review state. Modelled like a Slack thread — full back-and-forth between the human reviewer and the Implementation agent.

| Field | Type | Notes |
|-------|------|-------|
| `id` | uuid | |
| `task_id` | fk → Task | |
| `file_path` | string | |
| `line_number` | int | |
| `status` | enum | `open`, `agreed`, `force_approved` |
| `created_at` | timestamp | |

Relations: has many **ReviewMessages**.

---

### ReviewMessage

A single message within a ReviewThread. Supports full multi-turn conversations between the human and the agent.

| Field | Type | Notes |
|-------|------|-------|
| `id` | uuid | |
| `thread_id` | fk → ReviewThread | |
| `role` | enum | `human`, `agent` |
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
                         # what Crowbar renders when the Task is in this state
                         # `background` = no user-facing UI
  items: bool            # if true, this state creates and manages KanbanItems
  agent:
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
  agent:
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
| `task_complete` | `task_id`, `visited_states` | Task reaches a terminal state |

### Agent tool API (MCP)

Crowbar exposes these tools to agents via MCP. Agents call them like any Claude tool — no string parsing, no special output formats.

| Tool | Signature | Description |
|------|-----------|-------------|
| `crowbar_signal` | `(event: string, payload?: object)` | Trigger a state transition or emit an event |
| `crowbar_create_item` | `(title: string, description?: string) → item_id` | Create a KanbanItem |
| `crowbar_update_item_status` | `(item_id: string, status: string)` | Move an item to a new status |
| `crowbar_get_items` | `() → Item[]` | List all KanbanItems for this Task |
| `crowbar_open_thread` | `(file: string, line: int, content: string) → thread_id` | Open a ReviewThread |
| `crowbar_reply_thread` | `(thread_id: string, content: string)` | Append a message to a thread |
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
  - ai_review
  - needs_revision
  - human_review
  - done

states:
  - name: brainstorming
    ui: chat
    agent:
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
      tools: [crowbar.signal, crowbar.create_item, crowbar.update_item_status, crowbar.get_items, fs.read, fs.write, terminal]
      system_prompt: |
        Implement the approved spec. Create an item for each task before starting.
        Work through items one at a time. When done with an item, call
        crowbar_update_item_status(item_id, "ai_review").
        When all items reach "done", call crowbar_signal("all_items_complete").
    transitions:
      - to: human_review
        on: all_items_complete
      - to: spec
        on: scope_changed

  - name: human_review
    ui: diff
    transitions:
      - to: implementation
        on: changes_requested
      - to: complete
        on: approved

  - name: debugging
    ui: chat
    agent:
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
  - name: ai_item_reviewer
    description: Reviews each implementation item when it enters ai_review status
    trigger:
      event: item_status_changed
      filter: "new_status == 'ai_review'"
    scope: repo
    agent:
      tools: [crowbar.update_item_status, crowbar.open_thread, fs.read]
      system_prompt: |
        An implementation item has entered AI review. Review its diff against
        the repository memory and coding standards. Open a ReviewThread for
        each issue found, referencing file and line number. When done:
        - If clean: call crowbar_update_item_status(item_id, "done")
        - If issues found: call crowbar_update_item_status(item_id, "needs_revision")

  - name: review_pattern_extractor
    description: Extracts coding principles from resolved review threads
    trigger:
      event: thread_resolved
    scope: repo
    agent:
      tools: [crowbar.write_memory]
      system_prompt: |
        A review thread was resolved. Analyze the full conversation and extract
        any generalizable principle as a concise, actionable rule. Call
        crowbar_write_memory(content, type) to save it.
        Skip if the thread was force-approved with no substantive discussion.

  - name: debug_pattern_extractor
    description: Captures reusable debugging patterns after debug sessions
    trigger:
      event: task_complete
      filter: "visited_states includes 'debugging'"
    scope: repo
    agent:
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

- Create a Task on a Repository, selecting a Flow YAML file (defaults to `Repository.default_flow`)
- View the Task list for a Repository; Tasks waiting for user input are prominently highlighted
- Interact with the current Task state through the appropriate UI (chat, kanban, or diff)
- View full Task history — which states were visited and in what order

### Chat UI (brainstorming / debugging / spec states)

Real-time chat interface between the user and the agent. The agent can read the codebase and signal transitions. Conversation history is preserved and passed to subsequent states.

### Kanban UI (implementation state)

Displays KanbanItems and their statuses as columns. Shows which item is currently being implemented and which are in AI review. Items move across columns as the agent updates their status. The user can observe progress in real time but does not interact with individual items directly.

### Diff UI (human review state)

Displays the full git diff between the Task branch and the base branch. AI-opened ReviewThreads appear inline on the relevant lines. The human reviewer interacts as follows:

- **Open or continue a thread** → the human leaves a comment on a file/line; the Implementation agent responds
- **Agent emoji react** → when the agent agrees with a human comment, it reacts with an emoji (👍, 🚀, etc.) instead of a text reply. This signals agreement without noise.
- **Agent challenge** → when the agent disagrees, it replies in the thread with its reasoning. The human can respond, and the conversation continues until both agree or the human force-approves.
- **Force-change button** → available on any thread at any time; closes it with status `force_approved`. The agent must acknowledge with a brief note describing what it will implement.
- **Approve button** → once the human is satisfied with all threads (all are `agreed` or `force_approved`), they click Approve. Crowbar fires the `approved` event, moving the Task to the next state.

The Implementation agent reads the full message history of each thread when forming its response — not just the last message.

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

---

## Execution Model

When a Task enters a new state that has an `agent`:

1. Crowbar creates an **AgentRun** record
2. Spins up a **Docker container** for isolation
3. Creates a **git worktree** at a unique path within the repo
4. Injects into the agent context:
   - The state's `system_prompt`
   - Relevant Memory entries (semantic search against repo + project memory)
   - Full Task conversation history
   - The declared tool set (Crowbar MCP tools + standard tools)
5. The agent runs until it calls `crowbar_signal()` or fails
6. Crowbar evaluates the signal against the state's `transitions` and moves the Task

When an **improvement agent** trigger fires:

1. Crowbar evaluates the `filter` expression against the event payload
2. If it matches, creates a background AgentRun (no UI)
3. Container + worktree provisioned as above, but with the improvement agent's restricted tool set
4. Agent runs, writes Memory or opens threads, then exits
5. No state transition — the main Task is unaffected

---

## Memory Sync Invariant

Every write to Memory (by any agent or by the user) goes through `MemoryService`:

1. Write to SQLite
2. Compute vector embedding
3. Upsert to LanceDB

Reads for UI (browsing, editing) come from SQLite. Reads for agent context (semantic search) come from LanceDB. They are always consistent because all writes are synchronous through the service.
