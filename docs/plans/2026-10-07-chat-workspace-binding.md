# Chat → workspace binding on realtime streams

## Problem

`crowbar-api` idled at ~190% CPU. A live `sample` put the time in JSON-decoding
the whole `nodes_read` table. Every debounced file-change event runs
`Container.PushFile` → `chatsHolding` → `worktree.ChatsForWorkspace`, which
rebuilds the whole chat/folder forest from SQLite just to stamp the event with
`ChatIDs`. PR #215 makes that rebuild linear instead of quadratic; it is still
a full table decode per event.

## Root cause

A file or git event is a fact about a **workspace**. The chat-scoped routes
(`/chats/:chatId/git/…`, `/chats/:chatId/files/ws`) mean "whichever workspace
my chat currently resolves to". That binding is dynamic but rare to change
(fork, move, worktree assigned, delete). The subscriber's predicate, however,
is compiled once at connect (`ws.BuildPredicate`), so it cannot follow the
binding. `ChatFanoutFilter` works around this by making the *publisher*
re-derive the binding on every event. Hot path pays for a cold fact.

## Design

The binding becomes per-connection state, resolved at connect and re-resolved
when placement changes. Events carry only `WsID`.

1. `ws` gains a bound filter: a `FilterDef` variant whose value is resolved
   per client by a `Resolve(chatID) wsID` hook and held in the client
   (atomic). The predicate compares `event.WsID` to the client's current
   binding: O(1) per client per event, no store access.
2. `Broadcaster.Rebind(resolveMany)` re-resolves every bound client. The
   resolver is one `ListChats` + one node read for *all* clients
   (`workspaceFor` with a shared memo already does this), so a placement
   change costs O(chats + nodes) once, independent of subscriber count.
3. `Rebind` is triggered by the facts that change a binding, each already
   emitted today: node placement events (`registerHubProjection`'s
   `NodeEvent`), chat created/deleted, and `SetWorkspace`. Triggers set a
   dirty flag on a single worker (latest-wins channel, no timer), so a burst
   of moves is one rebind.
4. When a client's binding changes, a full-state stream (git status) sends
   that client a fresh snapshot for the new workspace. Files is news, not
   state, and sends nothing.
5. Delete `FileChangeEvent.ChatIDs`, `GitStatusEvent.ChatIDs`, `chatsHolding`,
   and `ChatFanoutFilter`/`ExtractSet` if nothing else uses them (deadcode
   gate).

Why this shape: the placement write owns the fact that a binding changed, so
nothing is cached beside it (CLAUDE.md: no second copy of state). Rejected:
- Workspace→chats index projection: a second copy of placement state that
  must be kept consistent with the tree.
- TTL/memo of `ChatsForWorkspace`: a timer-shaped fix to a symptom.
- Columns + `WHERE parent_id` only: cheaper per call, still O(chats) of work
  per file event for nothing. Worth doing separately (below).

## Independent, do regardless

Make `nodes_read` queryable: real `kind`/`order` columns and
`ListByParent` as `WHERE parent_id = ?` on the existing index. Every other
`ListByParent` caller still decodes the whole table (the comment on `nodeRow`
assumes few siblings, which only holds if the query filters in SQL).

## Steps

1. Failing test: a chat-scoped client receives events for ws-a; the chat is
   moved under ws-b's owner; the same client now receives ws-b events and no
   ws-a events. Second: `PushFile` performs zero chat/node reads (counting
   fakes).
2. Bound filter + `Broadcaster.Rebind` + worker, unit tested in `ws`.
3. Wire files and git defs; trigger rebind from node events and chat
   create/delete/`SetWorkspace`; git snapshot on rebind.
4. Delete the old fan-out plumbing; update `shared_workspace.go` and
   `snapshots.go` only if they depended on it.
5. `TestRegression_*` for fork-after-connect (the case push-time resolution
   existed for).

## Open questions (not yet verified in code)

- Which chat events (create, delete, `SetWorkspace`) currently reach a hook
  the worker can subscribe to; node events do via `WatchFunc`.
- Review/search/identity streams are named in `ChatFanoutFilter`'s doc as
  sharing this bucket; a grep found only git and files using it. Confirm
  before deleting the helper.
- `snapshots.go` and `usecases/chat/internal/tree/shared_workspace.go` also
  call `ChatsForWorkspace`; neither is on the file-event path, left as is.
- A frame arriving between a placement write and the rebind uses the stale
  binding. Today's push-time resolution has the same window; confirm the
  watcher's next event or the rebind snapshot covers it.
