# Agent Runner Model — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the running CLI a first-class `AgentRunner` aggregate so that moving it between chats is one atomic write, and delete `AgentSegment` entirely.

**Architecture:** The runner (a CLI in a PTY, keyed by the `crowbarSegmentID` we already mint) becomes a fourth asynx aggregate with three state transitions: `Started` / `SessionBound` / `Moved` / `Exited`. It carries **no status field** — the PTY is the sole authority on liveness. The chat aggregate loses `Segments` and `ActiveSegmentID` and gains nothing; the chat's conversation history and liveness become **projections of runner events**. The frontend binds a pane to `{runnerId, chatId}`, both mutable, so a pane follows its process.

**Tech Stack:** Go 1.23, `char2cs/asynx` (v0.7.0, write-path OCC), gorm read models, gin, React 19 + Zustand/immer, xterm.js, bun.

**Spec:** `docs/superpowers/specs/2026-07-12-agent-runner-model-design.md` — read it first. It is the source of truth; this plan implements it.

## Global Constraints

- **No timing in tests.** Never `time.Sleep`, `Eventually`, `Never`, poll intervals, or timeouts-as-synchronisation. Block on real signals: asynx `WaitIdle` / `Drain` / `SendWait`, channels. `go test -timeout` is the only backstop. (`feedback_no_timing_in_tests`)
- **No legacy migration.** Pre-production, zero users. Never write migration or cleanup code for stale persisted state. Delete the dev state dir / IndexedDB instead. (`feedback_no_legacy_migration`)
- **Every backend bug gets a `TestRegression_*`** in the integration suite — it is the v0 contract. (`feedback_blackbox_regression_tests`)
- **The reducer must never read the hook's `source` field.** Claude reports `source: clear`; Codex reports `source: startup` for the same event. Branch only on facts: *did the id change*, *is the new id known*. (spec §3)
- **No provider-specific code in Go.** All provider knowledge stays declarative in `api/internal/engine/agent/descriptors/*.yaml`.
- **Test files** live in `web/src/__tests__/` mirroring `web/src/`, using `@/` imports. Component files are kebab-case. (CLAUDE.md)
- **Store selectors must be narrow**: `useXxxStore((s) => s.field)`, never bare `useXxxStore()`. Stores must not import from `components/`. (CLAUDE.md)
- **Dev verification only.** `make dev-desktop`. Never touch the production `~/.crowbar` instance. (`feedback_dev_verification_isolation`)

---

## File Structure

**New — the runner aggregate**
- `api/internal/domain/agent_runner.go` — `domain.AgentRunner`.
- `api/internal/app/repositories/agentrunner/agentrunner.go` — package doc, `ErrNotFound`.
- `api/internal/app/repositories/agentrunner/event_store.go` — `EventStore` iface + `NewEventSourced`.
- `api/internal/app/repositories/agentrunner/internal/commands/{start,bind_session,move,exit}.go`
- `api/internal/app/repositories/agentrunner/internal/store/{store,storage,hub}.go` — the two projections + hub fan-out.

**Modified — backend**
- `api/internal/domain/agent_chat.go` — drop `Segments`, `ActiveSegmentID`.
- `api/internal/domain/agent_segment.go` — **deleted**.
- `api/internal/app/repositories/agentchat/internal/commands/{open_segment,end_segment,bind_session}.go` — **deleted**.
- `api/internal/app/repositories/agentchat/event_store.go` — drop those three methods and `CreateInput`'s segment fields.
- `api/internal/app/repositories/agentchat/internal/store/{store,storage}.go` — drop segment columns.
- `api/internal/engine/agent/registry.go` — becomes a **pure reducer** (`reducer.go`), keeping only `segToInjected`.
- `api/internal/app/usecases/agent/agent.go` — the bulk of the rewrite.
- `api/internal/adapter/container.go` — `AgentRunnerES()`, `AgentRunnerReadDB()`.
- `api/internal/app/container.go`, `api/internal/app/repositories/container.go` — wire `axAgentRunner`.
- `api/internal/api/v0/dto/agent.go` — chat DTO gains derived liveness; segment DTO gone.
- `api/cmd/crowbar/chat.go` — `rename --segment`.
- `api/internal/core/config/default.yaml` — `title_instruction` uses `{segid}`.
- `api/internal/engine/agent/template.go` — drop `{chatid}`.

**Modified — frontend**
- `web/src/features/panes/types/pane-content.ts` — `AgentChatContent` gains `runnerId`.
- `web/src/features/agent/api/agent-api.ts` — types follow the DTO.
- `web/src/features/agent/components/agent-chat-pane.tsx` — rewritten; auto-revive machinery deleted.
- `web/src/features/workspace/stores/hooks/use-workspace-agent-chats-stream.ts` — handles runner frames.
- `web/src/features/workspace/stores/slices/buffer-slice.ts` — `repointAgentChatBuffer`.

---

## Task 1: The `AgentRunner` domain type and its commands

**Files:**
- Create: `api/internal/domain/agent_runner.go`
- Create: `api/internal/app/repositories/agentrunner/internal/commands/start.go`
- Create: `api/internal/app/repositories/agentrunner/internal/commands/bind_session.go`
- Create: `api/internal/app/repositories/agentrunner/internal/commands/move.go`
- Create: `api/internal/app/repositories/agentrunner/internal/commands/exit.go`
- Test: `api/internal/app/repositories/agentrunner/internal/commands/commands_test.go`

**Interfaces:**
- Produces: `domain.AgentRunner`; commands `Start`, `BindSession`, `Move`, `Exit` each implementing asynx's command interface (`AggregateID() string`, `EventName() string`, `ShouldSnapshot() bool`, `Validate(*domain.AgentRunner) error`, `EmitEvent(*domain.AgentRunner) domain.AgentRunner`) — mirror `api/internal/app/repositories/agentchat/internal/commands/open_segment.go` exactly for shape.

- [ ] **Step 1: Write the failing test**

```go
// api/internal/app/repositories/agentrunner/internal/commands/commands_test.go
package commands_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestStart_EmitsRunnerOnItsChat(t *testing.T) {
	c := commands.Start{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	}
	require.NoError(t, c.Validate(nil))
	got := c.EmitEvent(nil)
	require.Equal(t, "r1", got.ID)
	require.Equal(t, "c1", got.CurrentChatID)
	require.Equal(t, "pty1", got.TerminalSession)
	require.Empty(t, got.CurrentSession, "no conversation is bound until the provider announces one")
}

func TestStart_RejectsMissingChat(t *testing.T) {
	c := commands.Start{RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude", TerminalSession: "pty1"}
	require.Error(t, c.Validate(nil), "a runner always points at exactly one chat (spec I1)")
}

// Move is THE command the whole refactor exists for: one write, one aggregate.
// It can never fail on the state of the destination chat, because it does not
// touch the destination chat. That is what makes the torn write unrepresentable.
func TestMove_RepointsRunnerAtomically(t *testing.T) {
	cur := &domain.AgentRunner{
		ID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", CurrentChatID: "c1", CurrentSession: "s1",
	}
	c := commands.Move{RunnerID: "r1", ToChatID: "c2", SessionID: "s2", Now: time.Unix(2, 0)}
	require.NoError(t, c.Validate(cur))
	got := c.EmitEvent(cur)
	require.Equal(t, "c2", got.CurrentChatID)
	require.Equal(t, "s2", got.CurrentSession)
	require.Equal(t, "pty1", got.TerminalSession, "the PTY travels with the runner — the terminal never changes")
	require.Equal(t, "claude", got.ProviderID)
}

func TestMove_RejectsUnknownRunner(t *testing.T) {
	c := commands.Move{RunnerID: "r1", ToChatID: "c2", SessionID: "s2"}
	require.Error(t, c.Validate(nil))
}

func TestBindSession_SetsConversationWithoutMovingChat(t *testing.T) {
	cur := &domain.AgentRunner{ID: "r1", CurrentChatID: "c1"}
	c := commands.BindSession{RunnerID: "r1", SessionID: "s1", Now: time.Unix(1, 0)}
	require.NoError(t, c.Validate(cur))
	got := c.EmitEvent(cur)
	require.Equal(t, "s1", got.CurrentSession)
	require.Equal(t, "c1", got.CurrentChatID)
}

// The runner has NO status field to consult (spec §2). Exit is a tombstone for
// audit; liveness is answered by the chat_liveness projection, which drops the
// row, and ultimately by the PTY.
func TestExit_TombstonesForAudit(t *testing.T) {
	cur := &domain.AgentRunner{ID: "r1", CurrentChatID: "c1"}
	c := commands.Exit{RunnerID: "r1", Now: time.Unix(9, 0)}
	require.NoError(t, c.Validate(cur))
	got := c.EmitEvent(cur)
	require.NotNil(t, got.ExitedAt)
	require.Equal(t, time.Unix(9, 0), *got.ExitedAt)
}

func TestEventNames_AreAgentRunnerScoped(t *testing.T) {
	require.Equal(t, "agentrunner.started.r1", commands.Start{RunnerID: "r1"}.EventName())
	require.Equal(t, "agentrunner.session_bound.r1", commands.BindSession{RunnerID: "r1"}.EventName())
	require.Equal(t, "agentrunner.moved.r1", commands.Move{RunnerID: "r1"}.EventName())
	require.Equal(t, "agentrunner.exited.r1", commands.Exit{RunnerID: "r1"}.EventName())
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `cd api && go test ./internal/app/repositories/agentrunner/... -run Test -v`
Expected: FAIL — package `commands` does not exist.

- [ ] **Step 3: Write the domain type**

```go
// api/internal/domain/agent_runner.go
package domain

