# Agents → Chats Re-architecture — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Take the agents/chats subsystem from its current state to the target
architecture, production ready — all nine stages of the spec's §8, end to end.

**Scope:** 23,900 production lines across 9 packages. The work is staged because each
stage must be independently green on the full CI gate: that is what makes a regression
bisectable and what lets the daemon keep running throughout. The staging is an
execution order, not a scope limit — this plan is done when stage 8 is committed and
every §9 success criterion holds.

**Architecture:** The engine gains what an engine should own — reading descriptors,
speaking a provider's protocol both ways, and the lifecycle of a live CLI with its
terminal — and the usecase shrinks to wiring the engine to asynx and to the frontend.
The descriptor becomes event-centric with per-event transport, so hooks/API/mixed
providers need no new concept and a new provider needs no Go.

**Tech Stack:** Go 1.x, `github.com/char2cs/asynx v0.8.0`, gorm/sqlite, gin,
`golangci-lint`, `go test -race`.

**Spec:** [`docs/superpowers/specs/2026-08-23-agents-chat-rearchitecture-design.md`](../specs/2026-08-23-agents-chat-rearchitecture-design.md)
— stages 0 and 1 of §8, satisfying §1.5 and success criteria §9 bullets 1 and 2.

## A note on the fanout's location

The spec (§8 stage 0) names the target `usecases/chat/internal/fanout`. This plan creates
it at **`usecases/agent/internal/fanout`** on purpose: the `agent` → `chat` rename is
stage 8, and moving the package twice would make stage 0 depend on a rename that has not
happened. The path is correct for now and moves with everything else in stage 8. Do not
"fix" it.

## Global Constraints

- All commands run from `api/`. Do **not** `cd` to the original repo root — this is a
  git worktree.
- Build tag `noEmbed` is required on every Go command. A plain `go test ./...` skips
  tagged files and can be green over broken code.
- **Never write a gate whose success message is unconditional.** `cmd | head && echo OK`
  reports the pipeline's status, not the tool's, and a `grep FAIL` followed by
  `echo "OK"` prints OK when it matched. Capture the exit code and quote the real line:

  ```bash
  go test -tags 'integration noEmbed' -race -timeout 600s -p 1 ./tests > /tmp/it.log 2>&1
  echo "EXIT=$?"; grep -E '^(--- FAIL|FAIL|ok|panic:)' /tmp/it.log
  ```

  This cost an hour in stage 1: a gate printed "INTEGRATION OK" while its own output
  said `FAIL .../api/tests 660.667s`.

- Full gate, all five must pass before any commit:
  - `go vet -tags noEmbed ./...`
  - `go vet -tags 'integration noEmbed' ./...` — **not optional.** `tests/` is behind
    the `integration` tag, so the untagged vet never compiles it. A stale import there
    took the integration suite red for a whole stage.
  - `go test -tags noEmbed -race ./...`
  - `make test-coverage` — hard floor **92%**, the build fails below it
  - `golangci-lint run --build-tags noEmbed ./...` — **zero new findings** vs HEAD.
    Four pre-existing `gocyclo`/`nestif` findings are accepted; anything else is yours.
- Integration gate before the final commit of each stage:
  `go test -tags 'integration noEmbed' -race -timeout 600s -p 1 ./tests ./internal/api/...`
- This is **pre-production**: rename outright, add no back-compat aliases, no
  deprecation shims.
- Do not push and do not open a PR. Local commits only.
- The git stash stack is shared with other worktrees. Never use bare `git stash` —
  use a WIP commit instead.

---

## File Structure

**Stage 0 — created:**

| file | responsibility |
|---|---|
| `internal/app/usecases/agent/internal/fanout/fanout.go` | Subscribes to the chat and runner watch seams; derives the WS frame and calls the hub. The only place agent lifecycle frames are produced. |
| `internal/app/usecases/agent/internal/fanout/fanout_test.go` | Table tests: every event kind produces the right frame; a forget produces `deleted`; a nil hub is a no-op. |

**Stage 0 — modified:**

| file | change |
|---|---|
| `internal/app/repositories/agentchat/internal/store/hub.go` | `BroadcastFunc` → `WatchFunc` carrying a repo-owned `ChatEvent`; the projector stops deriving `kind`/`working` for the wire and hands over the event. |
| `internal/app/repositories/agentchat/internal/store/store.go:98` | `New` takes `WatchFunc` instead of `BroadcastFunc`. |
| `internal/app/repositories/agentchat/event_store.go:30-33,215-226` | Re-export `WatchFunc`/`ChatEvent`; `NewEventSourced` signature follows. |
| `internal/app/repositories/agentrunner/…` | Same three changes, `RunnerEvent`. |
| `internal/app/repositories/container.go:141-149,197,225-226` | Drop the `hub.WebSocketHub` parameter entirely — it has exactly two uses, both agent broadcasts — and take two watch funcs instead. |
| `internal/app/container.go:118` | Build `fanout` over the hub and hand its watch funcs to `repositories.New`. |
| `internal/app/repositories/container_test.go` | Nine `repositories.New` call sites; broadcast-asserting tests keep their `captureHub` behind a real `fanout`. |

**Stage 1 — moved:** `internal/engine/terminal/**` → `internal/core/terminal/**`
(59 files reference it across 15 packages).

**Stage 1 — created:** `internal/engine/architecture_test.go` — the guard that no
`engine/X` imports `engine/Y`.

---

# STAGE 0 — Lift the WS broadcast out of the repository

## Task 1: Replace `BroadcastFunc` with a `Watch` seam in `agentchat`

**Files:**
- Modify: `internal/app/repositories/agentchat/internal/store/hub.go:55-110`
- Modify: `internal/app/repositories/agentchat/internal/store/store.go:94-110`
- Modify: `internal/app/repositories/agentchat/event_store.go:30-33` and the
  `NewEventSourced` signature at `:215-226`
- Test: `internal/app/repositories/agentchat/internal/store/hub_test.go`

**Interfaces:**
- Produces: `agentchat.ChatEvent{ChatID, WorkspaceID, Kind string; Working bool; Forgotten bool}`
  and `agentchat.WatchFunc func(ChatEvent)`.
  `agentchat.NewEventSourced(ax, es, storeDB, watch WatchFunc) (EventStore, error)`.
- Consumes: nothing from earlier tasks.

**Why the event type is repo-owned, not `asynxModels.Event`:** the usecase must not
learn the aggregate's asynx envelope to render a frame, and a repo-owned struct keeps
the seam testable without an asynx instance.

- [ ] **Step 1: Write the failing test**

Create `internal/app/repositories/agentchat/internal/store/hub_test.go`:

```go
package store

import (
	"context"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestHubProjector_OnEvent_EmitsRepoOwnedChatEvent(t *testing.T) {
	var got []ChatEvent
	p := &hubProjector{watch: func(e ChatEvent) { got = append(got, e) }}

	p.onEvent(context.Background(), asynxModels.Event[domain.AgentChat]{
		AggregateID: "chat-1",
		EventName:   "agentchat.turn_started.chat-1",
		Aggregate:   domain.AgentChat{ID: "chat-1", WorkspaceID: "ws-1", Working: true},
	})

	if len(got) != 1 {
		t.Fatalf("want 1 event, got %d", len(got))
	}
	want := ChatEvent{ChatID: "chat-1", WorkspaceID: "ws-1", Kind: "turn_started", Working: true}
	if got[0] != want {
		t.Fatalf("got %+v, want %+v", got[0], want)
	}
}

func TestHubProjector_OnForget_MarksForgotten(t *testing.T) {
	var got []ChatEvent
	p := &hubProjector{watch: func(e ChatEvent) { got = append(got, e) }}

	p.onForget(asynxModels.Event[domain.AgentChat]{
		Aggregate: domain.AgentChat{ID: "chat-9", WorkspaceID: "ws-1", Working: true},
	})

	if len(got) != 1 {
		t.Fatalf("want 1 event, got %d", len(got))
	}
	want := ChatEvent{ChatID: "chat-9", WorkspaceID: "ws-1", Kind: "deleted", Working: false, Forgotten: true}
	if got[0] != want {
		t.Fatalf("got %+v, want %+v", got[0], want)
	}
}

func TestHubProjector_NilWatch_DoesNotPanic(t *testing.T) {
	p := &hubProjector{}
	p.onEvent(context.Background(), asynxModels.Event[domain.AgentChat]{
		AggregateID: "chat-1",
		EventName:   "agentchat.created.chat-1",
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test -tags noEmbed -race ./internal/app/repositories/agentchat/internal/store/ -run TestHubProjector -v
```

Expected: FAIL to compile — `undefined: ChatEvent`, `unknown field watch`,
`p.onForget undefined`.

- [ ] **Step 3: Replace the broadcast type in `hub.go`**

In `internal/app/repositories/agentchat/internal/store/hub.go`, delete the
`BroadcastFunc` declaration at `:55-60` and put in its place:

```go
// ChatEvent is one projected agentchat lifecycle event, in repo-owned terms.
//
// Kind is the <kind> segment of the emitting command's EventName
// ("agentchat.<kind>.<id>"): created, segment_opened, segment_ended, session_bound,
// turn_started, turn_stopped, title_set, deleted.
//
// Working is the aggregate's OWN folded answer as of this event. It rides along
// because the alternative is a second implementation of the fold in TypeScript — and
// that is not hypothetical: the FE used to re-derive the spinner from Kind alone, so
// the instant "the turn ended" stopped meaning "the agent is done", the chat row went
// dark while the aggregate said Working.
type ChatEvent struct {
	ChatID      string
	WorkspaceID string
	Kind        string
	Working     bool
	Forgotten   bool
}

// WatchFunc receives every projected agentchat event. It replaces the former
// BroadcastFunc: the repository announces WHAT HAPPENED and the usecase decides what
// the frontend is told. Registering it is the repository's job only because asynx
// subscription is; deriving a wire frame is not.
type WatchFunc func(ChatEvent)
```

Change `registerHubProjection` to take `watch WatchFunc`, and replace its `OnForget`
body with a call to the new method:

```go
func registerHubProjection(
	ax asynx.Asynx[domain.AgentChat],
	watch WatchFunc,
) error {
	p := &hubProjector{watch: watch}
	if _, err := ax.Subscribe(asynx.Topic("agentchat.*"), p.onEvent); err != nil {
		return fmt.Errorf("agentchat hub projection: subscribe: %w", err)
	}
	// ax.Forget fires ONLY "asynx.aggregate.forget" via OnForget — it is not one of
	// the commands Subscribe's "agentchat.*" pattern matches — so without this a
	// forgotten chat would never reach any live client.
	if _, err := ax.OnForget(func(_ context.Context, evt asynxModels.Event[domain.AgentChat]) {
		p.onForget(evt)
	}); err != nil {
		return fmt.Errorf("agentchat hub projection: onforget: %w", err)
	}
	return nil
}

type hubProjector struct {
	watch WatchFunc
}

func (p *hubProjector) emit(e ChatEvent) {
	if p.watch == nil {
		return
	}
	p.watch(e)
}

func (p *hubProjector) onEvent(
	_ context.Context,
	evt asynxModels.Event[domain.AgentChat],
) {
	p.emit(ChatEvent{
		ChatID:      evt.AggregateID,
		WorkspaceID: evt.Aggregate.WorkspaceID,
		Kind:        eventKind(evt.EventName),
		Working:     evt.Aggregate.Working,
	})
}

// onForget announces a hard delete. A forgotten chat is not working: it is not
// anything.
func (p *hubProjector) onForget(evt asynxModels.Event[domain.AgentChat]) {
	p.emit(ChatEvent{
		ChatID:      evt.Aggregate.ID,
		WorkspaceID: evt.Aggregate.WorkspaceID,
		Kind:        "deleted",
		Forgotten:   true,
	})
}
```

Keep the existing `eventKind` helper and the `eventNamePrefix` const exactly as they
are.

- [ ] **Step 4: Bridge the production call site so the tree still compiles**

Task 1 on its own breaks the build: `repositories/container.go:197` passes
`h.BroadcastAgentChat`, whose signature no longer matches. Bridge it inline — task 4
deletes the bridge along with the hub parameter:

```go
	agentChat, err := agentchat.NewEventSourced(axAgentChat, adapters.AgentChatES(), adapters.AgentChatReadDB(),
		// Bridged inline until the fanout lands (task 4), which takes this decision
		// out of the repository layer entirely.
		func(e agentchat.ChatEvent) {
			h.BroadcastAgentChat(e.ChatID, e.WorkspaceID, e.Kind, e.Working && !e.Forgotten)
		})
```

