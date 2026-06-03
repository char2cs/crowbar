# Crowbar Backend — Chat Lifecycle

> **Status:** Approved
> **Date:** 2026-06-03
> **Depends on:** `00-architecture-and-domain.md`
> **Scope:** The lifecycle of a chat as a domain aggregate — creation, forking,
> renaming, deletion, and live status. **Conversation content** (turns, tool
> calls, widgets, agent TODO lists, streaming wire protocol) is **out of scope**
> and decided after the Agentic Bridge spike (`11-agentic-bridge-spike.md`).

---

## 1. Overview

Every workspace owns a list of AI conversations ("chats"). Chats form a **tree**:
any chat can be forked into a child that inherits context up to a chosen point
and then diverges. The sidebar renders this tree; opening a chat opens a
`crowbarChat` pane that shows its content.

Chats are stored in **Asynx**. The aggregate's state is small and bounded
regardless of conversation length, so event sourcing is cheap here and gives us
projections and reactions for free — consistent with how aggregates are modeled
across Rabbyte.

---

## 2. Chat Aggregate

```
Chat {
  id        uuid
  wsId      uuid          // workspace this chat belongs to
  title     string
  parentId  uuid?         // empty for root chats; set on fork
  status    ChatStatus    // idle | agent-running
  type      ChatType      // chat | workflow
  createdAt time.Time
  deletedAt time.Time?    // soft delete
}

ChatStatus = "idle" | "agent-running"
ChatType   = "chat" | "workflow"
```

The frontend reconstructs the tree client-side from `parentId` references (root
chats have no `parentId`; order within a level is by `createdAt`). The backend
returns a **flat list**.

`age` (relative-time string) is derived from `createdAt`, not stored.

---

## 3. Commands

| Command | Trigger | Effect | Transition |
|---------|---------|--------|------------|
| `CreateChat`        | `POST /v0/workspaces/:wsId/chats` | New root chat | → idle |
| `ForkChat`          | `POST /v0/chats/:id/fork`         | New child with `parentId` set, inheriting context up to `fromTurnId` | → idle |
| `RenameChat`        | `PATCH /v0/chats/:id`             | Update title | — |
| `DeleteChat`        | `DELETE /v0/chats/:id`            | Soft-delete; cascades to children | → deleted |
| `AgentRunStarted`   | AgentRun subscription (internal)  | Reflect a live agent | → agent-running |
| `AgentRunCompleted` | AgentRun subscription (internal)  | Reflect agent finishing | → idle |

`AgentRunStarted` / `AgentRunCompleted` are **never** called by the API directly.
They are issued automatically by the AgentRun Asynx subscription (see §5). The
Chat aggregate has no opinion on *why* an agent started — it only reflects status.

---

## 4. Fork Semantics

A fork creates a new `Chat` with `parentId` pointing at the source chat. The fork
point is identified by a `fromTurnId`.

- The child inherits conversation context up to and including `fromTurnId`, then
  continues independently.
- Forks can themselves be forked — depth is unbounded.
- A parent may have multiple forks.

> **How inherited context is materialized** (copy rows vs. reference a parent
> pointer vs. replay) is **deferred** — it depends on the conversation-content
> storage model decided after the Agentic Bridge spike. This spec fixes only the
> aggregate-level relationship (`parentId` + `fromTurnId`), not the content
> mechanics.

---

## 5. Status Projection (Hub Wiring)

A chat's `agent-running` status is **derived** from whether an AgentRun is
currently live for that chat. The chat never independently decides it is
"running" — it mirrors the AgentRun lifecycle.

```
User sends a message (endpoint = post-spike)
  └─► AgentRun created (Asynx): pending → running
        └─► AgentRun subscription fires
              └─► send AgentRunStarted to Chat aggregate
                    └─► Chat: idle → agent-running
                          └─► Chat subscription fires
                                └─► hub.BroadcastChat({ chatId, wsId, status })
                                      └─► WS /v0/ws/chats?wsId=  pushes to sidebar

Agent finishes
  └─► AgentRun (Asynx): running → done | error | interrupted
        └─► AgentRun subscription fires
              └─► send AgentRunCompleted to Chat aggregate
                    └─► Chat: agent-running → idle
                          └─► hub.BroadcastChat({ chatId, wsId, status: "idle" })
```

The sidebar spinner is driven entirely by these pushes — never by polling.

---

## 6. REST Surface (lifecycle only)

```
GET    /v0/workspaces/:wsId/chats     flat list for the workspace
POST   /v0/workspaces/:wsId/chats     create root chat { title? }
POST   /v0/chats/:id/fork             fork { fromTurnId } → child with parentId
PATCH  /v0/chats/:id                  rename { title }
DELETE /v0/chats/:id                  delete (cascades to children)
```

Message sending and the agent response stream are **post-spike** and specified
separately once the Agentic Bridge is designed.

---

## 7. WebSocket Surface

```
WS /v0/ws/chats?wsId=        namespace: chatId
                             pushes ChatStatus changes (idle ↔ agent-running)
                             for every chat in the workspace
```

The per-chat **content stream** (`WS /v0/ws/chats/:chatId/stream`) is listed in
the API-surface spec for completeness but its frame protocol is **post-spike**.

---

## 8. Deletion & Cascade

`DeleteChat` soft-deletes (sets `deletedAt`). Deleting a parent cascades to its
children — implemented via the Chat subscription reacting to a parent's delete
event and issuing `DeleteChat` for each child. This mirrors the
drag-to-delete behavior in the sidebar (UX spec §4, §15).
