# Agentic Engine — Event-Sourced `AgentChat` Aggregate (Design)

**Date:** 2026-07-09
**Branch:** `feature/agentic-bridge` (target; implemented *after* `develop` is merged in)
**Status:** design — deferred to the implementation phase

## Context / why

The agentic-bridge feature currently persists conversation state
(`domain.AgentChat` / `domain.AgentSegment`) in a **bespoke gorm store on
`GlobalView`** (`repositories/agentchat`), with a hand-rolled
`usecases/agent/keyed_mutex.go` serializing the hook read→reduce→persist per
`CrowbarSegmentID`.

The #44 refactor made the backend's durable aggregates **event-sourced
per-type** via `char2cs/asynx` (`Workspace`, `ReviewThread`) and *deleted* the
old event-sourced `domain.Chat` aggregate.

The agentic conversation domain is **event-native**: a chat is an append-only
sequence of provider *segments* (Claude↔Codex switches) and *turns*
(working/idle). Event sourcing fits it better than most existing aggregates —
`ReviewThread` is really CRUD-comments-plus-a-resolve-flag; a conversation
threaded across provider stints with switch/turn history is a richer event
vocabulary. This design converts `AgentChat` into an asynx aggregate — the
natural successor to the deleted `Chat`.

**Boundary preserved:** asynx owns **state + history**, never the process. The
vendor-CLI PTY/subprocess lifecycle stays in `usecases/agent` + the terminal
engine and *feeds pure commands in* (the `workspace/internal/reconcile`
"all IO out here, the command does none" pattern). This is the same boundary
terminal PTY and LSP already respect.

## Scope / non-goals

- **Not** a process-supervisor migration: the vendor-CLI PTY lifecycle stays in
  the usecase + terminal engine.
- **No data migration** (pre-production, no users): drop the gorm
  `agent_chats` / `agent_segments` tables, clear dev state, start on the event
  log. See [[feedback_no_legacy_migration]].
- **Sequencing:** lands *after* `develop` is merged into the feature branch
  (the `worktreepath.For`→`Derive` compile fix + 2 trivial conflicts). This doc
  is the blueprint for that implementation phase, not a merge-time edit.

## The aggregate

`domain.AgentChat` becomes the aggregate root, **chat-scoped**. Segments fold
*into* it as embedded state (no separate table / aggregate). Aggregate id =
chat id, so **every event for one conversation serializes through one asynx
aggregate in version order**.

Folded state is lean — *current* state, not an unbounded replay of every event:

```go
type AgentChat struct {
    ID          string
    WorkspaceID string
    Title       string
    TitleLocked bool
    CreatedAt   time.Time

    Segments        []AgentSegment // current set; invariant: ≤1 Status=="active"
    ActiveSegmentID string

    // live turn state (folded from Turn events; reconciled on boot)
    Working            bool
    CurrentTurnStarted *time.Time
    LastActivityAt     time.Time

    Status AgentChatStatus // active | archived | deleted (tombstone)
}

type AgentSegment struct {
    ID                string
    ProviderID        string
    ProviderSessionID string // learned at session_start (BindSession)
    CrowbarSegmentID  string
    TerminalSessionID string // reference into terminal-engine state (NOT asynx-owned)
    StartedAt         time.Time
    EndedAt           *time.Time
    Status            string // active | ended
}
```

**Why chat-scoped + embedded segments:**

- The **≤1-active-segment invariant** becomes an in-aggregate check inside each
  command's `Validate` — no cross-aggregate coordination, and
  `keyed_mutex.go` **is deleted** (asynx per-aggregate OCC does the
  serialization it was hand-rolling).
- A provider switch is **two events on the *same* aggregate** (`EndSegment` +
  `OpenSegment`), atomically ordered — no dance between two tables.

## Commands / events

Each is a pure `asynx.Command[domain.AgentChat]` (`Validate` + `EmitEvent`, no
IO). `EventName` is id-suffixed so reactors/projections topic-match.