Every stage must leave the tree green; a task that leaves it red is a task that cannot
be bisected past.

- [ ] **Step 5: Adapt the existing test doubles — do NOT rewrite their assertions**

Four fixtures take the old four-argument func. Each gets a `watch` adapter that reduces
a `ChatEvent` to the frame it already records, so this change is proven against the
existing suite:

| file | double | adapter |
|---|---|---|
| `internal/store/hub_test.go` | `captureHub` | `func (h *captureHub) watch(e ChatEvent)` |
| `internal/store/projections_test.go` | — | pass `h.watch` to `registerHubProjection` |
| `event_store_test.go` | `captureBroadcast` | `func (c *captureBroadcast) watch(e agentchat.ChatEvent)` |
| `usecases/agent/harness_test.go` | `fakeBroadcaster` | `func (f *fakeBroadcaster) watchAgentChat(e agentchat.ChatEvent)` |
| `usecases/agenttools/perf_test.go` | inline literal | `func(agentchat.ChatEvent) {}` |

`hub_test.go` is **229 lines with 10 existing tests** — append to it, never overwrite it.

- [ ] **Step 6: Thread the rename through `store.go` and `event_store.go`**

`internal/app/repositories/agentchat/internal/store/store.go` — change the `New`
parameter at `:98` from `broadcast BroadcastFunc` to `watch WatchFunc`, and the
`registerHubProjection(ax, broadcast)` call to `registerHubProjection(ax, watch)`.

`internal/app/repositories/agentchat/event_store.go` — replace the alias at `:30-33`:

```go
// WatchFunc and ChatEvent are aliases for the store-layer types, exposed so callers
// wire the watch seam without importing the internal store package.
type (
	WatchFunc = store.WatchFunc
	ChatEvent = store.ChatEvent
)
```

and change `NewEventSourced`'s last parameter from `broadcast BroadcastFunc` to
`watch WatchFunc`, passing it straight to `store.New`.

- [ ] **Step 7: Run the test to verify it passes**

```bash
go vet -tags noEmbed ./... && go test -tags noEmbed -race ./...
```

Expected: PASS everywhere — the three new `TestHubProjector_*` tests plus all 10
pre-existing ones, and no other package broken.

- [ ] **Step 8: Commit**

```bash
git add internal/app/repositories/agentchat/
git commit -m "refactor(agentchat): announce events, do not broadcast frames"
```

---

## Task 2: The same seam in `agentrunner`

**Files:**
- Modify: `internal/app/repositories/agentrunner/internal/store/hub.go`
- Modify: `internal/app/repositories/agentrunner/internal/store/store.go`
- Modify: `internal/app/repositories/agentrunner/event_store.go:32-37` and
  `NewEventSourced`
- Test: `internal/app/repositories/agentrunner/internal/store/hub_test.go`

**Interfaces:**
- Consumes: the pattern from Task 1 — same shape, different aggregate.
- Produces: `agentrunner.RunnerEvent{RunnerID, WorkspaceID, ChatID, Kind string}`
  and `agentrunner.WatchFunc func(RunnerEvent)`.
  `agentrunner.NewEventSourced(ax, es, storeDB, watch WatchFunc) (EventStore, error)`.

Note the shape differs from `ChatEvent` in two ways. A runner frame carries `ChatID`
(the chat it is placed on, empty while displaced) and has no `Working` — the work state
is the chat's fold, not the runner's. And it has no `Forgotten`: unlike `agentchat`,
the runner hub registers **no** `OnForget` handler today, and this task does not add
one.

The five real event kinds, from `internal/commands/*.go`: `started`, `session_bound`,
`moved`, `displaced`, `exited`.

- [ ] **Step 1: Write the failing test**

Create `internal/app/repositories/agentrunner/internal/store/hub_test.go`:

```go
package store

import (
	"context"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestHubProjector_OnEvent_EmitsRepoOwnedRunnerEvent(t *testing.T) {
	var got []RunnerEvent
	p := &hubProjector{watch: func(e RunnerEvent) { got = append(got, e) }}

	p.onEvent(context.Background(), asynxModels.Event[domain.AgentRunner]{
		AggregateID: "run-1",
		EventName:   "agentrunner.started.run-1",
		Aggregate: domain.AgentRunner{
			ID: "run-1", WorkspaceID: "ws-1", CurrentChatID: "chat-1",
		},
	})

	if len(got) != 1 {
		t.Fatalf("want 1 event, got %d", len(got))
	}
	want := RunnerEvent{RunnerID: "run-1", WorkspaceID: "ws-1", ChatID: "chat-1", Kind: "started"}
	if got[0] != want {
		t.Fatalf("got %+v, want %+v", got[0], want)
	}
}

func TestHubProjector_DisplacedRunner_CarriesEmptyChatID(t *testing.T) {
	var got []RunnerEvent
	p := &hubProjector{watch: func(e RunnerEvent) { got = append(got, e) }}

	p.onEvent(context.Background(), asynxModels.Event[domain.AgentRunner]{
		AggregateID: "run-2",
		EventName:   "agentrunner.displaced.run-2",
		Aggregate:   domain.AgentRunner{ID: "run-2", WorkspaceID: "ws-1"},
	})

	if got[0].ChatID != "" {
		t.Fatalf("a displaced runner must carry no chat id, got %q", got[0].ChatID)
	}
}

func TestHubProjector_NilWatch_DoesNotPanic(t *testing.T) {
	p := &hubProjector{}
	p.onEvent(context.Background(), asynxModels.Event[domain.AgentRunner]{AggregateID: "run-1"})
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test -tags noEmbed -race ./internal/app/repositories/agentrunner/internal/store/ -run TestHubProjector -v
```

Expected: FAIL to compile — `undefined: RunnerEvent`.

- [ ] **Step 3: Apply the Task 1 change shape to `agentrunner`**

In `hub.go`, replace `BroadcastFunc` with:

```go
// RunnerEvent is one projected agentrunner lifecycle event, in repo-owned terms.
// ChatID is the chat the runner is PLACED on, empty while it is displaced — pointed at
// nothing while its process finishes falling over.
type RunnerEvent struct {
	RunnerID    string
	WorkspaceID string
	ChatID      string
	Kind        string
}

// WatchFunc receives every projected agentrunner event. The repository announces what
// happened; the usecase decides what the frontend is told.
type WatchFunc func(RunnerEvent)
```

Give `hubProjector` a `watch WatchFunc` field and an `emit` / `onEvent` pair (no
`onForget` — this projector registers none today, and adding one is out of scope),
mapping `evt.Aggregate.CurrentChatID` onto `ChatID`.

Thread the rename through `store.New` and `event_store.go`'s alias and
`NewEventSourced`, exactly as in Task 1.

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test -tags noEmbed -race ./internal/app/repositories/agentrunner/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/repositories/agentrunner/
git commit -m "refactor(agentrunner): announce events, do not broadcast frames"
```

---

## Task 3: The `fanout` package

**Files:**
- Create: `internal/app/usecases/agent/internal/fanout/fanout.go`
- Test: `internal/app/usecases/agent/internal/fanout/fanout_test.go`

**Interfaces:**
- Consumes: `agentchat.ChatEvent` / `agentchat.WatchFunc` (Task 1),
  `agentrunner.RunnerEvent` / `agentrunner.WatchFunc` (Task 2).
- Produces:
  ```go
  func New(hub Hub) *Fanout
  func (f *Fanout) ChatWatch() agentchat.WatchFunc
  func (f *Fanout) RunnerWatch() agentrunner.WatchFunc

  type Hub interface {
      BroadcastAgentChat(chatID, workspaceID, kind string, working bool)
      BroadcastAgentRunner(runnerID, workspaceID, chatID, kind string)
  }
  ```

Both methods exist on the real hub with exactly these signatures — `hub.Hub` at
`internal/app/hub/hub.go:137` (`BroadcastAgentChat`) and `:254`
(`BroadcastAgentRunner`). They are currently passed as method values straight into the
event stores at `internal/app/repositories/container.go:197` and `:226`; this task's
`Hub` interface is the same two methods, so `*hub.Hub` satisfies it with no adapter.

- [ ] **Step 1: Write the failing test**

Create `internal/app/usecases/agent/internal/fanout/fanout_test.go`:

```go
package fanout_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/fanout"
)

type chatFrame struct {
	chatID, workspaceID, kind string
	working                   bool
}

type runnerFrame struct {
	runnerID, workspaceID, chatID, kind string
}

type spyHub struct {
	chats   []chatFrame
	runners []runnerFrame
}

func (h *spyHub) BroadcastAgentChat(chatID, workspaceID, kind string, working bool) {
	h.chats = append(h.chats, chatFrame{chatID, workspaceID, kind, working})
}

func (h *spyHub) BroadcastAgentRunner(runnerID, workspaceID, chatID, kind string) {
	h.runners = append(h.runners, runnerFrame{runnerID, workspaceID, chatID, kind})
}

func TestFanout_ChatEvent_ReachesHubAsAFrame(t *testing.T) {
	hub := &spyHub{}
	f := fanout.New(hub)

	f.ChatWatch()(agentchat.ChatEvent{
		ChatID: "chat-1", WorkspaceID: "ws-1", Kind: "turn_started", Working: true,
	})

	if len(hub.chats) != 1 {
		t.Fatalf("want 1 frame, got %d", len(hub.chats))
	}
	want := chatFrame{"chat-1", "ws-1", "turn_started", true}
	if hub.chats[0] != want {
		t.Fatalf("got %+v, want %+v", hub.chats[0], want)
	}
}

// A forgotten chat is announced as deleted and NOT working — the client drops it.
func TestFanout_ForgottenChat_IsNotWorking(t *testing.T) {
	hub := &spyHub{}
	f := fanout.New(hub)

	f.ChatWatch()(agentchat.ChatEvent{
		ChatID: "chat-9", WorkspaceID: "ws-1", Kind: "deleted", Working: true, Forgotten: true,
	})

	if hub.chats[0].working {
		t.Fatal("a forgotten chat must never be announced as working")
	}
	if hub.chats[0].kind != "deleted" {
		t.Fatalf("kind = %q, want deleted", hub.chats[0].kind)
	}
}

func TestFanout_RunnerEvent_ReachesHub(t *testing.T) {
	hub := &spyHub{}
	f := fanout.New(hub)

	f.RunnerWatch()(agentrunner.RunnerEvent{
		RunnerID: "run-1", WorkspaceID: "ws-1", ChatID: "chat-1", Kind: "spawned",
	})

	if len(hub.runners) != 1 {
		t.Fatalf("want 1 frame, got %d", len(hub.runners))
	}
}