import "time"

// AgentRunner is ONE live vendor-CLI process in ONE PTY — the thing that
// actually moves between chats when the user types /clear or /resume inside the
// CLI. Its ID is the crowbarSegmentID Crowbar mints at spawn and passes to every
// hook, so a hook can always name its runner.
//
// It deliberately has NO status field. The PTY is the SOLE authority on whether
// this process is alive (spec §2): two authorities on liveness always drift, and
// that drift is exactly what let a segment read "ended" while its CLI was still
// running. ExitedAt is an audit tombstone, never a liveness check — ask the
// chat_liveness projection (which drops the row on exit), or the terminal engine.
//
// What IS durable here is PLACEMENT — which chat, which conversation. Crowbar is
// its only writer, so it cannot drift. Persisting it is what makes a conversation
// switch a single atomic write instead of a torn cross-aggregate one.
type AgentRunner struct {
	ID              string `json:"id"` // == crowbarSegmentID
	WorkspaceID     string `json:"workspaceId"`
	ProviderID      string `json:"providerId"`
	TerminalSession string `json:"terminalSessionId"` // its PTY: identity AND heartbeat

	// CurrentChatID is always set (invariant I1). CurrentSession is empty only
	// between spawn and the provider's first session announcement.
	CurrentChatID  string `json:"currentChatId"`
	CurrentSession string `json:"currentSessionId,omitempty"`

	StartedAt time.Time  `json:"startedAt"`
	ExitedAt  *time.Time `json:"exitedAt,omitempty"` // audit only — NOT a liveness flag
}
```

- [ ] **Step 4: Write the commands**

```go
// api/internal/app/repositories/agentrunner/internal/commands/start.go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Start records a freshly-spawned CLI. No conversation is bound yet — the
// provider announces one via its session hook, which lands as BindSession.
type Start struct {
	RunnerID        string
	WorkspaceID     string
	ProviderID      string
	TerminalSession string
	ChatID          string
	Now             time.Time
}

func (c Start) AggregateID() string  { return c.RunnerID }
func (c Start) EventName() string    { return "agentrunner.started." + c.RunnerID }
func (c Start) ShouldSnapshot() bool { return false }