| Command | EventName | Trigger | Pure fold |
|---|---|---|---|
| `Create` | `agentchat.created.<id>` | first spawn in a workspace | seed chat, open first segment, set active |
| `OpenSegment` | `agentchat.segment_opened.<id>` | spawn / switch-in | append segment (active), set `ActiveSegmentID`; **Validate: no other active** |
| `EndSegment` | `agentchat.segment_ended.<id>` | switch-out / process exit | mark active segment `EndedAt` + `ended`, clear active |
| `BindSession` | `agentchat.session_bound.<id>` | `session_start` hook | set `ProviderSessionID` on the segment for a `CrowbarSegmentID` |
| `StartTurn` | `agentchat.turn_started.<id>` | `user_prompt` hook | `Working=true`, `CurrentTurnStarted=now` |
| `StopTurn` | `agentchat.turn_stopped.<id>` | `turn_stop` / Stop hook | `Working=false`, `CurrentTurnStarted=nil` |
| `SetTitle` | `agentchat.title_set.<id>` | first-prompt derive / user / agent `crowbar chat rename` | set `Title` honoring precedence user>agent>derived; `TitleLocked` on user rename |
| `Delete` | `agentchat.deleted.<id>` | user deletes chat | tombstone `Status=deleted` (→ `Forget` + reactor cleanup) |

The engine reducer (`engine/agent.Registry`) decides *which* command a hook
produces; it becomes **pure** — its in-memory `session_start` mutex is
unnecessary because OCC retry re-runs the decision against the current version.

## The read→reduce→emit flow (usecase)

`IngestHook` (and `/switch`, rename) become, per the reconcile pattern:

1. `ax.Get(chatID)` — read the current aggregate.
2. Run the pure engine reducer → decide the outcome (one command, or a switch =
   two).
3. Do any required **IO here**, outside the command (on switch:
   `TerminateGraceful` the old CLI, spawn the new one via `TerminalCommander`).
4. `SendWait(command...)` — apply the pure state transition(s); block until
   projections settle.

Concurrent hooks for the same chat: asynx OCC + versioned `Append` serialize
them; a losing writer retries against the new version and its `Validate`
re-checks the invariant. **`keyed_mutex.go` is removed.**

## Continuity & resume (switch-back)

A switch is a **new segment in the same chat**, but the target CLI's context is
restored one of two ways depending on history (already built —
`usecases/agent/agent.go:577-679`):

- **Forward switch** (target provider never used in this chat): spawn fresh,
  inject only the **handoff** — the per-chat ledger wrapped by `HandoffWrapper`
  — so the new provider sees the whole conversation so far.
- **Switch-*back*** (target has a prior segment with a resolved
  `ProviderSessionID`): spawn with the handoff **plus** a native
  `session.resume` into that prior session. So returning to Claude after a Codex
  detour gives Claude **both** its own resumed session (its segment-1 memory,
  native) **and** the handoff covering the Codex turns it missed.

The prior-session lookup scans the chat's segments for
`ProviderID==target && ProviderSessionID!=""` (`agent.go:646-649`). In the
aggregate this is an **in-memory scan of `chat.Segments`** — no query, always
consistent because it is folded state. `Claude→Codex→Claude` = 3 segments;
segments 1 and 3 share one `ProviderSessionID`, which is exactly why identity
splits `CrowbarSegmentID` (the slot) from `ProviderSessionID` (the vendor
session).

Restart continuity: the engine registry is **seeded from each segment's
`ProviderSessionID`** at boot (`agent.go:722-725`) so a resumed CLI's `/resume`
is recognized as the existing segment, not a new one.

## Ledger: conversation content vs aggregate metadata

The **ledger** — an append-only, per-chat record on disk spanning every provider
segment — remains the **conversation-content store and handoff source**
(`AssembleHandoff`, `agent.go:541-573`). It is **not** absorbed into the event
log. The split:

- **Ledger** owns message content; it is what a handoff replays, so it already
  carries one provider's turns into the next (what makes switch-back to Claude
  aware of the Codex detour).
- **`AgentChat` aggregate** owns identity, segments, session ids, title, and the
  live `Working` flag — plus a ledger cursor/count so a projection can relate the
  two. It never holds message bodies.

This **resolves the earlier "turn durability" question**: content is in the
ledger regardless, so aggregate `StartTurn`/`StopTurn` events are only the
working-state *timeline*, never message text. Per incoming turn there are two
writes — the **ledger append** (durable content truth) and the **turn event**
(live/metadata). The ledger is the recovery truth; boot-reconcile fixes a live
flag left stale by a crash between the two.

## Projections

- **Store projection** (`repositories/agentchat/internal/store`): folds
  `evt.Aggregate` into a read-model DB (`state/store/agent_chat.db`); serves
  `ListChats` / `GetChat` / segments-by-chat; rebuildable via `ax.Replay`.
  Emits the **same `dto/agent.go` shape → the frontend is unchanged.**
- **Hub projection**: subscribes to `agentchat.*`, broadcasts lifecycle frames
  — **replaces the manual `Broadcaster.BroadcastAgentChat` calls.** The FE gets
  working/idle + switch + rename live off the event stream, exactly as
  `ReviewThread` / `Workspace` do.