// A daemon wired without a hub (every unit test) must not panic.
func TestFanout_NilHub_IsANoOp(t *testing.T) {
	f := fanout.New(nil)
	f.ChatWatch()(agentchat.ChatEvent{ChatID: "chat-1"})
	f.RunnerWatch()(agentrunner.RunnerEvent{RunnerID: "run-1"})
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test -tags noEmbed -race ./internal/app/usecases/agent/internal/fanout/ -v
```

Expected: FAIL — package `fanout` does not exist.

- [ ] **Step 3: Write the implementation**

Create `internal/app/usecases/agent/internal/fanout/fanout.go`:

```go
// Package fanout turns repository lifecycle announcements into the frames the
// frontend receives.
//
// It exists so the decision of what a client is told lives in the usecase layer and
// not inside an asynx projection. The repositories announce WHAT HAPPENED; this
// package is the single place that shapes those announcements into wire frames.
package fanout

import (
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
)

// Hub is the WS broadcaster as this package needs it.
type Hub interface {
	BroadcastAgentChat(chatID, workspaceID, kind string, working bool)
	BroadcastAgentRunner(runnerID, workspaceID, chatID, kind string)
}

// Fanout holds the hub the frames are sent to. A nil hub degrades to a no-op so the
// daemon never panics when wired without one (tests).
type Fanout struct {
	hub Hub
}

func New(hub Hub) *Fanout { return &Fanout{hub: hub} }

// ChatWatch is the seam agentchat.NewEventSourced is wired with.
func (f *Fanout) ChatWatch() agentchat.WatchFunc {
	return func(e agentchat.ChatEvent) {
		if f.hub == nil {
			return
		}
		// A forgotten chat is not working: it is not anything.
		working := e.Working && !e.Forgotten
		f.hub.BroadcastAgentChat(e.ChatID, e.WorkspaceID, e.Kind, working)
	}
}

// RunnerWatch is the seam agentrunner.NewEventSourced is wired with.
func (f *Fanout) RunnerWatch() agentrunner.WatchFunc {
	return func(e agentrunner.RunnerEvent) {
		if f.hub == nil {
			return
		}
		f.hub.BroadcastAgentRunner(e.RunnerID, e.WorkspaceID, e.ChatID, e.Kind)
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test -tags noEmbed -race ./internal/app/usecases/agent/internal/fanout/ -v
```

Expected: PASS, all four tests.

- [ ] **Step 5: Commit**

```bash
git add internal/app/usecases/agent/internal/fanout/
git commit -m "feat(agent): add the fanout package that shapes repo events into WS frames"
```

---

## Task 4: Rewire the containers and drop the hub from the repository layer

**Files:**
- Modify: `internal/app/repositories/container.go:141-149` (the `New` signature),
  `:197` (agentchat call), `:225-226` (agentrunner call)
- Modify: `internal/app/container.go:118` — build the fanout, pass its watch funcs
- Modify: `internal/app/repositories/container_test.go` — 8+ `repositories.New` call
  sites at `:180`, `:206`, `:426`, `:491`, `:599`, `:652`, `:699`, `:759`, `:812`
- Test: the existing tests in that file, plus `internal/app/shutdown_test.go`, must stay
  green

**Interfaces:**
- Consumes: `fanout.New`, `(*Fanout).ChatWatch`, `(*Fanout).RunnerWatch` (Task 3); the
  new `NewEventSourced` signatures (Tasks 1, 2).
- Produces: `repositories.New` **without** its `h hub.WebSocketHub` parameter, gaining
  `chatWatch agentchat.WatchFunc, runnerWatch agentrunner.WatchFunc` in its place.

**Two corrections found during execution — both matter:**

1. **The hub cannot be dropped from `repositories.New`.** It is also used for workspace
   frames at `container.go:177` (`workspace.RegisterHubProjection`) and `:504`
   (`BroadcastWorkspace`). Workspace is outside this subsystem's scope, so the hub
   parameter stays and only the two agent seams move. Task 5's guard is narrowed
   accordingly — it asserts the **agent** repositories never broadcast, not that no
   repository does.
2. **The fanout cannot live behind `usecases/agent/internal` and be reachable from the
   composition root.** Go's internal rule forbids it, and the repositories are
   constructed *before* the usecase, so they need their seams first. Expose it through
   the usecase's own public face — `agent.NewFanout(hub)` in
   `usecases/agent/fanout.go`, delegating to `internal/fanout`. The decision still
   lives in the usecase layer; the root just gets a door.

**A third, smaller one:** test call sites must pass **non-nil** no-op seams.
`agentrunner`'s store refuses a nil watch at construction (see task 2), so `nil, nil`
breaks every container in those files.

- [ ] **Step 1: Confirm the hub really has only those two uses**

```bash
grep -nE '\bh\.[A-Z]|\.hub\b' internal/app/repositories/container.go
```

Expected: two agent broadcasts (lines 197, 226) **plus** two workspace uses
(`c.hub.BroadcastWorkspace` at 177 and 504). Convert only the agent pair.

- [ ] **Step 2: Write the failing test**

Add to `internal/app/repositories/container_test.go`:

```go
// The repository container is built with watch seams, not a hub: deciding what the
// frontend is told is the usecase's job (spec 2026-08-23 §1.5). Nil seams must
// construct cleanly, because most callers are tests with no hub at all.
func TestNew_NilWatchSeams_Construct(t *testing.T) {
	ctx := t.Context()
	ad := newAdapter(t)

	c, err := repositories.New(
		ctx, ad, ax[domain.ReviewThread](t), wsAx(t, ad),
		agentChatAx(t, ad), agentActivityAx(t, ad), agentRunnerAx(t, ad),
		nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c == nil {
		t.Fatal("New returned a nil container with no error")
	}
}

// The watch seams the container is given actually receive events.
func TestNew_ChatWatchSeam_ReceivesEvents(t *testing.T) {
	ctx := t.Context()
	ad := newAdapter(t)

	var seen []agentchat.ChatEvent
	c, err := repositories.New(
		ctx, ad, ax[domain.ReviewThread](t), wsAx(t, ad),
		agentChatAx(t, ad), agentActivityAx(t, ad), agentRunnerAx(t, ad),
		func(e agentchat.ChatEvent) { seen = append(seen, e) }, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Create a chat through the real event store, exactly as the nearest existing
	// chat-creating test in this file does, then assert the seam fired.
	if _, err := c.AgentChat.SpawnChat(ctx, /* copy args from the nearest test */); err != nil {
		t.Fatalf("SpawnChat: %v", err)
	}
	if len(seen) == 0 {
		t.Fatal("the chat watch seam received nothing; the projection is not wired")
	}
	if seen[0].WorkspaceID == "" {
		t.Fatal("the seam fired but carried no workspace id — the WS filter needs it")
	}
}
```

The exact parameter list and the `SpawnChat` arguments must be copied from the existing
call at `:426` and the nearest chat-creating test in the same file — the container takes
positional arguments and this plan must not guess them.

- [ ] **Step 3: Run the test to verify it fails**

```bash
go test -tags noEmbed -race ./internal/app/repositories/ -run 'TestNew_NilWatchSeams|TestNew_ChatWatchSeam' -v
```

Expected: FAIL to compile — `New` still takes a hub.

- [ ] **Step 4: Change the repository container**

In `internal/app/repositories/container.go`, delete the `h hub.WebSocketHub` parameter
from `New` (at `:144`) and add, in the same position:

```go
	chatWatch agentchat.WatchFunc,
	runnerWatch agentrunner.WatchFunc,
```

Replace `h.BroadcastAgentChat` at `:197` with `chatWatch`, and `h.BroadcastAgentRunner`
at `:226` with `runnerWatch`. Remove the now-unused `hub` import.

Update the doc comments at `:192` and `:221` — both currently assert that
`h.BroadcastAgentChat` / `h.BroadcastAgentRunner` is "the SOLE source" of frames. That
statement is still true of the *fanout*, so rewrite them to name it:

```go
	// The chat watch seam is the SOLE source of agent-chat lifecycle frames: one
	// event → one announcement → one frame. The repository does not decide what the
	// frontend is told; usecases/agent/internal/fanout does.
```

- [ ] **Step 5: Change the app container**

In `internal/app/container.go`, find the `repositories.New(` call at `:118` and the hub
value passed to it. Immediately before that call:

```go
	agentFanout := fanout.New(h)
```

replacing `h` with whatever the hub variable is actually called there (find it with
`grep -n 'hub\|NewHub' internal/app/container.go`). Then replace the hub argument in
the `repositories.New` call with:

```go
		agentFanout.ChatWatch(),
		agentFanout.RunnerWatch(),
```

Add the fanout import. If the hub is constructed *after* line 118, move the
`fanout.New` call to just after the hub — do not move the hub itself, and do not
construct a second one.

- [ ] **Step 6: Update every test call site**

```bash
grep -n 'repositories.New(' internal/app/repositories/container_test.go
```

Nine call sites. In each, replace the hub argument (`&captureHub{}` or `hub.NewHub()`)
with two arguments: `nil, nil`, **except** in any test that asserts on broadcasts via
`captureHub.count()`, `captureHub.last()` or `captureHub.lastWorking()`. For those,
pass a watch func that feeds the same `captureHub` through a `fanout`:

```go
	h := &captureHub{}
	f := fanout.New(h)
	c, err := repositories.New(ctx, ad, /* … */, f.ChatWatch(), f.RunnerWatch(), nil, nil)
```

so the assertion still exercises the whole path, now including the fanout. Do not
weaken any assertion to make it pass.

- [ ] **Step 7: Run the full unit gate**

```bash
go vet -tags noEmbed ./... && go test -tags noEmbed -race ./...
```

Expected: PASS. `internal/app/shutdown_test.go` and the agent handler tests exercise
this wiring and must be green without modification.

- [ ] **Step 8: Run the integration gate**

```bash
go test -tags 'integration noEmbed' -race -timeout 600s -p 1 ./tests ./internal/api/...
```

Expected: PASS. This is the gate that proves a real WS client still receives chat frames
through the new path — it is the only test that covers the seam end to end.

- [ ] **Step 9: Run coverage and lint**

```bash
make test-coverage
golangci-lint run --build-tags noEmbed ./...
```

Expected: coverage ≥ 92%; lint findings no worse than HEAD (four pre-existing
`gocyclo`/`nestif`).

- [ ] **Step 10: Commit**

```bash
git add internal/app/container.go internal/app/repositories/container.go internal/app/repositories/container_test.go
git commit -m "refactor(app): route agent WS frames through the usecase fanout"
```

---

## Task 5: Prove the repositories no longer reach the frontend

**Files:**
- Create: `internal/app/repositories/architecture_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: a standing guard. This is the test that makes stage 0 permanent — without
  it, the next person adds a broadcast back into a projection and nothing complains.

- [ ] **Step 1: Write the failing test**

Create `internal/app/repositories/architecture_test.go`:

```go
package repositories_test

import (
	"go/build"
	"path/filepath"
	"strings"
	"testing"
)

// The repository layer announces events. It must never reach the frontend: WS fan-out
// is the usecase's decision (spec 2026-08-23 §1.5). This guard is the reason that
// stays true.
func TestRepositories_DoNotImportRealtimeOrHub(t *testing.T) {
	const module = "github.com/char2cs/crowbar/api/internal/app/repositories"
	forbidden := []string{
		"github.com/char2cs/crowbar/api/internal/app/realtime",
		"github.com/char2cs/crowbar/api/internal/api",
	}

	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}

	for _, pkgDir := range goPackageDirs(t, root) {
		pkg, err := build.ImportDir(pkgDir, 0)
		if err != nil {
			continue // no buildable Go files here
		}
		for _, imp := range pkg.Imports {
			for _, bad := range forbidden {
				if imp == bad || strings.HasPrefix(imp, bad+"/") {
					rel, _ := filepath.Rel(root, pkgDir)
					t.Errorf("%s/%s imports %s — the repository layer must not reach the frontend",
						module, rel, imp)
				}
			}
		}
	}
}
```

Add the directory walker in the same file, and add `"os"` to the import block:

```go
func goPackageDirs(t *testing.T, root string) []string {
	t.Helper()
	var dirs []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return dirs
}
```

- [ ] **Step 2: Run the test**

```bash
go test -tags noEmbed -race ./internal/app/repositories/ -run TestRepositories_DoNotImportRealtime -v
```

Expected: PASS after Tasks 1–4. If it FAILS, an import you missed is still there — fix
the import, do not weaken the test.

- [ ] **Step 3: Prove the guard is not vacuous**

Temporarily add `_ "github.com/char2cs/crowbar/api/internal/app/realtime"` to
`internal/app/repositories/agentchat/event_store.go`, re-run the test, and confirm it
now FAILS naming that file. Then remove the import and confirm it passes again. A guard
that has never failed has not been tested.

- [ ] **Step 4: Commit**

```bash
git add internal/app/repositories/architecture_test.go
git commit -m "test(repositories): guard that the repo layer never reaches the frontend"
```

---

# STAGE 1 — Promote `terminal` to `core/`

## Task 6: Move `engine/terminal` → `core/terminal`

**Files:**
- Move: `internal/engine/terminal/**` → `internal/core/terminal/**` (the whole tree,
  including `internal/persistence`, `internal/registry`, `internal/session`)
- Modify: every file importing the old path — 59 files across 15 packages
- Modify: `internal/engine/container.go` — drop the terminal field, or keep it as a
  pass-through if other engines take it as a dependency

**Interfaces:**
- Consumes: nothing from stage 0.
- Produces: the import path `github.com/char2cs/crowbar/api/internal/core/terminal`.
  Every exported symbol keeps its name; only the path changes.

**Why:** `engine/terminal` has 8 distinct consumers and 7,229 lines — larger than the
agents engine. `core/` already holds packages with 1–19 consumers, so terminal beats
`core/config` (2), `core/gateway` (2), `core/paths` (1) and `core/shellenv` (1) by the
repo's own standard. Until it moves, `engine/agents` cannot own a PTY without a
cross-engine import (spec §2 rule 2).

- [ ] **Step 1: Record the baseline**

```bash
go test -tags noEmbed -race ./... 2>&1 | tail -20 > /tmp/baseline.txt
grep -rl 'engine/terminal' internal --include='*.go' | sort > /tmp/importers.txt
wc -l /tmp/importers.txt
```

Expected: 59 files. Keep both files — step 4 diffs against them.

- [ ] **Step 2: Move the tree and rewrite every import**

```bash
git mv internal/engine/terminal internal/core/terminal
grep -rl 'api/internal/engine/terminal' internal --include='*.go' \
  | xargs sed -i '' 's|api/internal/engine/terminal|api/internal/core/terminal|g'
```

- [ ] **Step 3: Fix the engine container**

```bash
grep -n 'terminal' internal/engine/container.go
```

If the container holds a `Terminal` field only so other layers can reach it, delete the
field and have `app/container.go` construct `core/terminal` directly. If another engine
genuinely takes it as a constructor dependency, leave that dependency — it is now a
`core/` import, which is allowed.

- [ ] **Step 4: Verify nothing else changed**

```bash
go build -tags noEmbed ./... && grep -rl 'engine/terminal' internal --include='*.go' | wc -l
```

Expected: build succeeds, and the grep count is **0**.

- [ ] **Step 5: Run the full unit gate**

```bash
go vet -tags noEmbed ./... && go test -tags noEmbed -race ./...
```

Expected: PASS, and the same test count as `/tmp/baseline.txt`. A *lower* count means a
test file was left behind by the move — find it before continuing.

- [ ] **Step 6: Commit**

```bash
git add -A internal/engine internal/core internal/app internal/api
git commit -m "refactor(terminal): promote engine/terminal to core/terminal"
```

Use `git add -A` restricted to those four paths — the move touches 59 files and listing
them individually is error-prone. Do **not** use a bare `git add -A`: other sessions
share this worktree.

---

## Task 7: The cross-engine import guard

**Files:**
- Create: `internal/engine/architecture_test.go`

**Interfaces:**
- Consumes: the completed move from Task 6.
- Produces: the standing guard behind spec §9 bullet 1.

- [ ] **Step 1: Write the failing test**

Create `internal/engine/architecture_test.go`:

```go
package engine_test

import (
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const enginePrefix = "github.com/char2cs/crowbar/api/internal/engine/"

// No engine may import another engine's capability. A capability two engines need is
// promoted to core/ (spec 2026-08-23 §2 rule 2). engine/container.go is the single
// composition root and is exempt: it is the package that wires them together.
func TestEngines_DoNotCrossImport(t *testing.T) {
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue // engine/container.go itself is the exempt composition root
		}
		own := enginePrefix + e.Name()
		walkGoPackages(t, filepath.Join(root, e.Name()), func(dir string, pkg *build.Package) {
			for _, imp := range pkg.Imports {
				if !strings.HasPrefix(imp, enginePrefix) {
					continue
				}
				if imp == own || strings.HasPrefix(imp, own+"/") {
					continue // importing your own subtree is fine
				}
				rel, _ := filepath.Rel(root, dir)
				t.Errorf("engine/%s imports %s — engines must not cross-import; promote the shared capability to core/", rel, imp)
			}
		})
	}
}

func walkGoPackages(t *testing.T, root string, fn func(string, *build.Package)) {
	t.Helper()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		pkg, perr := build.ImportDir(path, 0)
		if perr != nil {
			return nil // no buildable Go files in this directory
		}
		fn(path, pkg)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run the test**

```bash
go test -tags noEmbed -race ./internal/engine/ -run TestEngines_DoNotCrossImport -v
```

Expected: PASS. If it FAILS, the failure names a real violation Task 6 did not clear —
fix the import by promoting the shared package to `core/`, never by adding an exemption.

- [ ] **Step 3: Prove the guard is not vacuous**

Temporarily add `_ "github.com/char2cs/crowbar/api/internal/engine/git"` to any file
under `internal/engine/agents/`, re-run, and confirm it FAILS naming `engine/agents`.
Remove the import and confirm it passes. Do not commit the temporary import.

- [ ] **Step 4: Run the full gate**

```bash
go vet -tags noEmbed ./... \
  && go test -tags noEmbed -race ./... \
  && go test -tags 'integration noEmbed' -race -timeout 600s -p 1 ./tests ./internal/api/... \
  && make test-coverage \
  && golangci-lint run --build-tags noEmbed ./...
```

Expected: all five green; coverage ≥ 92%.

- [ ] **Step 5: Commit**

```bash
git add internal/engine/architecture_test.go
git commit -m "test(engine): guard that engines never cross-import"
```

---

## Stage 0 & 1 done criteria

- [ ] `grep -rl 'engine/terminal' internal --include='*.go'` returns nothing.
- [ ] `grep -rn 'BroadcastFunc' internal/app/repositories/` returns nothing.
- [ ] Both architecture guards pass, and both were observed to FAIL when their
      invariant was temporarily inverted.
- [ ] The full gate is green. Seven commits, nothing pushed.

---

# STAGE 2 — Descriptor v3

The descriptor becomes event-centric, the canonical vocabulary becomes data, and every
event gets a recorded fixture. Nothing outside `engine/agents/internal/descriptor`
changes shape in this stage — the existing `spec.Descriptor` consumers keep compiling
because v3 loads into the same struct until stage 3 replaces it.

## Task 8: The canonical vocabulary as data

**Files:**
- Create: `internal/engine/agents/internal/descriptor/vocabulary.yaml`
- Create: `internal/engine/agents/internal/descriptor/internal/schema/schema.go`
- Test: `internal/engine/agents/internal/descriptor/internal/schema/schema_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  ```go
  type Vocabulary struct{ Events map[string]EventRule }
  type EventRule struct {
      Direction string   // "in" | "out" | "ask"
      Required  []string // canonical field names that must be mapped
      Optional  []string
  }
  func Load() (Vocabulary, error)                       // reads the embedded vocabulary.yaml
  func (v Vocabulary) Validate(providerID string, events map[string]map[string]string) error
  ```

**Why:** event names are Go constants today (`spec.HookSessionStart` …
`internal/spec/hooks.go:4-24`) and per-event requirements are hardcoded in
`internal/descriptor/internal/rules/hook_vocabulary.go`. While that is true, a third
provider cannot be added without writing Go. The vocabulary is Crowbar-owned and
**closed**: a provider maps into it and cannot extend it.

- [ ] **Step 1: Write `vocabulary.yaml` from the existing constants**

Read the current authority first so nothing is dropped:

```bash
sed -n '1,30p' internal/engine/agents/internal/spec/hooks.go
cat internal/engine/agents/internal/descriptor/internal/rules/hook_vocabulary.go
```

Create `internal/engine/agents/internal/descriptor/vocabulary.yaml`. Every event listed
in `hooks.go` must appear, plus `compact_start` (new, spec §4.5). The `required:` lists
come from `hook_vocabulary.go` — do not invent requirements it does not state:

```yaml
# Crowbar's canonical conversation vocabulary. CLOSED: a provider descriptor maps INTO
# these names and cannot add to them. Adding an entry here is a Crowbar capability
# change and needs Go on the consuming side; adding a provider does not.
version: 3

events:
  session_start: { direction: in,  required: [session_id],        optional: [model] }
  user_prompt:   { direction: in,  required: [],                  optional: [message] }
  turn_stop:     { direction: in,  required: [message],           optional: [session_id] }
  turn_failed:   { direction: in,  required: [reason],            optional: [session_id] }
  tool_pre:      { direction: in,  required: [],                  optional: [session_id, tool_id, tool_name, tool_target, tool_input] }
  tool_post:     { direction: in,  required: [],                  optional: [session_id, tool_id, tool_name, tool_target, tool_input, tool_result, duration_ms] }
  tool_fail:     { direction: in,  required: [],                  optional: [session_id, tool_id, tool_name, tool_target, tool_input, tool_result, tool_error, duration_ms] }
  subagent_pre:  { direction: in,  required: [],                  optional: [session_id, subagent_id, agent_type] }
  subagent_post: { direction: in,  required: [],                  optional: [session_id, subagent_id, agent_type] }
  notification:  { direction: in,  required: [],                  optional: [session_id, message] }
  message_delta: { direction: in,  required: [message_id, index, text], optional: [session_id] }
  session_end:   { direction: in,  required: [],                  optional: [session_id] }
  telemetry:     { direction: in,  required: [],                  optional: [input_tokens, output_tokens, context_window, cost] }
  compact_pre:   { direction: in,  required: [],                  optional: [session_id, trigger] }
  compact_post:  { direction: in,  required: [],                  optional: [session_id, trigger, turn_id] }

  permission:    { direction: ask, required: [prompt_id],         optional: [session_id, message, tool_name, tool_target, tool_input, suggestions, questions] }
  elicitation:   { direction: ask, required: [prompt_id],         optional: [session_id, message, schema] }

  compact_start: { direction: out, required: [],                  optional: [session_id] }
  interrupt:     { direction: out, required: [],                  optional: [session_id] }
  prompt:        { direction: out, required: [text],              optional: [session_id] }
```

`session_start` requiring `session_id` and `turn_stop` requiring `message` are the two
rules `hook_vocabulary.go` enforces today; `message_delta`'s three are its third. Carry
them exactly.

- [ ] **Step 2: Write the failing test**

Create `internal/engine/agents/internal/descriptor/internal/schema/schema_test.go`:

```go
package schema_test

import (
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor/internal/schema"
)

func TestLoad_HasTheCanonicalEvents(t *testing.T) {
	v, err := schema.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, name := range []string{
		"session_start", "turn_stop", "message_delta", "permission",
		"elicitation", "compact_pre", "compact_post", "compact_start",
	} {
		if _, ok := v.Events[name]; !ok {
			t.Errorf("vocabulary is missing %q", name)
		}
	}
}

func TestValidate_MissingRequiredField_IsRejected(t *testing.T) {
	v, err := schema.Load()
	if err != nil {
		t.Fatal(err)
	}
	// session_start must map session_id.
	err = v.Validate("acme", map[string]map[string]string{
		"session_start": {"model": "model"},
	})
	if err == nil {
		t.Fatal("want an error for session_start with no session_id, got nil")
	}
	if !strings.Contains(err.Error(), "session_id") {
		t.Fatalf("error must name the missing field, got: %v", err)
	}
	if !strings.Contains(err.Error(), "acme") {
		t.Fatalf("error must name the provider, got: %v", err)
	}
}

func TestValidate_UnknownEvent_IsRejected(t *testing.T) {
	v, _ := schema.Load()
	err := v.Validate("acme", map[string]map[string]string{
		"invent_a_new_event": {"session_id": "session_id"},
	})
	if err == nil {
		t.Fatal("the vocabulary is CLOSED; an unknown event must be rejected")
	}
}

func TestValidate_UnknownFieldWithinAKnownEvent_IsRejected(t *testing.T) {
	v, _ := schema.Load()
	err := v.Validate("acme", map[string]map[string]string{
		"session_start": {"session_id": "session_id", "not_a_field": "x"},
	})
	if err == nil {
		t.Fatal("a field outside required+optional must be rejected, or typos map silently to nothing")
	}
}

func TestValidate_AProviderMappingOnlySomeEvents_IsAccepted(t *testing.T) {
	v, _ := schema.Load()
	// Capability is key-presence: declaring no telemetry is legal, not an error.
	if err := v.Validate("acme", map[string]map[string]string{
		"session_start": {"session_id": "session_id"},
		"turn_stop":     {"message": "msg"},
	}); err != nil {
		t.Fatalf("a partial provider must be accepted, got: %v", err)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

```bash
go test -tags noEmbed -race ./internal/engine/agents/internal/descriptor/internal/schema/ -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 4: Write the implementation**

Create `internal/engine/agents/internal/descriptor/internal/schema/schema.go`:

```go
// Package schema loads Crowbar's canonical conversation vocabulary and validates a
// provider descriptor's event table against it.
//
// The vocabulary is data (../../vocabulary.yaml) rather than Go constants so that
// adding a PROVIDER needs no Go. It is closed: adding an EVENT is a Crowbar capability
// change and does need Go on the consuming side.
package schema

import (
	_ "embed"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed ../../vocabulary.yaml
var vocabularyYAML []byte

type EventRule struct {
	Direction string   `yaml:"direction"`
	Required  []string `yaml:"required"`
	Optional  []string `yaml:"optional"`
}

type Vocabulary struct {
	Version int                  `yaml:"version"`
	Events  map[string]EventRule `yaml:"events"`
}

func Load() (Vocabulary, error) {
	var v Vocabulary
	if err := yaml.Unmarshal(vocabularyYAML, &v); err != nil {
		return Vocabulary{}, fmt.Errorf("schema: vocabulary: %w", err)
	}
	if len(v.Events) == 0 {
		return Vocabulary{}, fmt.Errorf("schema: vocabulary declares no events")
	}
	return v, nil
}

// Validate checks one provider's event table. A provider may map a subset of the
// vocabulary — capability is key-presence — but every event it DOES map must be a
// known one, must carry that event's required fields, and must not carry a field the
// event does not declare.
func (v Vocabulary) Validate(providerID string, events map[string]map[string]string) error {
	for _, name := range sortedKeys(events) {
		rule, ok := v.Events[name]
		if !ok {
			return fmt.Errorf("%s: unknown event %q: the vocabulary is closed", providerID, name)
		}
		fields := events[name]
		for _, req := range rule.Required {
			if fields[req] == "" {
				return fmt.Errorf("%s: event %q must map %q", providerID, name, req)
			}
		}
		allowed := make(map[string]bool, len(rule.Required)+len(rule.Optional))
		for _, f := range rule.Required {
			allowed[f] = true
		}
		for _, f := range rule.Optional {
			allowed[f] = true
		}
		for _, got := range sortedKeys(fields) {
			if !allowed[got] {
				return fmt.Errorf("%s: event %q declares no field %q", providerID, name, got)
			}
		}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
```

Sorted iteration matters: without it the error a bad descriptor produces changes between
runs and the test that asserts on it is flaky.

- [ ] **Step 5: Run the test to verify it passes**

```bash
go test -tags noEmbed -race ./internal/engine/agents/internal/descriptor/... -v
```

Expected: PASS, all five schema tests.

- [ ] **Step 6: Commit**

```bash
git add internal/engine/agents/internal/descriptor/
git commit -m "feat(descriptor): declare the canonical event vocabulary as data"
```

---

## Task 9: One path grammar

**Files:**
- Create: `internal/engine/agents/internal/descriptor/internal/mapping/mapping.go`
- Test: `internal/engine/agents/internal/descriptor/internal/mapping/mapping_test.go`
- Delete (at the end of stage 3, not now): `internal/engine/agents/internal/payload/`
  and `internal/catalog/internal/adapters/jsonpath.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  ```go
  func String(doc map[string]any, expr string) string
  func Int(doc map[string]any, expr string) (int, bool)
  func Bool(doc map[string]any, expr string) (bool, bool)
  func JSON(doc map[string]any, expr string) []byte
  func Objects(doc map[string]any, expr string) []map[string]any
  func Match(doc map[string]any, when map[string]string) bool
  ```

**Why:** there are two independent path resolvers today — `internal/payload/payload.go`
(`walk`) and `internal/catalog/internal/adapters/jsonpath.go` (`selectPath`,
`lookupField`). One grammar, one implementation. Alternation is currently
comma-overloaded (`a,b,c` = first non-empty); v3 makes it explicit `a || b || c` so a
path containing a comma stops being unrepresentable.

- [ ] **Step 1: Write the failing test**

Create `internal/engine/agents/internal/descriptor/internal/mapping/mapping_test.go`:

```go
package mapping_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor/internal/mapping"
)

func doc() map[string]any {
	return map[string]any{
		"session_id": "s-1",
		"tool_input": map[string]any{
			"command":   "ls -la",
			"file_path": "",
		},
		"turn": map[string]any{"lastAgentMessage": "done"},
		"item": map[string]any{"type": "commandExecution", "id": "i-1"},
		"usage": map[string]any{"inputTokens": float64(1200)},
		"questions": []any{
			map[string]any{"header": "Pick", "multiSelect": true},
		},
	}
}

func TestString_WalksADottedPath(t *testing.T) {
	if got := mapping.String(doc(), "turn.lastAgentMessage"); got != "done" {
		t.Fatalf("got %q, want done", got)
	}
}

func TestString_AlternationTakesTheFirstNonEmpty(t *testing.T) {
	got := mapping.String(doc(), "tool_input.file_path || tool_input.command")
	if got != "ls -la" {
		t.Fatalf("got %q, want the first NON-EMPTY branch", got)
	}
}

func TestString_AlternationSkipsMissingAsWellAsEmpty(t *testing.T) {
	got := mapping.String(doc(), "nope.nothing || tool_input.command")
	if got != "ls -la" {
		t.Fatalf("got %q, want ls -la", got)
	}
}

func TestString_MissingPathIsEmptyNotAPanic(t *testing.T) {
	if got := mapping.String(doc(), "a.b.c.d"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

// A literal comma in a path must survive — this is what comma-overloading broke.
func TestString_CommaIsNotAnOperator(t *testing.T) {
	d := map[string]any{"a,b": "kept"}
	if got := mapping.String(d, "a,b"); got != "kept" {
		t.Fatalf("got %q, want kept", got)
	}
}

func TestInt_ReadsANumericLeaf(t *testing.T) {
	got, ok := mapping.Int(doc(), "usage.inputTokens")
	if !ok || got != 1200 {
		t.Fatalf("got (%d,%v), want (1200,true)", got, ok)
	}
}

func TestObjects_ReadsAnArrayOfObjects(t *testing.T) {
	got := mapping.Objects(doc(), "questions")
	if len(got) != 1 || got[0]["header"] != "Pick" {
		t.Fatalf("got %+v", got)
	}
}

func TestMatch_SelectsOnAVariantField(t *testing.T) {
	if !mapping.Match(doc(), map[string]string{"item.type": "commandExecution"}) {
		t.Fatal("want a match on item.type")
	}
	if mapping.Match(doc(), map[string]string{"item.type": "fileChange"}) {
		t.Fatal("must not match a different variant")
	}
}

func TestMatch_AlternationInTheWhenValue(t *testing.T) {
	when := map[string]string{"item.type": "fileChange || commandExecution"}
	if !mapping.Match(doc(), when) {
		t.Fatal("a when: value may alternate; commandExecution is in the set")
	}
}

func TestMatch_EmptyWhenMatchesEverything(t *testing.T) {
	if !mapping.Match(doc(), nil) {
		t.Fatal("an event with no when: applies unconditionally")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test -tags noEmbed -race ./internal/engine/agents/internal/descriptor/internal/mapping/ -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the implementation**

Create `internal/engine/agents/internal/descriptor/internal/mapping/mapping.go`. Port
the typed readers from `internal/payload/payload.go` (`String`, `Int`, `Float`, `Bool`,
`Time`, `JSON`, `Objects`, `Object`, `Scalar`, `Count`) verbatim — they are correct and
tested — but replace their single-path `walk` with an alternation-aware resolver:

```go
// Package mapping is the one path grammar a descriptor writes.
//
// A path is dot-separated. `a || b` is alternation: the first branch that resolves to
// a non-empty value wins. Comma is NOT an operator — a key containing a comma is
// addressable, which the previous comma-overloaded alternation made impossible.
package mapping

import "strings"

const altSep = "||"

// resolve returns the first branch of expr that yields a present, non-empty value.
func resolve(doc map[string]any, expr string) (any, bool) {
	for _, branch := range strings.Split(expr, altSep) {
		v, ok := walk(doc, strings.TrimSpace(branch))
		if ok && !isEmpty(v) {
			return v, true
		}
	}
	return nil, false
}

// walk resolves ONE dotted path with no alternation.
func walk(doc map[string]any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	// Whole-key match first, so a key containing dots or commas stays addressable.
	if v, ok := doc[path]; ok {
		return v, true
	}
	var cur any = doc
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[seg]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	default:
		return false
	}
}

// Match reports whether every when: clause holds. An empty when matches everything.
func Match(doc map[string]any, when map[string]string) bool {
	for path, want := range when {
		got := String(doc, path)
		if !inAlternation(got, want) {
			return false
		}
	}
	return true
}

func inAlternation(got, want string) bool {
	for _, opt := range strings.Split(want, altSep) {
		if got == strings.TrimSpace(opt) {
			return true
		}
	}
	return false
}
```

Then add the typed accessors (`String`, `Int`, `Bool`, `JSON`, `Objects`, …) on top of
`resolve`, copying the coercion behaviour of `payload.go` exactly — JSON numbers arrive
as `float64` and `Int` must accept that.

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test -tags noEmbed -race ./internal/engine/agents/internal/descriptor/internal/mapping/ -v
```

Expected: PASS, all ten tests.

- [ ] **Step 5: Prove the ported readers still satisfy the old suite**

```bash
go test -tags noEmbed -race ./internal/engine/agents/... -v 2>&1 | tail -20
```

Expected: PASS. `internal/payload` is still present and still passing — it is deleted in
stage 3, once its callers move.

- [ ] **Step 6: Commit**

```bash
git add internal/engine/agents/internal/descriptor/internal/mapping/
git commit -m "feat(descriptor): one path grammar with explicit alternation and when:"
```

---

## Task 10: The v3 descriptor loader

**Files:**
- Modify: `internal/engine/agents/internal/spec/descriptor.go` — add the v3 shape
- Modify: `internal/engine/agents/internal/descriptor/descriptor.go` — load + validate
- Test: `internal/engine/agents/internal/descriptor/descriptor_v3_test.go`

**Interfaces:**
- Consumes: `schema.Load`, `schema.Validate` (Task 8); `mapping` (Task 9).
- Produces: `spec.Descriptor` gains
  ```go
  ProtocolVersion *VersionRange   `yaml:"protocol_version"`
  Runtime         RuntimeSpec     `yaml:"runtime"`
  Events          map[string]EventSpec `yaml:"events"`
  Catalog         map[string]CallSpec  `yaml:"catalog"`
  Inject          []InjectSpec         `yaml:"inject"`

  type EventSpec struct {
      In        string            `yaml:"in"`
      Out       string            `yaml:"out"`
      Ask       string            `yaml:"ask"`
      Transport string            `yaml:"transport"`   // overrides runtime.transport
      When      map[string]string `yaml:"when"`
      Map       map[string]string `yaml:"map"`
      Send      map[string]string `yaml:"send"`
      Reply     map[string]string `yaml:"reply"`
      TimeoutSeconds int          `yaml:"timeout_seconds"`
  }
  ```

- [ ] **Step 1: Write the failing test**

Create `internal/engine/agents/internal/descriptor/descriptor_v3_test.go`:

```go
package descriptor_test

import (
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor"
)

const minimalV3 = `
id: acme
display_name: Acme
protocol_version: { min: "1.0", max: "1.9" }
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [acme, serve]
  spawn:
    cmd: acme
events:
  session_start:
    in: thread/started
    map: { session_id: thread.id }
  turn_stop:
    in: turn/completed
    map: { message: turn.lastAgentMessage }
  permission:
    ask: approval/request
    timeout_seconds: 270
    map: { prompt_id: "$rpc.id" }
    reply: { allow: '{"decision":"approved"}' }
  compact_start:
    out: thread/compact/start
    send: { threadId: "{session_id}" }
`

func TestParseV3_LoadsEventsWithDirections(t *testing.T) {
	d, err := descriptor.ParseV3([]byte(minimalV3))
	if err != nil {
		t.Fatalf("ParseV3: %v", err)
	}
	if d.Events["session_start"].In != "thread/started" {
		t.Errorf("session_start.in = %q", d.Events["session_start"].In)
	}
	if d.Events["compact_start"].Out != "thread/compact/start" {
		t.Errorf("compact_start.out = %q", d.Events["compact_start"].Out)
	}
	if d.Events["permission"].Ask != "approval/request" {
		t.Errorf("permission.ask = %q", d.Events["permission"].Ask)
	}
}

func TestParseV3_RejectsAnEventOutsideTheVocabulary(t *testing.T) {
	bad := strings.Replace(minimalV3, "  session_start:", "  not_an_event:", 1)
	if _, err := descriptor.ParseV3([]byte(bad)); err == nil {
		t.Fatal("the vocabulary is closed; an unknown event must be rejected at load")
	}
}

func TestParseV3_RejectsAMissingRequiredField(t *testing.T) {
	bad := strings.Replace(minimalV3, "map: { message: turn.lastAgentMessage }", "map: {}", 1)
	if _, err := descriptor.ParseV3([]byte(bad)); err == nil {
		t.Fatal("turn_stop must map message")
	}
}

// Per-event transport is what makes a MIXED provider possible.
func TestParseV3_PerEventTransportOverridesTheRuntimeDefault(t *testing.T) {
	mixed := strings.Replace(minimalV3,
		"  compact_start:\n    out: thread/compact/start",
		"  compact_start:\n    transport: hooks\n    out: thread/compact/start", 1)
	d, err := descriptor.ParseV3([]byte(mixed))
	if err != nil {
		t.Fatalf("ParseV3: %v", err)
	}
	if got := d.TransportFor("compact_start"); got != "hooks" {
		t.Fatalf("TransportFor(compact_start) = %q, want hooks", got)
	}
	if got := d.TransportFor("session_start"); got != "api" {
		t.Fatalf("TransportFor(session_start) = %q, want the runtime default api", got)
	}
}

func TestParseV3_RejectsAnUnsupportedProtocolVersion(t *testing.T) {
	if err := descriptor.CheckProtocolVersion(
		mustParse(t, minimalV3), "2.4",
	); err == nil {
		t.Fatal("2.4 is outside [1.0,1.9] and must be refused at load, not at 3am")
	}
	if err := descriptor.CheckProtocolVersion(
		mustParse(t, minimalV3), "1.5",
	); err != nil {
		t.Fatalf("1.5 is in range: %v", err)
	}
}

func mustParse(t *testing.T, y string) *spec.Descriptor {
	t.Helper()
	d, err := descriptor.ParseV3([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	return d
}
```

Add the `spec` import for `mustParse`'s return type.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test -tags noEmbed -race ./internal/engine/agents/internal/descriptor/ -run V3 -v
```

Expected: FAIL — `descriptor.ParseV3` undefined.

- [ ] **Step 3: Add the v3 types to `spec`**

Add the `EventSpec`, `RuntimeSpec`, `APISpec`, `HooksSpec`, `CallSpec`, `InjectSpec` and
`VersionRange` structs listed under **Interfaces** above to
`internal/engine/agents/internal/spec/descriptor.go`, alongside the existing v2 fields.
Both shapes coexist until stage 3 deletes v2.

Add the transport resolver as a method on `*spec.Descriptor`:

```go
// TransportFor returns the transport an event uses: its own if it declares one, the
// runtime default otherwise. This is what makes a MIXED provider — API for turns,
// hooks for permissions — need no new concept.
func (d *Descriptor) TransportFor(event string) string {
	if e, ok := d.Events[event]; ok && e.Transport != "" {
		return e.Transport
	}
	return d.Runtime.Transport
}
```

- [ ] **Step 4: Write `ParseV3` and `CheckProtocolVersion`**

In `internal/engine/agents/internal/descriptor/descriptor.go`:

```go
// ParseV3 unmarshals a v3 descriptor and validates its event table against Crowbar's
// canonical vocabulary. Validation happens at LOAD so a bad descriptor fails when the
// daemon starts, never mid-conversation.
func ParseV3(raw []byte) (*spec.Descriptor, error) {
	var d spec.Descriptor
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("descriptor: parse: %w", err)
	}
	if d.ID == "" {
		return nil, fmt.Errorf("descriptor: missing id")
	}
	v, err := schema.Load()
	if err != nil {
		return nil, err
	}
	maps := make(map[string]map[string]string, len(d.Events))
	for name, e := range d.Events {
		maps[name] = e.Map
	}
	if err := v.Validate(d.ID, maps); err != nil {
		return nil, fmt.Errorf("descriptor: %w", err)
	}
	return &d, nil
}

// CheckProtocolVersion refuses a provider CLI whose protocol is outside the range the
// descriptor was written against. app-server is experimental and its method names move;
// this is the gate that turns that into a startup failure instead of a runtime one.
func CheckProtocolVersion(d *spec.Descriptor, actual string) error {
	if d.ProtocolVersion == nil || actual == "" {
		return nil // a provider that declares no range accepts any version
	}
	if semverLess(actual, d.ProtocolVersion.Min) || semverLess(d.ProtocolVersion.Max, actual) {
		return fmt.Errorf(
			"descriptor %s: provider protocol %s is outside the supported range [%s, %s]",
			d.ID, actual, d.ProtocolVersion.Min, d.ProtocolVersion.Max)
	}
	return nil
}
```

Write `semverLess` as a dotted numeric compare — the versions here are `major.minor`,
not full semver, and a `strings.Compare` gets `1.10 < 1.9` wrong.

- [ ] **Step 5: Run the test to verify it passes**

```bash
go test -tags noEmbed -race ./internal/engine/agents/internal/descriptor/ -v
```

Expected: PASS, all five v3 tests plus the existing suite.

- [ ] **Step 6: Commit**

```bash
git add internal/engine/agents/internal/spec/ internal/engine/agents/internal/descriptor/
git commit -m "feat(descriptor): v3 loader — event-centric, vocabulary-validated, version-gated"
```

---

## Task 11: Capture real fixtures

**Files:**
- Create: `internal/engine/agents/internal/protocol/testdata/fixtures/codex/*.json`
- Create: `internal/engine/agents/internal/protocol/testdata/fixtures/claude/*.json`
- Create: `internal/engine/agents/internal/descriptor/fixture_test.go`
- Create: `scripts/capture-codex-fixtures.sh`

**Interfaces:**
- Consumes: `ParseV3` (Task 10), `mapping` (Task 9).
- Produces: a replay test that fails when a provider changes payload shape.

**Why this task is not optional:** synthetic fixtures have previously hidden real output
shape in this repo. Every payload here must come **from the CLI**, not from a hand-typed
guess. The spec's §10 open question 2 — the unverified nested leaf paths — is closed
here or not at all.

- [ ] **Step 1: Write the capture script**

Create `scripts/capture-codex-fixtures.sh`:

```bash
#!/usr/bin/env bash
# Captures REAL codex app-server traffic into fixture files. Run manually; the output is
# committed. Never hand-write a file in the fixtures directory.
set -euo pipefail
OUT="${1:?usage: capture-codex-fixtures.sh <outdir>}"
mkdir -p "$OUT"

codex app-server generate-json-schema --out "$OUT/schema"

python3 - "$OUT" <<'PY'
import json, subprocess, sys, threading, pathlib
out = pathlib.Path(sys.argv[1])
p = subprocess.Popen(["codex","app-server","--listen","stdio://"],
                     stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True, bufsize=1)
def send(obj):
    p.stdin.write(json.dumps(obj)+"\n"); p.stdin.flush()

frames = []
def reader():
    for line in p.stdout:
        try: frames.append(json.loads(line))
        except ValueError: pass
t = threading.Thread(target=reader, daemon=True); t.start()

send({"jsonrpc":"2.0","id":1,"method":"initialize",
      "params":{"clientInfo":{"name":"crowbar-fixtures","title":"fixtures","version":"0.0.1"}}})
send({"jsonrpc":"2.0","id":2,"method":"thread/start","params":{"cwd":str(out.resolve())}})
t.join(timeout=30)
p.kill()

# One file per distinct method, so a fixture is addressable by the event that maps it.
seen = {}
for f in frames:
    key = f.get("method") or f"response-{f.get('id')}"
    seen.setdefault(key.replace("/","_"), f)
for name, frame in seen.items():
    (out / f"{name}.json").write_text(json.dumps(frame, indent=2) + "\n")
print(f"captured {len(seen)} frames into {out}")
PY
```

- [ ] **Step 2: Run it and inspect what came back**

```bash
chmod +x scripts/capture-codex-fixtures.sh
./scripts/capture-codex-fixtures.sh internal/engine/agents/internal/protocol/testdata/fixtures/codex
ls internal/engine/agents/internal/protocol/testdata/fixtures/codex/
```

Read the captured `thread_started.json`. **Its real field paths are the authority** —
if `thread.id` is not where the descriptor says, fix the descriptor, not the fixture.
This is the step that closes spec §10 question 2.

- [ ] **Step 3: Capture the Claude side**

Claude's hook payloads arrive over HTTP at the daemon, not from a probe, so capture them
from a live session instead: start the daemon with
`CROWBAR_HOOK_CAPTURE=<dir>` (add the env read to the hook handler in this step, ~6
lines: if the var is set, write each raw body to `<dir>/<canonical>.json` before
ingesting), drive one real Claude chat through a prompt, a tool call, a permission and a
`/compact`, then commit what lands. Remove nothing from the captured JSON.

- [ ] **Step 4: Write the replay test**

Create `internal/engine/agents/internal/descriptor/fixture_test.go`:

```go
package descriptor_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor/internal/mapping"
)

// Every event a descriptor maps must resolve its REQUIRED fields against real recorded
// traffic. This is the test that catches a provider changing payload shape — nothing
// else does.
func TestDescriptors_RequiredFieldsResolveAgainstRealFixtures(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			d, err := descriptor.Resolve(t.Context(), t.TempDir(), provider)
			if err != nil {
				t.Fatalf("resolve %s: %v", provider, err)
			}
			dir := filepath.Join("..", "protocol", "testdata", "fixtures", provider)

			for name, ev := range d.Events {
				wire := ev.In
				if wire == "" {
					wire = ev.Ask
				}
				if wire == "" {
					continue // outbound events have no inbound payload to replay
				}
				path := filepath.Join(dir, fixtureName(wire)+".json")
				raw, err := os.ReadFile(path)
				if os.IsNotExist(err) {
					t.Errorf("event %q maps wire event %q but no fixture exists at %s",
						name, wire, path)
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				var doc map[string]any
				if err := json.Unmarshal(raw, &doc); err != nil {
					t.Fatalf("%s: %v", path, err)
				}
				for field, expr := range ev.Map {
					if expr == "" {
						continue
					}
					if mapping.String(doc, expr) == "" {
						t.Errorf("%s/%s: %q → %q resolved to nothing against the real payload",
							provider, name, field, expr)
					}
				}
			}
		})
	}
}
```

Add a `fixtureName` helper that applies the same `/`→`_` transform the capture script
uses.

- [ ] **Step 5: Run the replay test**

```bash
go test -tags noEmbed -race ./internal/engine/agents/internal/descriptor/ -run Fixtures -v
```

Expected: FAIL at first, naming every path the descriptor got wrong. **Fix the
descriptor until it passes. Never edit a fixture.**

- [ ] **Step 6: Commit**

```bash
git add scripts/capture-codex-fixtures.sh internal/engine/agents/internal/protocol/testdata/ internal/engine/agents/internal/descriptor/
git commit -m "test(descriptor): replay real provider traffic against every mapped event"
```

---

## Task 12: Rewrite both descriptors in v3

**Files:**
- Rewrite: `internal/engine/agents/internal/descriptor/descriptors/claude.yaml`
- Rewrite: `internal/engine/agents/internal/descriptor/descriptors/codex.yaml`

**Interfaces:**
- Consumes: everything in tasks 8–11.
- Produces: two v3 descriptors that pass the vocabulary validator and the fixture replay.

- [ ] **Step 1: Rewrite `codex.yaml`**

Use spec §4.3 verbatim as the starting point, then correct every path the Task 11
fixtures disagree with. Keep the `icon:` inline. Keep `spawn.forbid_flags`.

- [ ] **Step 2: Rewrite `claude.yaml`**

Use spec §4.4 as the shape, and carry across **every** mapping the current v2 file has —
the `permission` block alone maps 17 fields including the `suggestion_label.*` lookup
table and the `AskUserQuestion` vocabulary. Losing one is a silent capability
regression, so diff the field lists before and after:

```bash
git show HEAD:api/internal/engine/agents/internal/descriptor/descriptors/claude.yaml \
  | grep -oE '^\s+[a-z_.]+:' | tr -d ' :' | sort -u > /tmp/v2fields.txt
grep -oE '^\s+[a-z_.]+:' internal/engine/agents/internal/descriptor/descriptors/claude.yaml \
  | tr -d ' :' | sort -u > /tmp/v3fields.txt
diff /tmp/v2fields.txt /tmp/v3fields.txt
```

Every line only in v2 must be either present in v3 or deliberately dropped with a reason
stated in the commit message.

- [ ] **Step 3: Run the full descriptor suite**

```bash
go test -tags noEmbed -race ./internal/engine/agents/... -v
```

Expected: PASS, including the fixture replay for both providers.

- [ ] **Step 4: Run the full gate**

```bash
go vet -tags noEmbed ./... \
  && go test -tags noEmbed -race ./... \
  && go test -tags 'integration noEmbed' -race -timeout 600s -p 1 ./tests ./internal/api/... \
  && make test-coverage \
  && golangci-lint run --build-tags noEmbed ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/engine/agents/internal/descriptor/descriptors/
git commit -m "feat(descriptor): migrate claude and codex to the v3 event-centric schema"
```

## Stage 2a — delete v2 (done)

Once v3 shipped and was daemon-verified, the v2 shape came out entirely rather than
being left as an inert second path:

- `descriptors/*.yaml`, `migration_test.go`, `rules/hook_vocabulary.go` and
  `schema/real_descriptors_test.go` deleted
- `spec.HookSpec` and `spec.AnswerSpec` deleted; the unified accessors lost their v2
  branches and `IsV3` with them
- 14 test files' YAML fixtures rewritten to the event table by a scripted transform
  (`/tmp/v2to3.py`), not by hand
- `ParseV3`'s missing-id error now wraps `ErrInvalid`, preserving the sentinel contract
  that `Resolve` callers switch on — caught by an existing test, not by review

No v2 compatibility shim was added on purpose. Accepting v2 YAML would mean a new
descriptor could skip vocabulary validation entirely, which is the one thing the
validation exists to prevent.

## Stage 2 done criteria

- [ ] `vocabulary.yaml` is the only place canonical event names are declared.
- [ ] One path resolver; `a || b` works and a comma-bearing key is addressable.
- [ ] Both descriptors load, validate, and replay against **captured** fixtures.
- [ ] `compact_start` exists in the vocabulary and in both descriptors.
- [ ] Spec §10 question 2 is closed — no unverified leaf path remains.

---

# STAGE 3 — Build `protocol/`

Fold the 16 leaf packages into one system with one public face. The engine is still
stateless at the end of this stage; nothing owns a PTY yet.

## Task 13: The `transport` layer

**Files:**
- Create: `internal/engine/agents/internal/protocol/internal/transport/transport.go`
- Create: `.../transport/internal/hooks/hooks.go` — HTTP in, response body out
- Create: `.../transport/internal/jsonrpc/jsonrpc.go` — duplex over stdio/unix/ws
- Create: `.../transport/internal/oneshot/oneshot.go` — spawn, read stdout, exit
- Test: one `_test.go` beside each

**Interfaces:**
- Consumes: `spec.RuntimeSpec` (Task 10).
- Produces:
  ```go
  type Transport interface {
      Start(ctx context.Context) error
      // Recv delivers every inbound frame: (wireEvent, rawPayload, correlationID).
      // correlationID is non-empty only for a frame that BLOCKS awaiting a reply.
      Recv() <-chan Frame
      Send(ctx context.Context, wireEvent string, payload []byte) error
      Reply(ctx context.Context, correlationID string, payload []byte) error
      Close() error
  }
  type Frame struct {
      WireEvent     string
      Raw           []byte
      CorrelationID string
  }
  func New(rt spec.RuntimeSpec, kind string) (Transport, error)  // kind: hooks|api|oneshot
  ```

**Why three implementations behind one interface:** `oneshot` replaces the `probe/`
package the spec explicitly does not create — a slash-catalog or telemetry probe is the
same descriptor entry and the same translate path, only a different byte-mover.

- [ ] **Step 1: Write the failing test for `jsonrpc`**

Create `.../transport/internal/jsonrpc/jsonrpc_test.go`. Drive a fake server over an
`io.Pipe` pair rather than a real process:

```go
func TestJSONRPC_Notification_ArrivesAsAFrameWithNoCorrelationID(t *testing.T) {
	// server writes: {"method":"turn/completed","params":{"threadId":"t-1"}}
	// assert: Frame{WireEvent:"turn/completed", CorrelationID:""}
}

func TestJSONRPC_ServerRequest_CarriesACorrelationID(t *testing.T) {
	// server writes: {"id":7,"method":"item/permissions/requestApproval","params":{}}
	// assert: Frame.CorrelationID == "7" — a request BLOCKS and must be replyable
}

func TestJSONRPC_Reply_WritesAResponseWithTheMatchingID(t *testing.T) {
	// Reply(ctx, "7", []byte(`{"decision":"approved"}`))
	// assert the server reads {"jsonrpc":"2.0","id":7,"result":{"decision":"approved"}}
}

func TestJSONRPC_Send_WritesARequestAndDoesNotBlockOnTheResponse(t *testing.T) {
	// Send must not deadlock when the server is slow: the turn loop cannot stall
	// on an unread response.
}

func TestJSONRPC_ServerClosesMidStream_ClosesRecvNotPanics(t *testing.T) {
}
```

Write each body out in full following the pattern above; the assertions named in the
comments are the contract.

- [ ] **Step 2: Run to verify it fails, then implement `jsonrpc`**

```bash
go test -tags noEmbed -race ./internal/engine/agents/internal/protocol/... -v
```

Implement newline-delimited JSON-RPC 2.0: a read pump turning each line into a `Frame`,
a write path for `Send`, and an id-keyed `Reply`. The correlation id is the JSON-RPC
`id` rendered as a string. Guard the writer with a mutex — two goroutines reply
concurrently whenever two prompts are open at once.

- [ ] **Step 3: Implement `hooks` and `oneshot` the same way**

`hooks`: the daemon's HTTP handler pushes a `Frame` in and blocks on `Reply` to produce
the response body — so `CorrelationID` is the hook's delivery id.
`oneshot`: `Start` spawns, `Recv` emits exactly one `Frame` carrying stdout, `Close`
reaps. Port the process handling from `internal/exec/runner.go` verbatim, including its
`waitDelay` and `Acquire` limiter — both are load-bearing.

- [ ] **Step 4: Commit**

```bash
git add internal/engine/agents/internal/protocol/internal/transport/
git commit -m "feat(protocol): three transports behind one Frame interface"
```

---

## Task 14: The `translate` layer

**Files:**
- Create: `internal/engine/agents/internal/protocol/internal/translate/translate.go`
- Create: `.../translate/internal/inbound/inbound.go`
- Create: `.../translate/internal/outbound/outbound.go`
- Create: `.../translate/internal/answer/answer.go`
- Test: one beside each

**Interfaces:**
- Consumes: `spec.Descriptor` (Task 10), `mapping` (Task 9).
- Produces:
  ```go
  func Inbound(d *spec.Descriptor, wireEvent string, raw []byte) (agents.Fact, bool, error)
  func Outbound(d *spec.Descriptor, in agents.Intent) (wireEvent string, payload []byte, error)
  func Reply(d *spec.Descriptor, canonical string, raw []byte, dec agents.Decision) ([]byte, error)
  ```
  The `bool` from `Inbound` is false when no event matches the wire event or its `when:`
  fails — an unmapped frame is ignored, not an error.

**Port, don't rewrite:** `inbound` is `internal/hooks/hooks.go` + `internal/telemetry` +
`internal/termprompt` reading from `EventSpec.Map` instead of `HookSpec.Events`.
`answer` is `internal/answers/answers.go` reading `EventSpec.Reply` instead of
`AnswerSpec`. Keep their behaviour identical — the existing tests in those packages are
the regression suite and must be moved across, not deleted.

- [ ] **Step 1: Move the existing tests first**

```bash
git mv internal/engine/agents/internal/hooks/hooks_test.go \
       internal/engine/agents/internal/protocol/internal/translate/internal/inbound/inbound_test.go
git mv internal/engine/agents/internal/hooks/choice_test.go \
       internal/engine/agents/internal/protocol/internal/translate/internal/inbound/choice_test.go
git mv internal/engine/agents/internal/answers/answers_test.go \
       internal/engine/agents/internal/protocol/internal/translate/internal/answer/answer_test.go
```

Fix their package clauses and imports. They will not compile yet — that is the failing
test for this task.

- [ ] **Step 2: Run to verify they fail**

```bash
go test -tags noEmbed -race ./internal/engine/agents/internal/protocol/... 2>&1 | head -20
```

Expected: compile errors naming the functions that do not exist yet.

- [ ] **Step 3: Add the variant-dispatch test the old suite could not express**

```go
// Codex's `item` is a sum type. One wire event serves three canonical events, selected
// by when: — this is the case the v2 schema had no way to state.
func TestInbound_WhenSelectsAmongEventsSharingAWireEvent(t *testing.T) {
	// descriptor: tool_pre { in: item/started, when: {item.type: commandExecution} }
	//             message_delta { in: item/started, when: {item.type: agentMessage} }
	// payload with item.type=agentMessage must produce message_delta, NOT tool_pre.
}

func TestInbound_NoMatchingWhen_IsIgnoredNotAnError(t *testing.T) {
	// item.type=reasoning matches nothing → (Fact{}, false, nil)
}
```

- [ ] **Step 4: Implement, keeping the ported behaviour byte-identical**

- [ ] **Step 5: Prove purity**

```go
// translate must be a pure function: no clock, no network, no state. Calling it twice
// with the same input must produce the same output, and it must be safe under -race
// from many goroutines.
func TestInbound_IsPureAndConcurrencySafe(t *testing.T) {
	// run 100 goroutines × the same fixture, assert every result is identical
}
```

- [ ] **Step 6: Run the full engine suite and commit**

```bash
go test -tags noEmbed -race ./internal/engine/agents/... -v
git add internal/engine/agents/
git commit -m "feat(protocol): translate — pure payload↔Fact in both directions"
```

---

## Task 15: The `Protocol` face and the great deletion

**Files:**
- Create: `internal/engine/agents/internal/protocol/protocol.go`
- Modify: `internal/engine/agents/agents.go`, `types.go`, `aliases.go`
- Delete: `internal/engine/agents/internal/{answers,catalog,env,exec,hooks,models,move,payload,promptorigin,registry,selection,spawn,spec,telemetry,template,termprompt}`

**Interfaces:**
- Produces:
  ```go
  type Protocol interface {
      Capabilities() agents.Capabilities
      Recv(wireEvent string, raw []byte) (agents.Fact, bool, error)
      Send(ctx context.Context, in agents.Intent) error
      Reply(ctx context.Context, correlationID string, dec agents.Decision) error
      Close() error
  }
  func Load(ctx context.Context, homeDir, providerID string) (Protocol, error)
  ```

- [ ] **Step 1: Write the failing test**

```go
// The runner holds ONE Protocol and never learns that descriptor, transport and
// translate are separate packages.
func TestLoad_ReturnsAWorkingProtocolForEachShippedProvider(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		p, err := protocol.Load(t.Context(), t.TempDir(), id)
		if err != nil { t.Fatalf("%s: %v", id, err) }
		if !p.Capabilities().Observes { t.Errorf("%s declares no inbound events", id) }
	}
}

// Capability is key-presence, with no flag that can drift from the descriptor.
func TestCapabilities_DeriveFromTheEventTableAlone(t *testing.T) {
	// a descriptor with no telemetry event → Capabilities().Telemetry == false
	// adding the event flips it, with no other edit
}
```

- [ ] **Step 2: Implement `protocol.Load`** — resolve the descriptor, build the
  transports named by `TransportFor`, and return a struct that wires `Recv` through
  `translate.Inbound` and `Send`/`Reply` through `translate.Outbound`/`Reply`.

- [ ] **Step 3: Repoint `agents.go` at `protocol` and delete the 16 packages**

```bash
git rm -r internal/engine/agents/internal/{answers,catalog,env,exec,hooks,models,move,payload,promptorigin,registry,selection,spawn,telemetry,template,termprompt}
```

`spec` is deleted last, after its types move to `protocol/internal/descriptor`. Move
`aliases.go`'s type aliases into `types.go` — the whole point of `aliases.go` was
re-exporting `models`, which no longer exists.

- [ ] **Step 4: Run the full gate**

```bash
go vet -tags noEmbed ./... \
  && go test -tags noEmbed -race ./... \
  && go test -tags 'integration noEmbed' -race -timeout 600s -p 1 ./tests ./internal/api/... \
  && make test-coverage && golangci-lint run --build-tags noEmbed ./...
```

Expected: green. Coverage will move — if it drops below 92, the deleted packages took
tested code with them and the replacement needs the equivalent tests, not a lowered
floor.

- [ ] **Step 5: Commit**

```bash
git add -A internal/engine/agents/
git commit -m "refactor(agents): one protocol face; fold 16 leaf packages into it"
```

## Stage 3 done criteria

- [ ] `ls internal/engine/agents/internal/` shows only `protocol/`.
- [ ] `Protocol` is the only thing `runner/` will need in stage 5.
- [ ] Coverage ≥ 92% without changing the floor.

---

# STAGE 4 — The runner aggregate moves into the engine

## Task 16: `domain.AgentRunner` → `agents.Runner`

**Files:**
- Modify: `internal/domain/agent_runner.go` → delete, type moves to
  `internal/engine/agents/types.go`
- Modify: every reference (find with `grep -rl 'domain.AgentRunner' internal`)

- [ ] **Step 1: Count the blast radius**

```bash
grep -rl 'domain.AgentRunner' internal --include='*.go' | tee /tmp/runner-refs.txt | wc -l
```

- [ ] **Step 2: Move the type verbatim**, preserving every doc comment — they encode
  hard-won facts (`ExitedAt` is audit only, not a liveness flag; `LaunchModel` is the
  ONLY authority on what the CLI is running).

- [ ] **Step 3: Rewrite the references**

```bash
xargs sed -i '' 's|domain\.AgentRunner|agents.Runner|g' < /tmp/runner-refs.txt
```

Then fix imports by hand — `goimports` cannot add `engine/agents` to a file that has no
other engine import.

- [ ] **Step 4: Build, test, commit**

```bash
go build -tags noEmbed ./... && go test -tags noEmbed -race ./...
git add -A && git commit -m "refactor(agents): the runner type belongs to the engine that owns it"
```

## Task 17: Move the aggregate

**Files:**
- Move: `internal/app/repositories/agentrunner/**` →
  `internal/engine/agents/internal/runner/internal/store/**`
- Modify: `internal/app/container.go` — inject `asynx.Asynx[agents.Runner]` into
  `agents.New` instead of into `repositories.New`
- Modify: `internal/app/repositories/container.go` — drop the runner store

**Interfaces:**
- Produces: `agents.New(ax asynx.Asynx[agents.Runner], es asynxModels.Store, db *gormdb.DB, watch RunnerWatch) (Agents, error)`

- [ ] **Step 1: Write the failing architecture test**

```go
// The engine declares the port; the app builds the instance. engine/ must never import
// the sqlite adapter, or it stops being testable against an in-memory asynx.
func TestEngine_DoesNotImportTheEventStoreAdapter(t *testing.T) {
	// walk internal/engine, fail on any import of internal/adapter/eventstore
}
```

Add it to `internal/engine/architecture_test.go` (Task 7).

- [ ] **Step 2: Move the tree**

```bash
git mv internal/app/repositories/agentrunner internal/engine/agents/internal/runner/internal/store
```

Rewrite the package path in every importer. The `agentrunner.ErrNotFound` sentinel
becomes `agents.ErrRunnerNotFound` and must stay a distinct sentinel — `LiveRunnerForChat`
treats it as "dormant", not "failed", and collapsing it into a generic error makes every
dormant chat look broken.

- [ ] **Step 3: Rewire both containers, run the full gate, commit**

## Stage 4 done criteria

- [ ] `internal/app/repositories/agentrunner` does not exist.
- [ ] `engine/` imports no `adapter/eventstore/*` — guarded by a test.
- [ ] `ErrRunnerNotFound` is still a distinct sentinel and dormant chats still resolve.

---

# STAGE 5 — The in-flight tier moves into the engine

The largest step. Every invariant in spec §7 gets its inverting test **here**.

## Task 18: `runner/internal/inflight`

**Files:**
- Move into `internal/engine/agents/internal/runner/internal/inflight/`:
  `gate.go`, `turns.go`, `answers.go`, `message_stream.go`, `message.go`,
  `pending_hooks.go`, and `adapter/store/agentjournal/*`
- Test: `inflight_test.go` plus one file per invariant

**Interfaces:**
- Produces:
  ```go
  type Inflight struct{ … }
  func New() *Inflight
  func (i *Inflight) Gate(space, key string) (release func())
  func (i *Inflight) OpenTurn(chatID string) 
  func (i *Inflight) CompleteTurn(chatID string)
  func (i *Inflight) AwaitTurn(ctx context.Context, chatID string) error
  func (i *Inflight) SetWorking(chatID string, working bool)
  func (i *Inflight) HoldForAnswer(deliveryID string, …) 
  func (i *Inflight) Resolve(choiceID string, dec Decision) error
  func (i *Inflight) SeenDelivery(deliveryID string) bool
  ```

- [ ] **Step 1: Write the six invariant tests FIRST**

One file each, and each must be shown to FAIL when the invariant is inverted:

```go
// §7.1 — the spawn gate is never taken on the hook-ingest path.
func TestInvariant_HookIngestNeverTakesTheSpawnGate(t *testing.T) {
	// Hold the spawn gate for chat-1. Ingest a hook for chat-1 with a short ctx.
	// It must complete. If it blocks, SwitchProvider's untimed park deadlocks the CLI.
}

// §7.2 — work-state publishes before turn-complete.
func TestInvariant_WorkStatePublishesBeforeTurnCompletes(t *testing.T) {
	// A reader waking on AwaitTurn must observe SetWorking(false) already applied.
	// Inverted, a switch reads "no turn, not yet known" as idle and kills a busy CLI.
}

// §7.3 — the runner points at the chat; the chat never points back.
func TestInvariant_LivenessIsRowExistenceOnly(t *testing.T) {}

// §7.4 — TerminateGraceful is SIGTERM, never SIGKILL.
func TestInvariant_TerminateIsAlwaysGraceful(t *testing.T) {}

// §7.5 — hook delivery is exactly-once by delivery id.
func TestInvariant_RetriedDeliveryAppliesOnce(t *testing.T) {
	// Same delivery id twice → one semantic hook. Inverted, users see duplicate turns.
}

// §7.6 — the hook ingress is a declared method, never a runtime type assertion.
func TestInvariant_HookIngressIsCompileTimeWired(t *testing.T) {
	// var _ TurnUsecase = (*turnUsecase)(nil) is not enough: assert the concrete
	// wiring the container produces actually carries IngestHookDelivery.
}
```

- [ ] **Step 2: For each, run it against the CURRENT code and confirm it passes**, then
  invert the invariant, confirm it FAILS, and restore. A guard that has never failed has
  not been tested. Record each observed failure message in the commit body.

- [ ] **Step 3: Move the machinery, one file per commit**, re-running all six after each.

- [ ] **Step 4: Fold `agentjournal` into `inflight`** — delivery dedup is a conversation
  concern, not a store. Keep the fsync-temp-rename writer; it is what makes §7.5 hold
  across a daemon restart.

## Task 19: The Fact stream goes live

**Files:**
- Modify: `internal/engine/agents/internal/runner/runner.go`
- Create: `internal/app/usecases/agent/internal/translate/translate.go`

- [ ] **Step 1: Write the failing test** — a Fact emitted by the engine must produce
  exactly one asynx command, and an unmapped Fact must produce none.

- [ ] **Step 2: Implement, run the full gate including integration, commit.**

## Stage 5 done criteria

- [ ] All six §7 invariants have a test, and every one was **observed to fail** inverted.
- [ ] `usecases/agent` contains no type holding a mutex.
- [ ] `adapter/store/agentjournal` does not exist.

---

# STAGE 6 — `agentchat` + `agentactivity` → `repositories/chat`

## Task 20: Merge the two aggregates

**Files:**
- Move: `internal/app/repositories/agentchat` → `internal/app/repositories/chat`
- Fold in: `internal/app/repositories/agentactivity`
- Move: `agentactivity/internal/store/internal/content` → `chat/internal/content`
- Modify: `internal/app/container.go` — one `asynx.Asynx[domain.Chat]` replaces two

- [ ] **Step 1: Write the failing test** — tool payload blobs must NOT ride the event
  stream:

```go
// Events are never pruned. A 4MB tool result on the event stream is 4MB forever, on
// every replay. Blobs are content-addressed beside the log and referenced by id.
func TestChat_ToolPayloadIsNotStoredInTheEventStream(t *testing.T) {
	// record a 4MB tool result, then read the raw event rows for that aggregate and
	// assert none exceeds a few KB
}
```

- [ ] **Step 2: Merge, keeping `domain.AgentChat` → `domain.Chat` and
  `domain.AgentActivity` folded in as a field. Run the gate. Commit.**

## Stage 6 done criteria

- [ ] One asynx instance for chat; `agentactivity` does not exist.
- [ ] The blob test passes and was seen to fail when payloads were put inline.

---

# STAGE 7 — Collapse the usecases

## Task 21: `usecases/chat`

**Files:**
- Move: `internal/app/usecases/agent` → `internal/app/usecases/chat`
- Fold in: `agentchatfolder` → `chat/internal/tree`, `agenttools` → `chat/internal/tools`
- Modify: `internal/api/v0/endpoints/agent/handlers/handlers.go` — five ports → one

- [ ] **Step 1: Write the failing test** — the usecase holds no machinery:

```go
// spec §2 rule 3. If a type in the usecase has a mutex, the machinery did not move.
func TestUsecase_HoldsNoMutex(t *testing.T) {
	// parse the package's AST; fail on any struct field of type sync.Mutex/RWMutex
}

// spec §1.3 — one port, not five.
func TestHandlers_TakeOneUsecasePort(t *testing.T) {
	// assert handlers.New has exactly two parameters: the chat usecase and broadcast
}
```

- [ ] **Step 2: Collapse the five interfaces into one `chat.Usecase`**, keeping
  `IngestHookDelivery` a **declared** method (§7.6) — never rediscovered by assertion.

- [ ] **Step 3: Add the compaction surface (§4.5)** — `compacting` as a chat work-state
  published on the WS channel, a ledger marker written by `compact_post` that
  `AssembleHandoff` respects, and `POST /chats/:id/compact` routed to `compact_start`.

```go
func TestCompact_SetsAWorkStateAndClearsIt(t *testing.T) {}
func TestCompact_LeavesALedgerMarkerCarryingTheTrigger(t *testing.T) {}
func TestAssembleHandoff_DoesNotReplayPreBoundaryTurns(t *testing.T) {}
func TestCompact_OnAProviderThatDeclaresNoCompactStart_Is404(t *testing.T) {}
```

- [ ] **Step 4: Full gate, commit.**

## Stage 7 done criteria

- [ ] `usecases/agent`, `agentchatfolder`, `agenttools` do not exist.
- [ ] `handlers.New` takes one usecase port.
- [ ] Compaction is triggerable, visible as a state, and marked on the ledger.

---

# STAGE 8 — Rename the endpoint group

## Task 22: `endpoints/agent` → `endpoints/chat`

**Files:**
- Move: `internal/api/v0/endpoints/agent` → `internal/api/v0/endpoints/chat`
- Modify: `internal/api/v0/router.go`, `endpoints/home/routes.go`
- Modify: the web client — every agent URL

- [ ] **Step 1: Rename the package and the routes**

`/workspaces/:wsId/agent/chats/...` → `/workspaces/:wsId/chats/...`
`/settings/agent/providers` → `/settings/chat/providers`

- [ ] **Step 2: Update the route-declaration audit** — the spec audit declares 22 agent
  routes; every renamed path must be re-declared or the audit fails.

- [ ] **Step 3: Update the frontend in the same commit**

```bash
grep -rn "/agent/" web/src --include='*.ts' --include='*.tsx' | grep -v test
```

Pre-production: rename outright, no aliases.

- [ ] **Step 4: Full gate + `make -C web test` + a live check in the Tauri app.**

## Stage 8 done criteria

- [ ] No route contains `/agent/`.
- [ ] The web client builds and a real chat works end to end in `make dev-desktop`.

---

# FINAL — production readiness

- [ ] Every §9 success criterion in the spec holds.
- [ ] `go vet`, `-race`, integration, `make test-coverage` (≥92%), `golangci-lint`: green.
- [ ] Both architecture guards and all six invariant tests pass, each having been
      observed to fail when inverted.
- [ ] A live verification in `make dev-desktop`: spawn a Claude chat and a Codex chat,
      run a tool, answer a permission, switch provider, compact, resume after a daemon
      restart. Pixels at rest prove nothing — exercise the state.
- [ ] Nothing pushed; no PR opened without being asked.
