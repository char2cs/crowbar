# WAVE 4 — API consolidation (single agent)

You are doing the **final API consolidation**: assemble the full `/v0` route
table, the seven concrete broadcasters, the dual-serve and snapshot-on-subscribe
behavior, and prove the backend end-to-end against the real frontend. Route
handlers were threaded through earlier waves; this wave makes `02` the canonical,
complete, consistent surface.

**Prerequisite:** Waves 0–3 landed (engines, app layer, hub projections green).

## Read first
- `api/docs/specs/v0/02-api-surface.md` (the canonical route index — primary)
- `api/docs/specs/v0/03-realtime-websockets.md` (all of it — the seven
  broadcasters, channel scoping, §1a snapshot-on-subscribe)
- `quiver.core` `internal/api/v0/` (router, `dispatch()`, broadcaster wiring)

## House rules
- Module `github.com/char2cs/crowbar/api`. Go 1.26.2. **Invoke `go-style` first.**
- Layered: this is the `api/` layer. Uniform response envelope
  `{ success, error, data? }`; all routes `/v0/*`; UUID path params.

## Build
1. **Router** (`api/v0/router.go`) — register **every** REST route from `02`
   §2 exactly (projects, repos, workspaces + hierarchy ops, chats-lifecycle,
   files, editor/LSP, git read/write, operation continue/abort, conflicts,
   review, provider read, search, terminal, health). No PR-create route.
2. **`dispatch()`** dual-serve on the marked routes (`GET /v0/workspaces`,
   `GET /v0/workspaces/:wsId/git/status`) — REST by default, WS on
   `Upgrade: websocket`.
3. **The seven `Broadcaster[T]` topics** with correct `StreamDef` scoping:
   Workspaces (**global**, `?projectId=`/`?repoId=` filters on the payload),
   Chats (`wsId`), Git (`wsId`), Files (`wsId`), LSP (`wsId`), Terminal
   (`sessionId`), ChatStream (`chatId`, **post-spike placeholder** — register the
   topic, no frames).
4. **Snapshot-on-subscribe** (`03` §1a) per topic: Workspaces/Chats/Git/LSP send
   current state (Workspaces computes the `agent-running`/`hasConflicts` overlays
   at snapshot time) under the registration lock; Files = no snapshot; Terminal =
   ring-buffer replay.
5. **Mount** everything in `internal.go`; confirm the lazy watcher/LSP refcount
   lifecycles trigger correctly on first/last subscription.

## Out of scope
Chat send path and the Agentic Bridge (`12`) — register the ChatStream topic but
implement no frames.

## Definition of done
- `go build ./...` + `go vet ./...` clean.
- Every `02` route resolves; the integration suite (`go test -tags integration
  -race ./tests/...`) is green.
- **End-to-end against the real frontend:** open the app → projects load →
  navigate a workspace → file tree + editor save → git status/diff/commit →
  terminal → live file-watch updates → workspace badges update over WS. The
  sidebar renders live on first connect (snapshot-on-subscribe working).

Report the final route count vs `02`, the e2e walkthrough result, and any spec
gap you hit during wiring.