## Boot reconcile (the "is it working" wrinkle)

`Working` / segment-`active` are **live subprocess state**; a crash is not an
event in the log, so naive replay shows a chat stuck `Working` and a segment
stuck `active`. A **boot reactor** (mirrors `Workspace`'s boot-sweep): on
startup, for each chat whose active segment's `TerminalSessionID` is not a live
terminal session → emit `EndSegment`, and if `Working` emit
`StopTurn(interrupted)`. History (past turns/segments) stays pure replay; only
the live flags are reconciled.

## Deletes / cleanup

`Delete` tombstones (`Status=deleted`) → asynx `Forget` + a delete reactor that
terminates the active segment's PTY (like `Workspace`'s delete reactor
`rm -rf`'ing the worktree). No cascade *below* the chat — segments are embedded.

## Edges & lifecycle hazards

- **Hook → chat routing.** A hook carries a provider session id; resolve it to a
  chat via the read-model index on `ProviderSessionID` (the boot-seeded registry
  is the in-memory fast path), then `Get` that aggregate. Replaces today's
  `GetActiveSegmentByCrowbarID`.
- **Runtime process exit (not a switch).** A CLI crash/quit fires the
  `TerminalCommander` `onExit` callback → emit `EndSegment` (and `StopTurn` if
  `Working`). Boot-reconcile is only the restart-time counterpart of this same
  rule.
- **Concurrent switch / IO-before-command.** The PTY spawn happens *before* the
  `OpenSegment` command (IO cannot live in a pure command). If two switches
  race, the second `EndSegment`/`OpenSegment` loses OCC and its `Validate`
  rejects (nothing active to end / one already opening) → the usecase **tears
  down the just-spawned orphan CLI**. Rule: spawn → try-commit → on OCC-loss,
  kill the orphan.
- **Workspace delete cascades to its chats.** The `Workspace` delete reactor
  `Forget`s that workspace's `AgentChat`s and kills their live PTYs — same shape
  as its existing cascade to `ReviewThread`s.

## Volume / snapshots

Turn events fire at conversation cadence (comparable to `ReviewThread` replies).
Enable periodic snapshots via `Command.ShouldSnapshot()` (e.g. every N events)
so replay / read-model rebuild stays cheap for long-lived chats.

## What gets deleted vs added

**Deleted:** `usecases/agent/keyed_mutex.go`; `repositories/agentchat` gorm
mutation methods; manual `BroadcastAgentChat` calls; the gorm `agent_chats` /
`agent_segments` tables.

**Added** (the standard aggregate recipe — same as `Workspace` / `ReviewThread`):
`domain` aggregate (extended); ~8 commands under
`repositories/agentchat/internal/commands`; an OCC-retry repo wrapper; store +
hub projections; a per-type event-store DB (`state/events/agent_chat.db`) +
read-model DB; the boot-reconcile reactor; container/adapter wiring
(`axAgentChat = newAsynx[domain.AgentChat](es)`).

## Testing

- **Commands** unit-tested pure: `Validate` rejects a 2nd active segment;
  `EmitEvent` folds correctly.
- **Integration** (`api/tests/integration/agent`): real hooks → assert
  aggregate state via `SendWait` / `WaitQuiescent`; a switch is two atomic
  events; boot reconcile marks an orphaned turn `interrupted`. **No timing** —
  block on asynx signals, never sleeps. See [[feedback_no_timing_in_tests]].
- **Live**: `make dev-desktop`, a real Claude↔Codex switch — confirm the
  working/idle icon, the switch history, and rename. See
  [[feedback_verify_in_tauri_before_claiming]].

## Definition of done

- `go build -tags noEmbed ./...`, `go test` (+ `-race`) and lint green in
  touched packages; FE typecheck + tests green.
- `keyed_mutex.go` gone; `agentchat` is a standard asynx aggregate structurally
  matching `Workspace` / `ReviewThread`.
- Live switch + working-state + restart-reconcile verified in the dev Tauri app.

## Remaining judgment call

Turn-durability is now settled (content lives in the ledger; aggregate turn
events are only the working-state timeline). The one fork left is whether the
**ledger** stays a **separate append-only disk store** (recommended — it is
already event-log-shaped and content-heavy; duplicating it into asynx buys
nothing) or later becomes a **second asynx event stream**. Recommendation:
**keep it separate**; revisit only if the ledger itself needs to be
replayable/rebuildable through asynx. Everything else in this design is defined
at the design level — remaining unknowns (exact command signatures, projection
SQL, wiring) belong in the implementation *plan*, not here.