func (c Start) Validate(current *domain.AgentRunner) error {
	if current != nil {
		return fmt.Errorf("start runner: already started: %w", asynxModels.ErrValidation)
	}
	if c.RunnerID == "" {
		return fmt.Errorf("start runner: missing runner id: %w", asynxModels.ErrValidation)
	}
	// Invariant I1: a runner points at exactly one chat, always.
	if c.ChatID == "" {
		return fmt.Errorf("start runner: missing chat id: %w", asynxModels.ErrValidation)
	}
	if c.TerminalSession == "" {
		return fmt.Errorf("start runner: missing terminal session: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Start) EmitEvent(_ *domain.AgentRunner) domain.AgentRunner {
	return domain.AgentRunner{
		ID:              c.RunnerID,
		WorkspaceID:     c.WorkspaceID,
		ProviderID:      c.ProviderID,
		TerminalSession: c.TerminalSession,
		CurrentChatID:   c.ChatID,
		StartedAt:       c.Now,
	}
}
```

```go
// api/internal/app/repositories/agentrunner/internal/commands/bind_session.go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// BindSession records the provider's conversation id for a runner that is
// staying put. It is the reducer's "bound" outcome: the runner announced its
// FIRST conversation. A runner that announces a DIFFERENT conversation is
// Move-ing, not binding.
type BindSession struct {
	RunnerID  string
	SessionID string
	Now       time.Time
}

func (c BindSession) AggregateID() string  { return c.RunnerID }
func (c BindSession) EventName() string    { return "agentrunner.session_bound." + c.RunnerID }
func (c BindSession) ShouldSnapshot() bool { return false }

func (c BindSession) Validate(current *domain.AgentRunner) error {
	if current == nil {
		return fmt.Errorf("bind session: no runner: %w", asynxModels.ErrValidation)
	}
	if c.SessionID == "" {
		return fmt.Errorf("bind session: missing session id: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c BindSession) EmitEvent(current *domain.AgentRunner) domain.AgentRunner {
	next := *current
	next.CurrentSession = c.SessionID
	return next
}
```

```go
// api/internal/app/repositories/agentrunner/internal/commands/move.go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Move repoints a runner at a different chat and conversation. This ONE command
// is the entire reason the refactor exists.
//
// Note what it does NOT do: it does not touch the chat being left, and it does
// not touch the chat being entered. It cannot fail on their state, because it
// never reads their state. That is what makes the torn cross-aggregate write
// (which bricked a chat in production) unrepresentable rather than merely
// avoided — there is no second write to fail.
//
// The PTY, the provider and the runner id all travel unchanged: the process did
// not restart, it just changed which conversation it is showing. This is why the
// terminal never remounts on a /clear.
type Move struct {
	RunnerID  string
	ToChatID  string
	SessionID string
	Now       time.Time
}

func (c Move) AggregateID() string  { return c.RunnerID }
func (c Move) EventName() string    { return "agentrunner.moved." + c.RunnerID }
func (c Move) ShouldSnapshot() bool { return false }

func (c Move) Validate(current *domain.AgentRunner) error {
	if current == nil {
		return fmt.Errorf("move runner: no runner: %w", asynxModels.ErrValidation)
	}
	if c.ToChatID == "" {
		return fmt.Errorf("move runner: missing chat id: %w", asynxModels.ErrValidation)
	}
	if c.SessionID == "" {
		return fmt.Errorf("move runner: missing session id: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Move) EmitEvent(current *domain.AgentRunner) domain.AgentRunner {
	next := *current
	next.CurrentChatID = c.ToChatID
	next.CurrentSession = c.SessionID
	return next
}
```

```go
// api/internal/app/repositories/agentrunner/internal/commands/exit.go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Exit tombstones a runner whose PTY has died. It is emitted ONLY by the
// terminal engine's exit callback or by boot reconciliation — i.e. it is always
// CAUSED by the PTY, never by an independent opinion about liveness. That is how
// the PTY stays the sole authority (spec §2).
type Exit struct {
	RunnerID string
	Now      time.Time
}

func (c Exit) AggregateID() string  { return c.RunnerID }
func (c Exit) EventName() string    { return "agentrunner.exited." + c.RunnerID }
func (c Exit) ShouldSnapshot() bool { return false }

func (c Exit) Validate(current *domain.AgentRunner) error {
	if current == nil {
		return fmt.Errorf("exit runner: no runner: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Exit) EmitEvent(current *domain.AgentRunner) domain.AgentRunner {
	next := *current
	at := c.Now
	next.ExitedAt = &at
	return next
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd api && go test ./internal/app/repositories/agentrunner/... -v`
Expected: PASS (7 tests).

- [ ] **Step 6: Commit**

```bash
git add api/internal/domain/agent_runner.go api/internal/app/repositories/agentrunner/
git commit -m "feat(agent): the AgentRunner aggregate — a CLI is a thing that moves

Move is one command on one aggregate: it touches neither the chat being left nor
the chat being entered, so the torn cross-aggregate write becomes unrepresentable.
No status field — the PTY is the sole authority on liveness."
```

---

## Task 2: Runner projections — `chat_liveness` and `chat_conversations`

**Files:**
- Create: `api/internal/app/repositories/agentrunner/internal/store/storage.go`
- Create: `api/internal/app/repositories/agentrunner/internal/store/store.go`
- Create: `api/internal/app/repositories/agentrunner/internal/store/hub.go`
- Test: `api/internal/app/repositories/agentrunner/internal/store/store_test.go`

**Interfaces:**
- Consumes: Task 1's `domain.AgentRunner` and the four command event names.
- Produces (all on the **`internal/store`** package — Task 3 wraps these, it does not reimplement them):
  - `store.New(db *gormdb.DB, es asynxModels.Store, ax asynx.Asynx[domain.AgentRunner], broadcast BroadcastFunc) (*Store, error)`
  - `(*Store) AllLive(ctx) ([]domain.AgentRunner, error)` — every live runner; Task 6's boot reconciliation needs it. The live read model holds only live runners by construction, so this is a plain select.
  - `(*Store) LiveRunnerForChat(ctx, chatID string) (domain.AgentRunner, error)` — `ErrNotFound` when dormant
  - `(*Store) LiveRunnerForSession(ctx, wsID, sessionID string) (domain.AgentRunner, error)`
  - `(*Store) ChatForSession(ctx, wsID, sessionID string) (string, error)` — `ErrNotFound` when unknown
  - `(*Store) LastConversation(ctx, chatID string) (domain.ChatConversation, error)`
  - `(*Store) Get(ctx, runnerID string) (domain.AgentRunner, error)`
  - `(*Store) ForgetChat(ctx, chatID string) error`
  - `type BroadcastFunc func(runnerID, workspaceID, chatID, kind string)`
- Add to `api/internal/domain/agent_runner.go`:

```go
// ChatConversation is one conversation a chat has hosted. Append-only history,
// projected from runner events — NOT chat state. It is what AgentSegment really
// was, minus everything that described a process (no status, no PTY, no runner
// id). History cannot drift from reality; only live state can. That is why this
// is safe to persist while the runner's liveness is not.
type ChatConversation struct {
	ChatID      string    `json:"chatId"`
	ProviderID  string    `json:"providerId"`
	SessionID   string    `json:"sessionId"`
	FirstSeenAt time.Time `json:"firstSeenAt"`
}
```

- [ ] **Step 1: Write the failing test**

**Layer boundary — do not cross it.** This task builds ONLY the `internal/store` package. The top-level `agentrunner` package (`EventStore`, `StartInput`, OCC retry, the exported `ErrNotFound` bridge) is **Task 3's** deliverable; do not create it here.

So the harness drives the projection **directly through asynx**, sending `agentrunner/internal/commands` types via `ax.SendWait` — which is exactly what the real sibling harness does. Model it on `api/internal/app/repositories/agentchat/internal/store/store_test.go`, and use the store package's **local** `ErrNotFound` sentinel (the top-level package bridges it outward via `mapNotFound` later, mirroring `agentchat`).

The test bodies below are written against the eventual repo API for readability; **translate the plumbing** (`h.repo.Start(...)` → send `commands.Start{...}` through asynx) while keeping **every scenario and assertion exactly as written**.

**Block on asynx's own signal — never sleep, never poll.** Projections are asynchronous; a sleeping test will be rejected in review.

```go
// api/internal/app/repositories/agentrunner/internal/store/store_test.go
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// A chat is "live" iff some runner points at it. It is NOT a stored flag.
func TestLiveness_FollowsTheRunner(t *testing.T) {
	h := newHarness(t) // see agentchat's store_test.go for the pattern

	_, err := h.repo.Start(h.ctx, agentrunner.StartInput{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	require.NoError(t, err)
	h.drain()

	live, err := h.repo.LiveRunnerForChat(h.ctx, "c1")
	require.NoError(t, err)
	require.Equal(t, "r1", live.ID)

	// c2 is not live yet.
	_, err = h.repo.LiveRunnerForChat(h.ctx, "c2")
	require.ErrorIs(t, err, agentrunner.ErrNotFound)
}

// THE test for the user's bug. The runner moves; the chat it LEFT goes dormant
// and the chat it ENTERED goes live — and neither chat aggregate was written.
func TestMove_TransfersLivenessWithoutWritingEitherChat(t *testing.T) {
	h := newHarness(t)
	_, err := h.repo.Start(h.ctx, agentrunner.StartInput{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	require.NoError(t, err)
	_, err = h.repo.BindSession(h.ctx, "r1", "s1", time.Unix(2, 0))
	require.NoError(t, err)
	_, err = h.repo.Move(h.ctx, "r1", "c2", "s2", time.Unix(3, 0))
	require.NoError(t, err)
	h.drain()

	_, err = h.repo.LiveRunnerForChat(h.ctx, "c1")
	require.ErrorIs(t, err, agentrunner.ErrNotFound, "the chat we left is dormant")

	live, err := h.repo.LiveRunnerForChat(h.ctx, "c2")
	require.NoError(t, err)
	require.Equal(t, "r1", live.ID)
	require.Equal(t, "pty1", live.TerminalSession, "same PTY — the terminal never changed")
}

// The reducer's "is this id known?" question, and Resume's "where do I pick up?",
// both answered from append-only history.
func TestConversations_AreAppendOnlyHistory(t *testing.T) {
	h := newHarness(t)
	_, err := h.repo.Start(h.ctx, agentrunner.StartInput{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	require.NoError(t, err)
	_, err = h.repo.BindSession(h.ctx, "r1", "s1", time.Unix(2, 0))
	require.NoError(t, err)
	_, err = h.repo.Move(h.ctx, "r1", "c2", "s2", time.Unix(3, 0))
	require.NoError(t, err)
	h.drain()

	chatID, err := h.repo.ChatForSession(h.ctx, "w1", "s1")
	require.NoError(t, err)
	require.Equal(t, "c1", chatID, "s1 still belongs to c1 even though the runner left")

	chatID, err = h.repo.ChatForSession(h.ctx, "w1", "s2")
	require.NoError(t, err)
	require.Equal(t, "c2", chatID)

	_, err = h.repo.ChatForSession(h.ctx, "w1", "never-seen")
	require.ErrorIs(t, err, agentrunner.ErrNotFound)

	// Resume reads the tail.
	last, err := h.repo.LastConversation(h.ctx, "c1")
	require.NoError(t, err)
	require.Equal(t, "claude", last.ProviderID)
	require.Equal(t, "s1", last.SessionID)
}

// Exit drops the liveness row but NOT the history: a dormant chat must still be
// resumable, and its conversation must still be recognised on a later /resume.
func TestExit_ClearsLivenessKeepsHistory(t *testing.T) {
	h := newHarness(t)
	_, err := h.repo.Start(h.ctx, agentrunner.StartInput{
		RunnerID: "r1", WorkspaceID: "w1", ProviderID: "claude",
		TerminalSession: "pty1", ChatID: "c1", Now: time.Unix(1, 0),
	})
	require.NoError(t, err)
	_, err = h.repo.BindSession(h.ctx, "r1", "s1", time.Unix(2, 0))
	require.NoError(t, err)
	_, err = h.repo.Exit(h.ctx, "r1", time.Unix(4, 0))
	require.NoError(t, err)
	h.drain()

	_, err = h.repo.LiveRunnerForChat(h.ctx, "c1")
	require.ErrorIs(t, err, agentrunner.ErrNotFound)

	chatID, err := h.repo.ChatForSession(h.ctx, "w1", "s1")
	require.NoError(t, err)
	require.Equal(t, "c1", chatID, "history survives the process")
}

// Invariant I3: at most one LIVE runner per conversation. This is what the
// eviction path queries.
func TestLiveRunnerForSession_FindsTheIncumbent(t *testing.T) {
	h := newHarness(t)
	_, err := h.repo.Start(h.ctx, agentrunner.StartInput{
		RunnerID: "r2", WorkspaceID: "w1", ProviderID: "codex",
		TerminalSession: "pty2", ChatID: "c2", Now: time.Unix(1, 0),
	})
	require.NoError(t, err)
	_, err = h.repo.BindSession(h.ctx, "r2", "s2", time.Unix(2, 0))
	require.NoError(t, err)
	h.drain()

	inc, err := h.repo.LiveRunnerForSession(h.ctx, "w1", "s2")
	require.NoError(t, err)
	require.Equal(t, "r2", inc.ID)
}

var _ = domain.AgentRunner{}
var _ = context.Background
```

- [ ] **Step 2: Run and watch it fail**

Run: `cd api && go test ./internal/app/repositories/agentrunner/internal/store/... -v`
Expected: FAIL — `store` package / `newHarness` do not exist.

- [ ] **Step 3: Write the storage rows**

```go
// api/internal/app/repositories/agentrunner/internal/store/storage.go
package store

import "time"

// runnerRow is the LIVE-runner read model: one row per running CLI. A runner's
// row is DELETED on exit, so "does a row exist for this chat" IS the liveness
// question — there is no status column to go stale (spec §2).
type runnerRow struct {
	ID              string `gorm:"primaryKey"`
	WorkspaceID     string `gorm:"index"`
	ProviderID      string
	TerminalSession string
	CurrentChatID   string `gorm:"index"`
	CurrentSession  string `gorm:"index"`
	StartedAt       time.Time
}

func (runnerRow) TableName() string { return "agent_runners" }

// conversationRow is APPEND-ONLY history: every conversation a chat has ever
// hosted. It replaces AgentSegment, minus everything that described a process.
// Never updated, never deleted (except by the chat delete cascade).
type conversationRow struct {
	ChatID      string `gorm:"primaryKey;index"`
	SessionID   string `gorm:"primaryKey;index"`
	WorkspaceID string `gorm:"index"`
	ProviderID  string
	FirstSeenAt time.Time
}

func (conversationRow) TableName() string { return "agent_chat_conversations" }
```

- [ ] **Step 4: Write the projection**

`store.go` mirrors `agentchat/internal/store/store.go`: `New(db, es, ax, broadcast)` runs `db.AutoMigrate(&runnerRow{}, &conversationRow{})`, subscribes to `asynx.Topic("agentrunner.*")`, and folds each event:

```go
// api/internal/app/repositories/agentrunner/internal/store/store.go — core of onEvent
func (p *projector) onEvent(ctx context.Context, evt asynxModels.Event[domain.AgentRunner]) {
	r := evt.Aggregate

	// EXITED: drop the live row. History (conversationRow) is untouched — a
	// dormant chat must stay resumable and its conversations stay recognisable.
	if r.ExitedAt != nil {
		if err := p.db.WithContext(ctx).Delete(&runnerRow{}, "id = ?", r.ID).Error; err != nil {
			slog.ErrorContext(ctx, "agentrunner projection: delete live row", "runner", r.ID, "err", err)
		}
		return
	}

	// STARTED / SESSION_BOUND / MOVED: upsert the single live row. Placement is
	// whatever the aggregate says — Crowbar is its only writer, so it cannot drift.
	row := runnerRow{
		ID: r.ID, WorkspaceID: r.WorkspaceID, ProviderID: r.ProviderID,
		TerminalSession: r.TerminalSession, CurrentChatID: r.CurrentChatID,
		CurrentSession: r.CurrentSession, StartedAt: r.StartedAt,
	}
	if err := p.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(&row).Error; err != nil {
		slog.ErrorContext(ctx, "agentrunner projection: upsert live row", "runner", r.ID, "err", err)
	}

	// Append the (chat, conversation) pair the runner now holds. Idempotent:
	// DoNothing on conflict keeps it append-only under replay.
	if r.CurrentSession != "" {
		conv := conversationRow{
			ChatID: r.CurrentChatID, SessionID: r.CurrentSession,
			WorkspaceID: r.WorkspaceID, ProviderID: r.ProviderID,
			FirstSeenAt: r.StartedAt,
		}
		if err := p.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
			Create(&conv).Error; err != nil {
			slog.ErrorContext(ctx, "agentrunner projection: append conversation",
				"chat", r.CurrentChatID, "session", r.CurrentSession, "err", err)
		}
	}
}
```

Queries (all on `*Store`), each returning `ErrNotFound` (local sentinel, bridged in `event_store.go`) when there is no row:

```go
func (s *Store) LiveRunnerForChat(ctx context.Context, chatID string) (domain.AgentRunner, error)
func (s *Store) LiveRunnerForSession(ctx context.Context, wsID, sessionID string) (domain.AgentRunner, error)
func (s *Store) ChatForSession(ctx context.Context, wsID, sessionID string) (string, error)   // conversationRow
func (s *Store) LastConversation(ctx context.Context, chatID string) (domain.ChatConversation, error) // ORDER BY first_seen_at DESC LIMIT 1
func (s *Store) Get(ctx context.Context, runnerID string) (domain.AgentRunner, error)
func (s *Store) ForgetChat(ctx context.Context, chatID string) error // delete conversationRows for the chat
```

`hub.go` mirrors `agentchat/internal/store/hub.go` exactly: derive `kind` from the event name (`agentrunner.<kind>.<id>`) and call
`broadcast(evt.AggregateID, evt.Aggregate.WorkspaceID, evt.Aggregate.CurrentChatID, kind)`. Kinds: `started`, `session_bound`, `moved`, `exited`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd api && go test ./internal/app/repositories/agentrunner/... -race -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/app/repositories/agentrunner/ api/internal/domain/agent_runner.go
git commit -m "feat(agent): project chat liveness and conversation history from runner events

Liveness is a row that exists while the runner does — no status column to go
stale. Conversation history is append-only and survives the process, so a dormant
chat stays resumable and its sessions stay recognisable."
```

---

## Task 3: Wire the aggregate (adapter + containers)

**Files:**
- Create: `api/internal/app/repositories/agentrunner/agentrunner.go` (package doc + `ErrNotFound`)
- Create: `api/internal/app/repositories/agentrunner/event_store.go` (`EventStore` iface, `StartInput`, `NewEventSourced`, `mapNotFound`)
- Modify: `api/internal/adapter/container.go` — add `AgentRunnerES()` / `AgentRunnerReadDB()` beside `AgentChatES()` (`:231`), opening `state/events/agent_runner.db` and `state/store/agent_runner.db`
- Modify: `api/internal/app/container.go:48,74,150,189` — add `axAgentRunner asynx.Asynx[domain.AgentRunner]`, build with `newAsynx[domain.AgentRunner](adapters.AgentRunnerES())`, add to `Shutdown`
- Modify: `api/internal/app/repositories/container.go:78,183` — construct via `agentrunner.NewEventSourced(...)`
- Modify: `api/internal/app/hub/hub.go` — add `BroadcastAgentRunner(runnerID, wsID, chatID, kind string)`
- Test: `api/internal/app/repositories/agentrunner/event_store_test.go` (mirror `agentchat/event_store_test.go`)

**Interfaces:**
- Produces:
```go
type StartInput struct {
	RunnerID, WorkspaceID, ProviderID, TerminalSession, ChatID string
	Now time.Time
}
type EventStore interface {
	Start(ctx context.Context, in StartInput) (domain.AgentRunner, error)
	BindSession(ctx context.Context, runnerID, sessionID string, now time.Time) (domain.AgentRunner, error)
	Move(ctx context.Context, runnerID, toChatID, sessionID string, now time.Time) (domain.AgentRunner, error)
	Exit(ctx context.Context, runnerID string, now time.Time) (domain.AgentRunner, error)
	Get(ctx context.Context, runnerID string) (domain.AgentRunner, error)
	LiveRunnerForChat(ctx context.Context, chatID string) (domain.AgentRunner, error)
	LiveRunnerForSession(ctx context.Context, wsID, sessionID string) (domain.AgentRunner, error)
	ChatForSession(ctx context.Context, wsID, sessionID string) (string, error)
	LastConversation(ctx context.Context, chatID string) (domain.ChatConversation, error)
	ForgetChat(ctx context.Context, chatID string) error
}
```

- [ ] **Step 1: Write the failing test** — `event_store_test.go` asserting OCC round-trip: `Start` → `Move` → `Get` returns the moved placement, and a stale-version concurrent `Move` retries rather than clobbering (copy the OCC test from `agentchat/event_store_test.go`).
- [ ] **Step 2: Run it, watch it fail.** `cd api && go test ./internal/app/repositories/agentrunner/...`
- [ ] **Step 3: Implement** `agentrunner.go` + `event_store.go` (mirror `agentchat/event_store.go:142` `NewEventSourced` shape), then the adapter and both containers.
- [ ] **Step 4: Verify the whole backend still builds and passes.**

Run: `cd api && go build ./... && go test ./... -count=1`
Expected: PASS. (`AgentChat` is untouched so far; this task is purely additive.)

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/repositories/agentrunner/ api/internal/adapter/container.go api/internal/app/container.go api/internal/app/repositories/container.go api/internal/app/hub/hub.go
git commit -m "feat(agent): wire axAgentRunner as the fourth per-type asynx aggregate"
```

---

## Task 4: The reducer becomes pure

**Files:**
- Create: `api/internal/engine/agent/reducer.go`
- Test: `api/internal/engine/agent/reducer_test.go`

**This task is ADDITIVE. Do NOT touch `registry.go`.**

The Registry's maps (`segToChat`, `sessionToChat`, `segToSession`) and its methods (`OnSessionStart`, `BindSegment`, `Seed`, `ChatFor`) are still called from **six live sites** in `api/internal/app/usecases/agent/agent.go` (`:212`, `:256`, `:324`, `:518`, `:606`, `:713`, `:1253`). Deleting them here would break the build.

**Task 5 owns their deletion**, in the same commit that rewrites every caller onto the runner aggregate. Task 4 just puts the pure replacement in place beside them. (`segToInjected` — the injected-context echo guard — survives the deletion entirely: it is genuinely ephemeral per-spawn state with no durable counterpart to drift from.)

**Interfaces:**
- Consumes: nothing (pure).
- Produces:
```go
type MoveKind string
const (
	MoveNoop      MoveKind = "noop"       // same conversation — nothing happened
	MoveBind      MoveKind = "bind"       // runner's FIRST conversation
	MoveToNew     MoveKind = "move_new"   // unknown conversation → mint a chat
	MoveToKnown   MoveKind = "move_known" // known conversation → go to its chat
)
type Decision struct {
	Kind   MoveKind
	ChatID string // set only for MoveToKnown
}
// Decide is a PURE function of facts. It never reads the hook's `source` field.
func Decide(currentSession, announcedSession, knownChatID string, known bool) Decision
```

- [ ] **Step 1: Write the failing test**

```go
// api/internal/engine/agent/reducer_test.go
package agent_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
)

func TestDecide(t *testing.T) {
	tests := []struct {
		name                       string
		current, announced, known  string
		isKnown                    bool
		want                       agent.Decision
	}{
		{"same conversation is a no-op", "s1", "s1", "", false,
			agent.Decision{Kind: agent.MoveNoop}},
		{"first announcement binds", "", "s1", "", false,
			agent.Decision{Kind: agent.MoveBind}},
		{"unknown new id mints a chat (/clear, /new)", "s1", "s2", "", false,
			agent.Decision{Kind: agent.MoveToNew}},
		{"known id goes to its chat (/resume)", "s1", "s2", "c2", true,
			agent.Decision{Kind: agent.MoveToKnown, ChatID: "c2"}},
		{"first announcement of a KNOWN id still binds in place", "", "s1", "c1", true,
			agent.Decision{Kind: agent.MoveBind}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, agent.Decide(tc.current, tc.announced, tc.known, tc.isKnown))
		})
	}
}

// LOCKS IN spec §3. Claude reports source=clear, Codex reports source=startup for
// the SAME event (verified against the real binaries). Decide takes no `source`
// argument at all — this test exists so nobody adds one.
func TestDecide_IsSourceAgnosticByConstruction(t *testing.T) {
	// The signature has no `source` parameter. If this file stops compiling
	// because someone added one, that is the bug.
	got := agent.Decide("s1", "s2", "", false)
	require.Equal(t, agent.MoveToNew, got.Kind)
}
```

- [ ] **Step 2: Run it, watch it fail.** `cd api && go test ./internal/engine/agent/ -run TestDecide -v`
- [ ] **Step 3: Implement `Decide`**

```go
// api/internal/engine/agent/reducer.go
package agent

// Decide is the context-move reducer, and it is a PURE function of two facts:
//
//	1. did the conversation id under this runner change?
//	2. is the new id one we already know?
//
// It deliberately takes NO `source` argument. Claude reports source=clear where
// Codex reports source=startup for the very same event (verified against the real
// binaries, spec §7) — so any branch on that vocabulary is provider-specific and
// will break on the next CLI. Branching only on facts is what makes this engine
// provider-agnostic.
//
// It also never decides ANYTHING about whether the move is allowed. By the time a
// hook fires, the CLI has already switched: this reducer reconciles a fait
// accompli, it does not authorise one (spec §3).
func Decide(currentSession, announcedSession, knownChatID string, known bool) Decision {
	switch {
	case announcedSession == currentSession:
		return Decision{Kind: MoveNoop}
	case currentSession == "":
		// The runner is announcing its first conversation. It stays where it is,
		// even if we happen to know the id already (a resumed spawn).
		return Decision{Kind: MoveBind}
	case known:
		return Decision{Kind: MoveToKnown, ChatID: knownChatID}
	default:
		return Decision{Kind: MoveToNew}
	}
}
```

- [ ] **Step 4: Run tests.** Expected PASS.
- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/agent/reducer.go api/internal/engine/agent/reducer_test.go api/internal/engine/agent/registry.go
git commit -m "refactor(agent): the reducer becomes a pure function of facts

It mutates nothing (the in-memory maps that caused the split brain are gone) and
takes no \`source\` argument — claude says 'clear' where codex says 'startup' for
the same event, so branching on that vocabulary would be provider-specific."
```

---

## Task 5: Rewrite the usecase onto runners; delete segments

This is the heavy task. Split the commit, not the task: the tree must build at the end.

**Files:**
- Modify: `api/internal/app/usecases/agent/agent.go` (all of `handleSessionStart`, `moveToNewChat`, `moveToKnownChat`, `bindSession`, `spawnSegment`, `IngestHook`, `SpawnChat`, `SwitchProvider`, `ResumeChat`, `PurgeChat`, `reconcileSegmentExit`, `appendTurn`)
- Modify: `api/internal/domain/agent_chat.go` — delete `Segments`, `ActiveSegmentID`
- Delete: `api/internal/domain/agent_segment.go`
- Delete: `api/internal/app/repositories/agentchat/internal/commands/{open_segment,end_segment,bind_session}.go` and `segment_test.go`
- Modify: `api/internal/app/repositories/agentchat/event_store.go` — drop `OpenSegment`, `EndSegment`, `BindSession`, and `CreateInput`'s `SegmentID`/`CrowbarSegmentID`/`ProviderID`/`TerminalSession`
- Modify: `api/internal/app/repositories/agentchat/internal/store/{store,storage}.go` — drop segment columns
- Modify: `api/internal/engine/agent/registry.go` — **strip it to `segToInjected` only.** Delete the `segToChat` / `sessionToChat` / `segToSession` maps and the `OnSessionStart` / `BindSegment` / `Seed` / `ChatFor` methods, plus `ForgetChat`'s map sweeps. These are the in-memory shadow of durable state that caused the split brain (bug 2): the reducer mutated them BEFORE the aggregate commands ran, so when the commands failed the registry still believed the runner had moved, and the orphaned CLI's turn hooks were routed into a chat it had left. Task 4 has already added the pure `Decide` replacement beside them; this task deletes them in the SAME commit that rewrites their six callers (`agent.go:212,256,324,518,606,713,1253`). `segToInjected` SURVIVES — it is genuinely ephemeral per-spawn state (the injected-context echo guard) with no durable counterpart to drift from.
- Test: `api/internal/app/usecases/agent/agent_test.go`

**Interfaces:**
- Consumes: Task 3's `agentrunner.EventStore`; Task 4's `agent.Decide`.
- `Usecase` gains a `runners agentrunner.EventStore` field; `New(...)` takes it.

**Carried forward from Task 2's review — you must handle this here:**
`agentrunner`'s `ForgetChat` deletes a chat's conversation history but **leaves a live `runnerRow` pointing at the hard-deleted chat**. That is deliberate: it is the *chat delete cascade's* job, and the cascade lives in this task (`PurgeChat`).

The fix follows from the design rule — the PTY is the sole authority on liveness, so you do not "delete the runner row", you **kill the process and let the row follow**:

```go
// PurgeChat hard-deletes a chat. A runner may still be pointed at it, and a
// runner whose chat no longer exists is a runner with nowhere to write — its
// hooks would resolve to a chat that is gone.
//
// So terminate it FIRST and let the PTY's death carry the runner away
// (TerminateGraceful → the engine's onExit → Exit → the projection drops the live
// row). We never reach into the read model to delete a runner row by hand: that
// would be Crowbar asserting a liveness fact it does not own, which is the exact
// dual-authority mistake this refactor deletes.
if live, err := u.runners.LiveRunnerForChat(ctx, chatID); err == nil {
    if err := u.term.TerminateGraceful(ctx, live.TerminalSession); err != nil {
        slog.ErrorContext(ctx, "agent: purge chat: terminate runner", "runner", live.ID, "err", err)
    }
}
// ...then the existing hard-delete + u.runners.ForgetChat(ctx, chatID) for history.
```

Cover it: `TestRegression_DeleteChat_KillsItsRunner` — delete a chat that has a live runner, assert the PTY was terminated and no live runner points at the dead chat.

**Key rewrites, in full:**

```go
// handleSessionStart reconciles a conversation announcement. By the time we are
// called the CLI has ALREADY switched — we record it, and we never fail (spec §3).
func (u *Usecase) handleSessionStart(ctx context.Context, runnerID string, ev engineagent.CanonicalEvent) error {
	runner, err := u.runners.Get(ctx, runnerID)
	if err != nil {
		return nil // a hook from a runner we do not know: ignore, never resurrect
	}

	knownChatID, err := u.runners.ChatForSession(ctx, runner.WorkspaceID, ev.SessionID)
	known := err == nil
	if err != nil && !errors.Is(err, agentrunner.ErrNotFound) {
		return fmt.Errorf("agent: ingest hook: lookup session: %w", err)
	}

	switch d := engineagent.Decide(runner.CurrentSession, ev.SessionID, knownChatID, known); d.Kind {
	case engineagent.MoveNoop:
		return nil

	case engineagent.MoveBind:
		_, err := u.runners.BindSession(ctx, runnerID, ev.SessionID, time.Now())
		return err

	case engineagent.MoveToNew:
		// /clear or /new. Create the chat FIRST: a create can never destroy
		// anything, so the worst failure here is a stray empty chat (spec §4.2).
		newChatID := uuid.NewString()
		if _, err := u.chats.Create(ctx, agentchat.CreateInput{
			ID: newChatID, WorkspaceID: runner.WorkspaceID, Now: time.Now(),
		}); err != nil {
			return fmt.Errorf("agent: ingest hook: mint chat: %w", err)
		}
		_, err := u.runners.Move(ctx, runnerID, newChatID, ev.SessionID, time.Now())
		return err

	case engineagent.MoveToKnown:
		// /resume into a conversation we know. Invariant I3: at most one LIVE
		// runner per conversation — two CLIs on one session id corrupt the
		// provider's own session file. So the incumbent is evicted.
		//
		// Order: MOVE FIRST (record reality — this cannot fail on anyone else's
		// state), THEN evict. If the kill fails, our record is still ACCURATE:
		// two runners really do hold the conversation, and only cleanup needs a
		// retry. Never the reverse — ending something before recording reality is
		// exactly what bricked a chat.
		incumbent, incErr := u.runners.LiveRunnerForSession(ctx, runner.WorkspaceID, ev.SessionID)

		if _, err := u.runners.Move(ctx, runnerID, d.ChatID, ev.SessionID, time.Now()); err != nil {
			return fmt.Errorf("agent: ingest hook: move: %w", err)
		}

		if incErr == nil && incumbent.ID != runnerID {
			u.evict(ctx, incumbent)
		}
		return nil
	}
	return nil
}

// evict terminates a runner that is holding a conversation another runner has
// just taken over (invariant I3). Its PTY dies, and THAT is what makes it dead —
// the PTY is the sole authority, so the Exit event follows from the terminal
// engine's onExit callback rather than being asserted here.
//
// TerminateGraceful, not a hard kill: it is the existing seam (agent.go:46) and
// it SIGTERMs, so the evicted CLI flushes its own transcript on the way out. An
// evicted agent must not lose its last turn — the conversation it was holding is
// about to be read by the runner taking over.
func (u *Usecase) evict(ctx context.Context, incumbent domain.AgentRunner) {
	if err := u.term.TerminateGraceful(ctx, incumbent.TerminalSession); err != nil {
		slog.ErrorContext(ctx, "agent: evict incumbent runner", "runner", incumbent.ID, "err", err)
	}
}
```

```go
// appendTurn / IngestHook routing: resolve the chat from DURABLE runner state.
// This is what stops an orphaned CLI writing turns into a chat it has left — the
// in-memory map that could disagree is gone.
func (u *Usecase) chatForRunner(ctx context.Context, runnerID string) (string, error) {
	r, err := u.runners.Get(ctx, runnerID)
	if err != nil {
		return "", err
	}
	return r.CurrentChatID, nil
}
```

```go
// ResumeChat brings a dormant chat back. "Dormant" = no runner points at it,
// which is a QUERY, not a flag.
func (u *Usecase) ResumeChat(ctx context.Context, chatID string) (string, error) {
	if _, err := u.runners.LiveRunnerForChat(ctx, chatID); err == nil {
		return "", nil // already live — reopening is a no-op
	}
	last, err := u.runners.LastConversation(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: resume: no conversation to resume: %w", err)
	}
	return u.spawnRunner(ctx, chatID, last.ProviderID, last.SessionID)
}
```

- [ ] **Step 1: Write the failing regression test — the user's bug**

```go
// api/internal/app/usecases/agent/agent_test.go

// TestRegression_ResumeIntoOccupiedChat_DoesNotBrickSource
//
// The bug, exactly as the user hit it: runner R1 is on chat A. Inside its CLI the
// user /resume's into chat B's conversation — and B ALREADY has its own live
// runner R2. The old code ran EndSegment(A) (committed), then OpenSegment(B)
// (failed, because B had an active segment), with no rollback: chat A was left
// with no active segment and unusable, and B was never joined.
func TestRegression_ResumeIntoOccupiedChat_DoesNotBrickSource(t *testing.T) {
	h := newUsecaseHarness(t)

	chatA, r1 := h.spawn(t, "claude") // R1 on A, conversation sA
	h.announce(t, r1, "sA")
	chatB, r2 := h.spawn(t, "codex")  // R2 on B, conversation sB
	h.announce(t, r2, "sB")

	// R1's CLI resumes into B's conversation.
	h.announce(t, r1, "sB")

	// Chat A must still be usable: dormant, but with its history intact and
	// resumable. It must NOT be a chat with no way back.
	last, err := h.runners.LastConversation(h.ctx, chatA)
	require.NoError(t, err, "chat A keeps its conversation history and stays resumable")
	require.Equal(t, "sA", last.SessionID)

	// R1 now holds chat B.
	live, err := h.runners.LiveRunnerForChat(h.ctx, chatB)
	require.NoError(t, err)
	require.Equal(t, r1, live.ID, "the mover took the chat over")

	// R2 was evicted — TerminateGraceful'd (invariant I3: one live runner per
	// conversation, or the provider's own session file gets two writers).
	r2runner, err := h.runners.Get(h.ctx, r2)
	require.NoError(t, err)
	require.Contains(t, h.term.Terminated(), r2runner.TerminalSession,
		"the incumbent was terminated")
}
```

- [ ] **Step 2: Run it and watch it fail** — `cd api && go test ./internal/app/usecases/agent/ -run TestRegression_ResumeIntoOccupiedChat -v`. Expected FAIL.
- [ ] **Step 3: Rewrite the usecase** per the code above; delete the segment commands, the segment domain type, and the three `EventStore` methods; update `agentchat` store/storage to drop the segment columns.
- [ ] **Step 4: Run the full backend suite.** `cd api && go build ./... && go test ./... -race -count=1`. Expected PASS.
- [ ] **Step 5: Commit**

```bash
git add -A api/internal
git commit -m "refactor(agent)!: runners move; chats never write each other

Deletes AgentSegment. A conversation switch is now one Move on one aggregate, so
the torn write that bricked a chat cannot happen. Eviction records reality FIRST
and kills the incumbent second — never the reverse."
```

---

## Task 6: Boot reconciliation against the PTY

**Files:**
- Modify: `api/internal/app/usecases/agent/agent.go` — replace the segment-ending boot reactor
- Modify: `api/internal/app/container.go` (the boot NOTE site) — call it
- Test: `api/internal/app/usecases/agent/agent_test.go`

**⚠️ Task 5 DELETED the old boot reconcile along with the segments, and the damage is worse than "stale rows".** Do not under-scope this. `agent_runners` is a durable sqlite table that is never truncated at boot, so after **any daemon restart**:

1. **No chat that ever had a runner can be revived, ever.** `ResumeChat` checks `LiveRunnerForChat` first, finds the **stale row** from before the restart, and returns it as a no-op. The Resume button silently does nothing.
2. The pane attaches to a **dead terminal session**.
3. A chat that was mid-turn at shutdown keeps `Working: true` **forever** — a permanent spinner. (The old `ReconcileOnBoot` cleared it.)

All three are *regressions against pre-refactor behaviour*. This task is what makes the refactor a net improvement rather than a net loss, so it is not optional polish.

**Task 6 is load-bearing a SECOND way (added after Task 5's review):** Task 5 introduced `Displace`, which clears a runner's placement while its process is still alive. If the subsequent `TerminateGraceful` **fails**, that runner keeps its live row forever — placed nowhere, owned by nobody, never `Exit`ed. **Boot reconcile is the only thing that reaps it.** Without this task, those rows accumulate across restarts.

**⚠️ `Displace` BROKE the tmp-dir path derivation — you cannot just restore the old reap.** The crash-orphan sweep derived a spawn dir from `SegmentDir(chatsDir, runner.CurrentChatID, runner.ID, runner.ProviderID)`. Task 5's `Displace` **clears `CurrentChatID`**, so a displaced runner whose kill failed — and which then died with the daemon — can no longer have its tmp dir located from its row at all. Derive the path from something `Displace` does not erase (the runner id and provider are retained), or record the dir at spawn.

**Also restore what went with it:** Task 5's deletion also removed the **crash-orphan tmp reap** — the sweep that deleted per-spawn dirs left behind by a CLI that died without cleanup. They now accumulate forever. (They hold only the rendered hook config — claude's `settings.json`, 0600 in a 0700 dir. **They do NOT hold credentials**: the engine has only three inject verbs — `set_env`, `write_file`, `pass_arg` — there is no `copy_file`, and no descriptor references `auth.json`. Any comment in the tree still claiming a codex `auth.json` copy is **stale**, left over from the removed `CODEX_HOME` design; delete such comments on sight.)

- [ ] **Step 1: Write the failing test**

```go
// A runner cannot outlive its PTY. This is the ONE place liveness is reconciled,
// and it reconciles against the single authority (spec §2).
func TestRegression_DeadPTY_MeansDeadRunner(t *testing.T) {
	h := newUsecaseHarness(t)
	chatID, runnerID := h.spawn(t, "claude")
	h.announce(t, runnerID, "s1")

	h.term.SetLive( /* nothing */ ) // the daemon restarted; every PTY is gone

	require.NoError(t, h.uc.ReconcileRunnersOnBoot(h.ctx))

	_, err := h.runners.LiveRunnerForChat(h.ctx, chatID)
	require.ErrorIs(t, err, agentrunner.ErrNotFound, "no runner may outlive its PTY")

	// ...but the chat is still resumable.
	last, err := h.runners.LastConversation(h.ctx, chatID)
	require.NoError(t, err)
	require.Equal(t, "s1", last.SessionID)
}
```

- [ ] **Step 2: Run, watch it fail.**
- [ ] **Step 3: Implement**

```go
// ReconcileRunnersOnBoot runs ONCE at startup. A PTY does not survive a daemon
// restart, so every runner whose terminal session the engine no longer lists is
// dead. This is the only place liveness is reconciled, and it reconciles against
// the PTY — the single authority. It replaces the old boot reactor that ended
// segments, which maintained a SECOND opinion about liveness and could disagree
// with reality (observed: segment "ended", CLI very much alive).
func (u *Usecase) ReconcileRunnersOnBoot(ctx context.Context) error {
	runners, err := u.runners.AllLive(ctx)
	if err != nil {
		return fmt.Errorf("agent: boot reconcile: list runners: %w", err)
	}
	for _, r := range runners {
		// SessionLive is the seam that asks "is this PROCESS alive", not the
		// engine's SessionExists, which is also true for a PTY-less suspended
		// placeholder. Asking the wrong one is what previously let a
		// restart-orphaned chat keep advertising a live agent (see the seam's
		// doc comment at agent.go:52).
		if u.term.SessionLive(ctx, r.TerminalSession) {
			continue
		}
		if _, err := u.runners.Exit(ctx, r.ID, time.Now()); err != nil {
			slog.ErrorContext(ctx, "agent: boot reconcile: exit dead runner", "runner", r.ID, "err", err)
		}
	}
	return nil
}
```

Add `AllLive(ctx) ([]domain.AgentRunner, error)` to `agentrunner.EventStore` and its store (select all `runnerRow`s — the live read model holds only live runners by construction).

**Do not add new methods to `TerminalCommander`.** It already carries everything this task needs: `TerminateGraceful(ctx, sessionID)` for eviction and `SessionLive(ctx, sessionID) bool` for the liveness authority (`api/internal/app/usecases/agent/agent.go:33-67`).

- [ ] **Step 4: Run tests.** Expected PASS.
- [ ] **Step 5: Commit.** `git commit -m "feat(agent): reconcile runners against the PTY once at boot"`

---

## Task 7: Rename resolves the chat at call time

**Files:**
- Modify: `api/cmd/crowbar/chat.go:20-35` — `rename <title>` with a `--segment` flag replacing the `<chatid>` positional
- Modify: `api/internal/api/v0/endpoints/agent/routes.go` — `POST /agent/runners/:segid/rename`
- Modify: `api/internal/api/v0/endpoints/agent/handlers/chats.go` — resolve `segid → runner → CurrentChatID`, then `SetTitle`
- Modify: `api/internal/core/config/default.yaml:3-5` — `title_instruction` uses `{segid}`
- Modify: `api/internal/engine/agent/template.go` — drop `{chatid}` from `Expand`
- Test: `api/cmd/crowbar/chat_test.go`, `api/internal/api/v0/endpoints/agent/handlers/chats_test.go`

- [ ] **Step 1: Write the failing test**

```go
// TestRegression_RenameResolvesChatAtCallTime
//
// The chat id used to be baked into the CLI's --append-system-prompt at spawn. So
// after ANY conversation move, an agent that titled itself renamed the chat it
// used to be in. Resolving through the runner at call time makes that
// unrepresentable.
func TestRegression_RenameResolvesChatAtCallTime(t *testing.T) {
	h := newUsecaseHarness(t)
	chatA, runnerID := h.spawn(t, "claude")
	h.announce(t, runnerID, "sA")
	h.announce(t, runnerID, "sB") // /clear — the runner is now on a NEW chat

	require.NoError(t, h.uc.RenameByRunner(h.ctx, runnerID, "New Title", "agent"))

	moved, err := h.runners.Get(h.ctx, runnerID)
	require.NoError(t, err)
	require.NotEqual(t, chatA, moved.CurrentChatID)

	got, err := h.chats.Get(h.ctx, moved.CurrentChatID)
	require.NoError(t, err)
	require.Equal(t, "New Title", got.Title, "the title lands on the chat the runner is on NOW")

	old, err := h.chats.Get(h.ctx, chatA)
	require.NoError(t, err)
	require.NotEqual(t, "New Title", old.Title, "and NOT on the chat it used to be on")
}
```

- [ ] **Step 2: Run, watch it fail.**
- [ ] **Step 3: Implement.** `default.yaml`:

```yaml
    title_instruction: |
      Give this conversation a short title, once, by running exactly this command (replace the placeholder with a concise 2-5 word Title-Case title of the task):
      {crowbar} chat rename {scope_flags} --segment {segid} "<title>"
```

- [ ] **Step 4: Run tests + the CLI round-trip test.** Expected PASS.
- [ ] **Step 5: Commit.** `git commit -m "fix(agent): rename resolves the chat from the runner at call time"`

---

## Task 8: DTOs and the REST surface

**Files:**
- Modify: `api/internal/api/v0/dto/agent.go` — `AgentChatDTO` drops `activeSegmentId`; gains `liveRunnerId`, `terminalSessionId`, keeps `activeProviderId` (now derived: the live runner's provider, else `LastConversation`'s). Detail DTO returns `conversations []ChatConversation` instead of `segments`.
- Modify: `api/internal/api/v0/endpoints/agent/handlers/chats.go` — join the projections
- Test: `api/internal/api/v0/endpoints/agent/handlers/chats_test.go`

- [ ] **Step 1: Write the failing test** — `GET /agent/chats` returns `liveRunnerId` + `terminalSessionId` for a live chat and empty strings for a dormant one; `activeProviderId` falls back to the last conversation's provider when dormant (so the provider dropdown still shows the right thing).
- [ ] **Step 2: Run, watch it fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run `cd api && go test ./internal/api/... -count=1`.** Expected PASS.
- [ ] **Step 5: Commit.** `git commit -m "feat(agent): chat DTOs expose the live runner and its PTY"`

---

## Task 9: Frontend — the pane follows its runner

**Files:**
- Modify: `web/src/features/panes/types/pane-content.ts:112-117` — `AgentChatContent` gains `runnerId: string`
- Modify: `web/src/features/workspace/stores/slices/buffer-slice.ts` — add `repointAgentChatBuffer(bufferId, { chatId, runnerId })`
- Modify: `web/src/features/agent/api/agent-api.ts` — types follow Task 8's DTO; drop `AgentSegment`
- Rewrite: `web/src/features/agent/components/agent-chat-pane.tsx`
- Test: `web/src/__tests__/features/agent/components/agent-chat-pane.test.tsx`

**⚠️ PRESERVE THE USER'S STYLING — it is committed (`13dcd293`) and it is NOT yours to change.**

This is a **behaviour** rewrite, not a visual one. The user deliberately stopped the pane from stripping CossUI's `Frame` down to nothing and now uses it as designed. Keep these exactly:

```tsx
<Frame className="h-full w-full">
  <FramePanel className="min-h-0 flex-1 overflow-hidden">
  <FrameFooter className="flex items-center">
```

Do **not** reintroduce `rounded-none border-0 bg-transparent p-0 shadow-none before:hidden` on the panel, and do not hand-roll the footer padding (`px-2 py-1.5`). If your rewrite touches the JSX, carry these classes through verbatim.

**The pane's new contract:**
- Attach to `chat.terminalSessionId` when `chat.liveRunnerId` is set. The PTY liveness double-check in `attachAgentSegment` **collapses** — `liveRunnerId` being present *is* the liveness answer, because its row only exists while the PTY does.
- **Delete `canAutoRevive`, `reviveAttempts`, `MAX_REVIVE_ATTEMPTS`.** That machinery existed only because a pane could not tell "my CLI died" from "my CLI moved". It can now: a move arrives as a `moved` frame naming the new chat. Revive becomes explicit: dormant chat + user opens it = spawn a runner.
- Terminal keyed by `terminalSessionId`; on a move that id is **unchanged**, so xterm never remounts.

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/__tests__/features/agent/components/agent-chat-pane.test.tsx

// The user's requirement: "/clear should change the conversation WITHOUT changing
// the terminal." The runner keeps its PTY, so the xterm instance must survive.
it('follows its runner to a new chat without remounting the terminal', async () => {
  const { store, wsId } = seedWorkspace()
  store.getState().seedAgentChats([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
  render(<AgentChatPane chatId="c1" runnerId="r1" wsId={wsId} bufferId="b1" isActivePane />)

  const term = await screen.findByTestId('xterm')
  expect(term).toHaveAttribute('data-session-id', 'pty1')

  // The runner /clears into a brand-new chat — SAME pty.
  act(() => {
    store.getState().seedAgentChats([
      dormantChat({ id: 'c1' }),
      liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1', title: 'Fresh' }),
    ])
  })

  // The tab relabels and the terminal is the SAME DOM node — not remounted.
  expect(store.getState().buffers.find((b) => b.id === 'b1')).toMatchObject({ chatId: 'c2' })
  expect(await screen.findByTestId('xterm')).toBe(term)
  expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()
})

// Losing your runner must NOT be mistaken for the CLI dying.
it('does not show the exited state when the runner merely moved', async () => { /* as above, assert no Resume button */ })

it('shows Resume only when no runner holds the chat', async () => { /* dormant chat → Resume */ })
```

- [ ] **Step 2: Run, watch it fail.** `cd web && bun run test agent-chat-pane`
- [ ] **Step 3: Implement the pane rewrite.**
- [ ] **Step 4: Run `cd web && bun run test:coverage && bun tsc --noEmit && bunx prettier --check src`.** Expected PASS (CI gates on tsc + prettier).
- [ ] **Step 5: Commit.** `git commit -m "feat(agent): the pane follows its runner; auto-revive machinery deleted"`

---

## Task 10: Frontend — the runner WS stream, eviction UX

**Files:**
- Modify: `web/src/features/workspace/stores/hooks/use-workspace-agent-chats-stream.ts`
- Test: `web/src/__tests__/features/workspace/stores/hooks/use-workspace-agent-chats-stream.test.ts`

**Behaviour:**
- Runner frames arrive on the **existing** workspace-scoped agent-chat WS feed (Task 3 wired `Subscriber.PushAgentRunner` onto it). Kinds: `started` | `session_bound` | `moved` | `displaced` | `exited`, carrying `{runnerId, workspaceId, chatId, kind}`.

- **`displaced` is the one you must not skip, and `chatId` is EMPTY on it — that emptiness *is* its meaning.** Task 5 added `Displace`: it clears a runner's placement while the process is still alive, and it is issued whenever a runner is pushed off a chat by someone else (eviction, provider switch, chat delete).

  **A client following that runner must let go on `displaced` and must NOT wait for `exited`** — because if the kill failed, `exited` **never comes**. Treating `displaced` as "wait and see" is how you get a pane welded to a runner that no longer owns anything.

- **`runnerId` is the discriminator — do not "simplify" it away.** `session_bound` exists in *both* the agentchat and agentrunner event vocabularies. A frame is a runner frame **iff** `runnerId` is present (it is `omitempty`, and chat frames never set it). Branching on `kind` alone is ambiguous and will misroute. This is a structural guarantee, not a temporal one.

- **⚠️ A `moved` frame names only the chat ENTERED, never the chat LEFT.** (Task 3's review caught this.) `chatId` on `moved` is the runner's *destination* — read off the reduced aggregate post-`Move`. So a handler keyed purely on `ev.chatId` will refetch the destination and **never learn the vacated chat went dormant**, leaving its liveness indicator stale until the next reseed.

  You must invalidate **both** chats. You can, because you hold the mapping: look up the buffer/chat the runner was on *before* this frame (the `runnerId → chatId` map this task maintains anyway), and refetch that chat too.

- On `moved`: find the buffer whose `runnerId` matches and `repointAgentChatBuffer` it to the new `chatId`. If a buffer for the destination chat **already exists** (the eviction collision), close the *evicted* one and activate the taker — decision (a): "one tab per live conversation".
- On `exited` where the chat has no other runner: the pane renders dormant + Resume.
- Eviction toast: `toast.info('Conversation moved', '<Provider> was closed — that conversation is now in this terminal.')`. Fire it from a component watching store state, **never from the store** (CLAUDE.md).

- [ ] **Step 1: Write the failing test** — a `moved` frame re-points the buffer; a `moved` frame onto a chat that already has a tab closes the evicted tab and focuses the taker.
- [ ] **Step 2: Run, watch it fail.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run the web suite + tsc + prettier.** Expected PASS.
- [ ] **Step 5: Commit.** `git commit -m "feat(agent): tabs follow runner moves; eviction closes the orphaned tab"`

---

## Task 11: The remaining regression tests

**Files:**
- Modify: `api/tests/` (integration tag) — the black-box v0 contract (`feedback_blackbox_regression_tests`)

- [ ] **Step 1: Write all four remaining regressions from spec §9**, each named for the bug it locks out:
  - `TestRegression_HookAfterFailedMove_DoesNotPolluteLedger` — an orphaned runner's turn hook lands in the chat the runner *actually* holds, never a chat it has left.
  - `TestRegression_ClearMintsChat_KeepsSamePTY` — the moved-to chat carries the same runner and the same `terminalSessionId`.
  - `TestRegression_DeleteChat_LeavesProviderSessionIntact` — Crowbar never deletes a provider's session (the standing rule).
  - `TestRegression_ExitDoesNotRespawn` — a runner that exits stays exited; nothing auto-revives it.
- [ ] **Step 2: Run them against the pre-refactor commit to prove they fail** (`git stash` the impl, or check out the parent). A regression test that passes on the broken code is worthless.
- [ ] **Step 3: Run the full suite.** `cd api && go test ./... -race -count=1 && go test -tags integration ./tests/... -count=1`
- [ ] **Step 4: Commit.**

---

## Task 12: Real-CLI integration — both providers, both flows

**Files:**
- Modify: the existing real-CLI integration suite (the one that already runs 9/9 against claude + codex)

- [ ] **Step 1: Add an in-CLI `/clear` round trip** for each provider: drive the PTY, assert a new chat appears carrying the same PTY, assert the old chat keeps its ledger.
- [ ] **Step 2: Add an in-CLI `/resume` round trip**: resume the first conversation, assert the runner returns to the original chat (`MoveToKnown`) and the agent recalls its earlier content.
- [ ] **Step 3: Assert the codex path specifically tolerates LAZY announcement** (spec §7): after `/new`, no hook fires until the first prompt; the `session_start` for the new conversation must arrive BEFORE that prompt's `user_prompt`, so no turn is misfiled.
- [ ] **Step 4: Run.** Expected: green for both providers.
- [ ] **Step 5: Commit.**

---

## Task 13: Live verification in the running Tauri app — MANDATORY

**No task is done until this passes.** Tests, `tsc` and code review are not a substitute for seeing it work (`feedback_verify_in_tauri_before_claiming`, `feedback_manual_tauri_in_loop`).

**Setup:** `make dev-desktop` (builds the sidecar from local source). **Never** touch the production `~/.crowbar` instance (`feedback_dev_verification_isolation`). Drive via the Tauri MCP; drive the PTY via a TCP→unix relay + the terminal WS (`{"data":"…"}` then `{"data":"\r"}`) — the MCP bridge **cannot** inject xterm keystrokes (`project_tauri_mcp_driving`).

Run each scenario, screenshot the result, and paste the evidence into the task's completion note:

- [ ] **1. `/clear` in Claude** → the tab **relabels** to the new chat, the terminal does **not** flicker or clear, the old chat is still in the sidebar with its history.
- [ ] **2. `/resume` into a dormant chat** → the tab follows, the agent recalls the earlier conversation.
- [ ] **3. `/resume` into a chat that is open and live in ANOTHER tab** (the user's bug) → the incumbent is evicted, its tab closes, focus lands on the taker, the toast appears, and **the source chat is still healthy and resumable**.
- [ ] **4. All three again against Codex** (the lazy-announcement path — the new chat should appear on the first prompt, not the keystroke).
- [ ] **5. `/exit`** → the chat goes dormant and shows Resume; it does **not** respawn itself; Resume brings it back with its conversation.
- [ ] **6. Delete the active chat** → the pane does not blank; an adjacent tab activates.
- [ ] **7. Confirm no orphans:** after all of the above, every `claude`/`codex` process on the machine belongs to a chat the UI shows as live. (The bug left a Claude running in a PTY nothing displayed.)

- [ ] **Step 8: Commit the evidence** — add the screenshots/notes to the PR description, not the repo.

---

## Task 14: Cleanup

- [ ] **Step 1:** `grep -rn "AgentSegment\|activeSegmentId\|crowbarSegmentId" api/ web/src/` — the only surviving hits should be `crowbarSegmentID`-as-runner-id in comments. Anything else is a leftover.
- [ ] **Step 2:** Delete the dev state dir (`<workspace>/.crowbar`) and re-run `make dev-desktop` from clean, to prove a fresh install works with no migration (`feedback_no_legacy_migration`).
- [ ] **Step 3:** Clear dev IndexedDB — persisted buffers from the old shape carry `chatId` with no `runnerId`.
- [ ] **Step 4:** Full suite: `cd api && go test ./... -race && go test -tags integration ./tests/...`; `cd web && bun run test:coverage && bun tsc --noEmit`.
- [ ] **Step 5:** Open the PR.

---

## Self-Review

**Spec coverage:** §2 model → T1, T2, T5. §3 reconcile-never-transact + source-agnostic reducer → T4, T5. §4.1–4.8 flows → T5 (spawn/clear/resume/eviction/turn-routing), T6 (exit/boot), T7 (rename). §5 projections → T2, T3. §6 frontend → T9, T10. §7 provider-agnosticism → T4 (`TestDecide_IsSourceAgnosticByConstruction`), T12. §8 impossibility claims → T11. §9 testing → T11, T12, T13. §10 out-of-scope (TMUX scrub) correctly absent.

**Naming consistency:** `Decide`/`Decision`/`MoveKind` (T4) are used in T5's `handleSessionStart`. `agentrunner.EventStore` methods declared in T3 are exactly those called in T5, T6, T8. `runnerId`/`terminalSessionId` in T8's DTO match T9/T10's frontend usage. `ChatConversation` is declared in T2 and consumed in T5's `ResumeChat` and T8's detail DTO.

**Known gap, deliberate:** T5's `MoveToNew` is two writes (create chat, then move). The plan does not pretend otherwise — the ordering bounds the failure to a stray empty chat, and spec §4.2 states the guarantee as *"a failure can never destroy a chat"*, not *"every operation is a single write."*
