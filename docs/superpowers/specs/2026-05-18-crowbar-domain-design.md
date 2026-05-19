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
                         # what Crowbar renders when the Task is in this state.
                         # `background` = no user-facing UI.
                         # A chat panel is ALWAYS available alongside any primary
                         # UI type when an agent is running — `kanban` means kanban
                         # is the primary view with chat alongside it; `diff` means
                         # the diff is primary with chat alongside it. This lets the
                         # user talk to the agent at any point during its execution.
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
        Implement the approved spec. Create an item for each task before starting.
        Work through items one at a time. When done with an item, call
        crowbar_update_item_status(item_id, "ai_review").

        Periodically check for open AI review threads:
          crowbar_get_threads(status="open", phase="ai_review")
        For each open thread, read the concern, make the fix if valid, and reply with
        crowbar_reply_thread(thread_id, content). After addressing all threads for an
        item, call crowbar_update_item_status(item_id, "ai_review") to re-submit it.

        When ALL items have reached "human_review" status, call
        crowbar_signal("all_items_ready").
    transitions:
      - to: human_review
        on: all_items_ready
      - to: spec
        on: scope_changed

  - name: human_review
    ui: diff
    agent:
      intelligence: high
      tools: [crowbar.signal, crowbar.get_threads, crowbar.reply_thread, fs.read, terminal]
      system_prompt: |
        The implementation is complete and a human is reviewing the full diff.
        You are available via the chat panel to answer any questions about the
        implementation, architectural choices, or trade-offs.

        Continuously monitor for open human review threads:
          crowbar_get_threads(status="open", phase="human_review")
        For each open thread, read the human's concern and reply with your reasoning
        or the fix you will make using crowbar_reply_thread(thread_id, content).

        Force-approved threads: the human has overridden — you MUST implement the
        requested change. Acknowledge with a brief note describing what you will do.

        The human controls progression:
        - If they click Approve: Crowbar fires "approved" — you are done.
        - If they click Request Changes: Crowbar fires "changes_requested" — you will
          be re-invoked in the implementation state to address broader rework.
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
  - name: ai_item_reviewer
    description: Reviews each implementation item when it enters ai_review status. Runs on every review round — thread history from prior rounds is available via file pointers.
    trigger:
      event: item_status_changed
      filter: "new_status == 'ai_review'"
    scope: repo
    context:
      - step: implementation   # expose the implementation AgentRun's events.jsonl + diff.patch
    agent:
      intelligence: high
      tools: [crowbar.update_item_status, crowbar.open_thread, crowbar.get_threads, crowbar.reply_thread, fs.read]
      system_prompt: |
        An implementation item has entered AI review (possibly for the Nth time).

        1. Read the item's diff using the file pointer provided in context.
        2. Check existing threads (crowbar_get_threads) to see what was raised in
           prior rounds and how the implementer responded. Do NOT re-open threads
           that were already agreed or force-approved.
        3. For any remaining unresolved issues, or new issues introduced in this
           revision, open a new ReviewThread with crowbar_open_thread(file, line, concern).
        4. When your review is complete:
           - If no open issues: crowbar_update_item_status(item_id, "human_review")
           - If issues found: crowbar_update_item_status(item_id, "needs_revision")

        The implementer will address your threads and re-submit for ai_review.
        You will run again — this loop continues until the item is clean.

  - name: review_pattern_extractor
    description: Extracts coding principles from resolved review threads
    trigger:
      event: thread_resolved
    scope: repo
    agent:
      intelligence: medium
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
| **Archive** | Any non-running Task | Permanent; removes worktree from disk |

**How AgentRuns fail:** the ACP subprocess crashes, the Docker container dies, the MCP connection drops, or the agent exits without calling `crowbar_signal()`. All of these produce `status: failed`. User-initiated stops produce `status: interrupted`. Recovery is identical for both — Retry creates a fresh AgentRun for the same state.

### Chat UI

Real-time chat interface between the user and the agent. The agent can read the codebase and signal transitions. Conversation history is preserved and passed to subsequent states.

**Chat is always available alongside any primary UI type** when an agent is running. In `kanban` states (implementation), the user can talk to the implementer while watching it work. In `diff` states (human_review), the user can ask the agent questions about the code while reviewing the diff. The chat panel does not replace the primary view — it sits alongside it.

### Kanban UI (implementation state)

Displays KanbanItems and their statuses as columns. Shows which item is currently being implemented and which are in AI review. Items move across columns as the agent updates their status. The user can observe progress in real time but does not interact with individual items directly.

### File Explorer

Read-only file tree and viewer for the Task's worktree. The backend serves directory listings and raw file contents on demand via REST — no watching, purely on-demand reads from the worktree path on disk.

**Frontend dependency:** CodeMirror (read-only mode) for file viewing with syntax highlighting across all languages.

### Markdown Rendering

Markdown rendering is entirely a frontend concern — the backend serves raw content, the browser renders it. Used in two places: agent chat messages in brainstorming and spec states (which produce markdown), and `.md` files opened in the file explorer.

**Frontend dependencies:**
- **markdown-it** — markdown parsing and rendering
- **mermaid.js** — renders ```` ```mermaid ```` fenced blocks as diagrams (state machines, flowcharts, ER diagrams)
- **highlight.js** — syntax highlighting inside code blocks, as a markdown-it plugin

### Diff UI (human review state)

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
