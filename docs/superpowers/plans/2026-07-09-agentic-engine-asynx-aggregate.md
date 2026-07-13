# Agentic Engine — Event-Sourced `AgentChat` Aggregate — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert the agentic-bridge conversation store (`domain.AgentChat` /
`domain.AgentSegment`) from a bespoke gorm store into a `char2cs/asynx`
event-sourced aggregate, structurally identical to `Workspace` / `ReviewThread`,
while preserving the existing switch/resume/handoff behavior and keeping all
subprocess/PTY supervision out of asynx.

**Architecture:** `AgentChat` becomes a **chat-scoped** aggregate with segments
folded in as embedded state; hooks become pure commands; the usecase does IO
(spawn/terminate PTY) then `SendWait`s a pure command (the
`workspace/reconcile.go` pattern). Conversation content stays in the existing
append-only **ledger** (the handoff source); the aggregate owns
identity/segments/session-ids/title/live-`Working` only. Two projections (store
+ hub) serve queries and WS fan-out. A boot reactor reconciles live flags left
stale by a crash. Design: `docs/superpowers/specs/2026-07-09-agentic-engine-asynx-aggregate-design.md`.

**Tech Stack:** Go, `github.com/char2cs/asynx` v0.6.2 (event-sourcing/CQRS),
gorm (read-model projections only), gin, the terminal engine (PTY seam).

## Global Constraints

- **Build:** `go build -tags noEmbed ./...` (daemon tag; `web/dist` not
  required). Never `go build ./...` — it fails on `cmd/crowbar/web_embed.go`.
- **No timing in tests.** NEVER sleep / `Eventually` / poll. Block on asynx
  signals (`SendWait`, `WaitQuiescent`, `WaitPublish`) and channels. `go test
  -timeout` is the only backstop. This is a hard rule.
- **No legacy migration.** Pre-production, no users: drop the gorm `agent_chats`
  / `agent_segments` tables outright; do NOT write migration code. Clear dev
  state (`<repo>/.crowbar`) when the schema changes.
- **Aggregate structure mirrors `ReviewThread`.** Command = pure
  `Validate` + `EmitEvent` (no IO, no randomness). Repo = OCC-retry wrapper over
  a per-type `asynx.Asynx[domain.AgentChat]` singleton. `store.New` registers
  the save-only store projection AND the hub projection. Event name is
  id-suffixed: `"agentchat.<verb>." + c.ID`.
- **asynx never supervises processes.** The vendor-CLI PTY lifecycle stays in
  `usecases/agent` + the terminal engine and *feeds* pure commands in.
- **Dev isolation.** Verify live only on `make dev-desktop` (roots state at
  `<repo>/.crowbar`). NEVER touch prod `~/.crowbar`.
- **Sequencing.** Task 1 (merge develop) is the prerequisite baseline; Tasks
  2–13 are the aggregate conversion and assume it is done.

## File Structure

Mirrors `repositories/reviewthread/`:

- `api/internal/domain/agent_chat.go` — the `AgentChat` aggregate struct
  (extended: embedded segments + live state) + `AgentChatStatus` enum. **Modify.**
- `api/internal/domain/agent_segment.go` — `AgentSegment` becomes an embedded
  value type (drop gorm `TableName`/tags). **Modify.**
- `api/internal/app/repositories/agentchat/agentchat.go` — asynx-backed repo
  (OCC-retry wrapper). **Rewrite** (was the gorm store).
- `api/internal/app/repositories/agentchat/internal/commands/*.go` — one pure
  command per transition. **Create.**
- `api/internal/app/repositories/agentchat/internal/store/{store,storage,hub}.go`
  — store + hub projections. **Create.**
- `api/internal/app/repositories/agentchat/internal/reactors/{boot,delete}.go`
  — boot-reconcile + delete reactors. **Create.**
- `api/internal/app/container.go`, `api/internal/adapter/container.go`,
  `api/internal/app/repositories/container.go` — wiring. **Modify.**
- `api/internal/app/usecases/agent/agent.go` — read→reduce→emit; delete
  `keyed_mutex.go`. **Modify.**
- `api/internal/app/usecases/agent/keyed_mutex.go` — **Delete.**
- `api/internal/app/usecases/container.go` — `worktreepath.For`→`w.WorktreePath`
  fix (Task 1). **Modify.**
- `api/tests/integration/agent/*.go` — real-CLI integration. **Modify/Create.**

---

## Task 1: Baseline — merge `develop`, fix the compile break, green

**Files:**
- Modify: `api/internal/app/usecases/container.go:202`
- Modify: `api/internal/app/usecases/agent_workspace_reader_internal_test.go`
- Resolve conflicts: `api/internal/app/usecases/container.go` (imports),
  `api/internal/api/v0/endpoints/agent/handlers/status.go` (or wherever
  `agentchat.ErrNotFound` folds into `isNotFound()`)

**Interfaces:**
- Produces: a green `feature/agentic-bridge` containing `develop` (the #44
  per-type asynx refactor). `agentWorkspaceReader.WorktreeDir` returns the
  workspace read model's `WorktreePath`.

- [ ] **Step 1: Merge develop.**
```bash
git checkout feature/agentic-bridge
git merge origin/develop     # auto-merges most; leaves import + ErrNotFound conflicts
```

- [ ] **Step 2: Fix the compile break** in `usecases/container.go`. The deleted
  `worktreepath.For(...)` is replaced by the read model's stored path (develop's
  `domain.Workspace` already carries `WorktreePath`):
```go
// WorktreeDir implements agent.WorkspaceReader.
func (r *agentWorkspaceReader) WorktreeDir(
	ctx context.Context,
	workspaceID string,
) (crowbarHome, projectID, repoID, worktree string, err error) {
	home, err := r.crowbarHome()
	if err != nil {
		return "", "", "", "", fmt.Errorf("usecases: agent workspace reader: crowbar home: %w", err)
	}
	w, err := r.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return "", "", "", "", fmt.Errorf("usecases: agent workspace reader: get workspace: %w", err)
	}
	return home, w.ProjectID, w.RepoID, w.WorktreePath, nil
}
```
Remove the now-unused `worktreepath` import if nothing else in the file uses it
(the ledger dir still uses `worktreepath.StorageDir`, which survives — keep the
import iff still referenced).

- [ ] **Step 3: Fix the internal test** `agent_workspace_reader_internal_test.go`
so the expected worktree equals the read model's `WorktreePath` (set a known
`WorktreePath` on the fake workspace and assert equality) instead of
reconstructing via the deleted `worktreepath.For`.

- [ ] **Step 4: Resolve the 2 textual conflicts.** Union the `usecases/container.go`
import block; fold `agentchat.ErrNotFound` into the shared `isNotFound()` error
helper the agent handlers use.

- [ ] **Step 5: Build + test.**
```bash
go build -tags noEmbed ./...
go test ./api/internal/app/usecases/... ./api/internal/api/v0/endpoints/agent/...
```
Expected: PASS (this is a merge/fix; existing tests are the gate).

- [ ] **Step 6: Commit.**
```bash
git add -A
git commit -m "chore(agent): land refactored develop; WorktreeDir reads workspace WorktreePath"
```

---

## Task 2: `AgentChat` aggregate struct + status enum

**Files:**
- Modify: `api/internal/domain/agent_chat.go`
- Modify: `api/internal/domain/agent_segment.go`
- Create: `api/internal/domain/agent_chat_status.go`
- Test: `api/internal/domain/agent_chat_test.go`

**Interfaces:**
- Produces: `domain.AgentChat` (aggregate root with `Segments []AgentSegment`,
  `ActiveSegmentID`, `Working`, `CurrentTurnStarted *time.Time`,
  `LastActivityAt`, `LedgerCursor int`, `Status AgentChatStatus`),
  `domain.AgentSegment` (embedded value type), `domain.AgentChatStatus`
  (`active|archived|deleted`).

- [ ] **Step 1: Write the failing test** `agent_chat_test.go`:
```go
package domain_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestAgentChat_ZeroValueIsInactiveNotWorking(t *testing.T) {
	var c domain.AgentChat
	if c.Working {
		t.Fatal("zero-value AgentChat must not be Working")
	}
	if len(c.Segments) != 0 {
		t.Fatal("zero-value AgentChat must have no segments")
	}
}

func TestAgentChatStatus_Values(t *testing.T) {
	if domain.AgentChatStatusActive != "active" ||
		domain.AgentChatStatusDeleted != "deleted" {
		t.Fatal("unexpected status literals")
	}
}
```

- [ ] **Step 2: Run it — expect FAIL** (`AgentChatStatus` undefined):
`go test ./api/internal/domain/ -run AgentChat`

- [ ] **Step 3: Implement.** `agent_chat_status.go`:
```go
package domain

// AgentChatStatus is the lifecycle state of an AgentChat aggregate.
type AgentChatStatus string

const (
	AgentChatStatusActive   AgentChatStatus = "active"
	AgentChatStatusArchived AgentChatStatus = "archived"
	AgentChatStatusDeleted  AgentChatStatus = "deleted"
)
```
`agent_chat.go` (drop gorm tags; add embedded state):
```go
package domain

import "time"

// AgentChat is the Crowbar-owned agentic conversation aggregate, tracked across
// provider segments. Mutated only through asynx commands. Conversation content
// lives in the ledger, not here — this aggregate holds identity, segments,
// session ids, title, and live Working state, plus a ledger cursor.
type AgentChat struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspaceId"`
	Title       string          `json:"title"`
	TitleLocked bool            `json:"titleLocked"`
	CreatedAt   time.Time       `json:"createdAt"`
	Status      AgentChatStatus `json:"status,omitempty"`

	Segments        []AgentSegment `json:"segments"`
	ActiveSegmentID string         `json:"activeSegmentId,omitempty"`

	// Live turn state — folded from Turn events, reconciled on boot. Not durable
	// truth: a crash between the ledger append and the turn event can leave these
	// stale; the boot reactor repairs them.
	Working            bool       `json:"working"`
	CurrentTurnStarted *time.Time `json:"currentTurnStarted,omitempty"`
	LastActivityAt     time.Time  `json:"lastActivityAt"`

	// LedgerCursor is the count of ledger entries the aggregate has observed —
	// the pointer relating aggregate state to the append-only content log.
	LedgerCursor int `json:"ledgerCursor"`
}
```
`agent_segment.go` (embedded value type — remove `TableName` + gorm tags):
```go
package domain

import "time"

// AgentSegment is one provider stint within an AgentChat, embedded in the
// aggregate (no longer its own table). Invariant (enforced in command Validate):
// at most one segment with Status=="active" per AgentChat.
type AgentSegment struct {
	ID                string     `json:"id"`
	ProviderID        string     `json:"providerId"`
	ProviderSessionID string     `json:"providerSessionId,omitempty"`
	CrowbarSegmentID  string     `json:"crowbarSegmentId"`
	TerminalSessionID string     `json:"terminalSessionId"`
	StartedAt         time.Time  `json:"startedAt"`
	EndedAt           *time.Time `json:"endedAt,omitempty"`
	Status            string     `json:"status"` // active | ended
}
```

- [ ] **Step 4: Run tests — expect PASS:** `go test ./api/internal/domain/ -run AgentChat`

- [ ] **Step 5: Commit.** `git commit -am "feat(agent): AgentChat aggregate struct + status enum"`

---

## Task 3: Segment-lifecycle commands (`Create`, `OpenSegment`, `EndSegment`)

**Files:**
- Create: `api/internal/app/repositories/agentchat/internal/commands/{create,open_segment,end_segment}.go`
- Test: `api/internal/app/repositories/agentchat/internal/commands/segment_test.go`

**Interfaces:**
- Consumes: `domain.AgentChat`, `asynxModels.ErrValidation`.
- Produces: three `asynx.Command[domain.AgentChat]` implementations. `Validate`
  enforces the ≤1-active-segment invariant. `EventName`:
  `agentchat.created.<id>`, `agentchat.segment_opened.<id>`,
  `agentchat.segment_ended.<id>`.

- [ ] **Step 1: Write the failing test** (`segment_test.go`) — the invariant is
the crux:
```go
package commands_test

import (
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestOpenSegment_RejectsSecondActive(t *testing.T) {
	now := time.Unix(0, 0)
	chat := &domain.AgentChat{
		ID:              "c1",
		ActiveSegmentID: "s1",
		Segments:        []domain.AgentSegment{{ID: "s1", Status: "active"}},
	}
	c := commands.OpenSegment{ChatID: "c1", SegmentID: "s2", CrowbarSegmentID: "cs2", ProviderID: "codex", Now: now}
	if err := c.Validate(chat); err == nil {
		t.Fatal("opening a 2nd active segment must fail Validate")
	}
}

func TestEndSegment_ClearsActive(t *testing.T) {
	now := time.Unix(10, 0)
	chat := &domain.AgentChat{
		ID:              "c1",
		ActiveSegmentID: "s1",
		Segments:        []domain.AgentSegment{{ID: "s1", Status: "active"}},
	}
	out := commands.EndSegment{ChatID: "c1", Now: now}.EmitEvent(chat)
	if out.ActiveSegmentID != "" || out.Segments[0].Status != "ended" || out.Segments[0].EndedAt == nil {
		t.Fatalf("EndSegment must end the active segment and clear ActiveSegmentID: %+v", out)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (package undefined):
`go test ./api/internal/app/repositories/agentchat/internal/commands/`

- [ ] **Step 3: Implement the three commands.** `create.go`:
```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Create seeds a new AgentChat with its first (active) segment.
type Create struct {
	ID               string
	WorkspaceID      string
	SegmentID        string
	CrowbarSegmentID string
	ProviderID       string
	TerminalSession  string
	Now              time.Time
}

func (c Create) AggregateID() string   { return c.ID }
func (c Create) EventName() string     { return "agentchat.created." + c.ID }
func (c Create) ShouldSnapshot() bool  { return false }

func (c Create) Validate(current *domain.AgentChat) error {
	if current != nil {
		return fmt.Errorf("create agent chat: exists: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.WorkspaceID == "" || c.SegmentID == "" {
		return fmt.Errorf("create agent chat: missing ids: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Create) EmitEvent(_ *domain.AgentChat) domain.AgentChat {
	return domain.AgentChat{
		ID:              c.ID,
		WorkspaceID:     c.WorkspaceID,
		Status:          domain.AgentChatStatusActive,
		ActiveSegmentID: c.SegmentID,
		Segments: []domain.AgentSegment{{
			ID:                c.SegmentID,
			ProviderID:        c.ProviderID,
			CrowbarSegmentID:  c.CrowbarSegmentID,
			TerminalSessionID: c.TerminalSession,
			StartedAt:         c.Now,
			Status:            "active",
		}},
		CreatedAt:      c.Now,
		LastActivityAt: c.Now,
	}
}
```
`open_segment.go` (Validate rejects a 2nd active; appends + sets active):
```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// OpenSegment appends a new active segment (switch-in / resume).
type OpenSegment struct {
	ChatID           string
	SegmentID        string
	CrowbarSegmentID string
	ProviderID       string
	TerminalSession  string
	Now              time.Time
}

func (c OpenSegment) AggregateID() string  { return c.ChatID }
func (c OpenSegment) EventName() string    { return "agentchat.segment_opened." + c.ChatID }
func (c OpenSegment) ShouldSnapshot() bool { return false }

func (c OpenSegment) Validate(current *domain.AgentChat) error {
	if current == nil {
		return fmt.Errorf("open segment: no chat: %w", asynxModels.ErrValidation)
	}
	for _, s := range current.Segments {
		if s.Status == "active" {
			return fmt.Errorf("open segment: active segment exists: %w", asynxModels.ErrValidation)
		}
	}
	if c.SegmentID == "" {
		return fmt.Errorf("open segment: missing segment id: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c OpenSegment) EmitEvent(current *domain.AgentChat) domain.AgentChat {
	next := *current
	next.Segments = append(append([]domain.AgentSegment{}, current.Segments...), domain.AgentSegment{
		ID:                c.SegmentID,
		ProviderID:        c.ProviderID,
		CrowbarSegmentID:  c.CrowbarSegmentID,
		TerminalSessionID: c.TerminalSession,
		StartedAt:         c.Now,
		Status:            "active",
	})
	next.ActiveSegmentID = c.SegmentID
	next.LastActivityAt = c.Now
	return next
}
```
`end_segment.go`:
```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// EndSegment ends the currently-active segment (switch-out / process exit).
type EndSegment struct {
	ChatID string
	Now    time.Time
}

func (c EndSegment) AggregateID() string  { return c.ChatID }
func (c EndSegment) EventName() string    { return "agentchat.segment_ended." + c.ChatID }
func (c EndSegment) ShouldSnapshot() bool { return false }

func (c EndSegment) Validate(current *domain.AgentChat) error {
	if current == nil {
		return fmt.Errorf("end segment: no chat: %w", asynxModels.ErrValidation)
	}
	return nil // idempotent: ending with no active segment is a no-op fold
}

func (c EndSegment) EmitEvent(current *domain.AgentChat) domain.AgentChat {
	next := *current
	segs := append([]domain.AgentSegment{}, current.Segments...)
	for i := range segs {
		if segs[i].ID == current.ActiveSegmentID && segs[i].Status == "active" {
			ended := c.Now
			segs[i].Status = "ended"
			segs[i].EndedAt = &ended
		}
	}
	next.Segments = segs
	next.ActiveSegmentID = ""
	next.LastActivityAt = c.Now
	return next
}
```

- [ ] **Step 4: Run — expect PASS.**
`go test ./api/internal/app/repositories/agentchat/internal/commands/`

- [ ] **Step 5: Commit.** `git commit -am "feat(agent): segment lifecycle commands + ≤1-active invariant"`

---

## Task 4: Session + turn commands (`BindSession`, `StartTurn`, `StopTurn`)

**Files:**
- Create: `.../internal/commands/{bind_session,start_turn,stop_turn}.go`
- Test: `.../internal/commands/turn_test.go`

**Interfaces:**
- Produces: `BindSession` (sets `ProviderSessionID` on a segment by
  `CrowbarSegmentID`), `StartTurn` (`Working=true`), `StopTurn`
  (`Working=false`). Event names `agentchat.session_bound.<id>`,
  `agentchat.turn_started.<id>`, `agentchat.turn_stopped.<id>`.

- [ ] **Step 1: Failing test** (`turn_test.go`):
```go
package commands_test

import (
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestStartStopTurn_TogglesWorking(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1"}
	started := commands.StartTurn{ChatID: "c1", Now: time.Unix(1, 0)}.EmitEvent(chat)
	if !started.Working || started.CurrentTurnStarted == nil {
		t.Fatal("StartTurn must set Working + CurrentTurnStarted")
	}
	stopped := commands.StopTurn{ChatID: "c1", Now: time.Unix(2, 0)}.EmitEvent(&started)
	if stopped.Working || stopped.CurrentTurnStarted != nil {
		t.Fatal("StopTurn must clear Working + CurrentTurnStarted")
	}
}

func TestBindSession_SetsSessionOnSegment(t *testing.T) {
	chat := &domain.AgentChat{
		ID:       "c1",
		Segments: []domain.AgentSegment{{ID: "s1", CrowbarSegmentID: "cs1", Status: "active"}},
	}
	out := commands.BindSession{ChatID: "c1", CrowbarSegmentID: "cs1", ProviderSessionID: "claude-sess-1"}.EmitEvent(chat)
	if out.Segments[0].ProviderSessionID != "claude-sess-1" {
		t.Fatal("BindSession must set ProviderSessionID on the matching segment")
	}
}
```

- [ ] **Step 2: Run — expect FAIL.**

- [ ] **Step 3: Implement.** `start_turn.go`:
```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type StartTurn struct {
	ChatID string
	Now    time.Time
}

func (c StartTurn) AggregateID() string  { return c.ChatID }
func (c StartTurn) EventName() string    { return "agentchat.turn_started." + c.ChatID }
func (c StartTurn) ShouldSnapshot() bool { return false }

func (c StartTurn) Validate(current *domain.AgentChat) error {
	if current == nil {
		return fmt.Errorf("start turn: no chat: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c StartTurn) EmitEvent(current *domain.AgentChat) domain.AgentChat {
	next := *current
	t := c.Now
	next.Working = true
	next.CurrentTurnStarted = &t
	next.LastActivityAt = c.Now
	return next
}
```
`stop_turn.go` (mirror; `Working=false`, `CurrentTurnStarted=nil`; `EventName`
`agentchat.turn_stopped.<id>`). `bind_session.go`:
```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type BindSession struct {
	ChatID            string
	CrowbarSegmentID  string
	ProviderSessionID string
}

func (c BindSession) AggregateID() string  { return c.ChatID }
func (c BindSession) EventName() string    { return "agentchat.session_bound." + c.ChatID }
func (c BindSession) ShouldSnapshot() bool { return false }

func (c BindSession) Validate(current *domain.AgentChat) error {
	if current == nil {
		return fmt.Errorf("bind session: no chat: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c BindSession) EmitEvent(current *domain.AgentChat) domain.AgentChat {
	next := *current
	segs := append([]domain.AgentSegment{}, current.Segments...)
	for i := range segs {
		if segs[i].CrowbarSegmentID == c.CrowbarSegmentID && segs[i].Status == "active" {
			segs[i].ProviderSessionID = c.ProviderSessionID
		}
	}
	next.Segments = segs
	return next
}
```

- [ ] **Step 4: Run — expect PASS.**

- [ ] **Step 5: Commit.** `git commit -am "feat(agent): session-bind + turn working-state commands"`

---

## Task 5: Title + delete commands (`SetTitle`, `Delete`)

**Files:**
- Create: `.../internal/commands/{set_title,delete}.go`
- Test: `.../internal/commands/title_delete_test.go`

**Interfaces:**
- Produces: `SetTitle{ChatID, Title, Source, Lock}` honoring precedence
  user>agent>derived via `TitleLocked`; `Delete{ChatID}` → tombstone
  `Status=deleted`. Event names `agentchat.title_set.<id>`,
  `agentchat.deleted.<id>`.

- [ ] **Step 1: Failing test** (`title_delete_test.go`):
```go
package commands_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestSetTitle_LockedRejectsDerived(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1", Title: "User Title", TitleLocked: true}
	if err := (commands.SetTitle{ChatID: "c1", Title: "derived", Source: "derived"}).Validate(chat); err == nil {
		t.Fatal("a locked title must reject a derived overwrite")
	}
}

func TestDelete_Tombstones(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1", Status: domain.AgentChatStatusActive}
	out := commands.Delete{ChatID: "c1"}.EmitEvent(chat)
	if out.Status != domain.AgentChatStatusDeleted {
		t.Fatal("Delete must tombstone Status=deleted")
	}
}
```

- [ ] **Step 2: Run — expect FAIL.**

- [ ] **Step 3: Implement.** `set_title.go` (`Source` ∈ `user|agent|derived`;
`user` locks; a locked title rejects lower-precedence sources in `Validate`;
`EmitEvent` sets `Title` and `TitleLocked = Source=="user"`). `delete.go`
(`Validate`: chat must exist; `EmitEvent`: `next.Status =
domain.AgentChatStatusDeleted`).

- [ ] **Step 4: Run — expect PASS.**

- [ ] **Step 5: Commit.** `git commit -am "feat(agent): title (precedence) + delete tombstone commands"`

---

## Task 6: `agentchat` repository over asynx (OCC-retry); delete gorm store

**Files:**
- Rewrite: `api/internal/app/repositories/agentchat/agentchat.go`
- Test: `api/internal/app/repositories/agentchat/agentchat_test.go` (rewrite)

**Interfaces:**
- Consumes: the Task 3–5 commands; `asynx.Asynx[domain.AgentChat]`; the Task 7
  `store.New`.
- Produces: `agentchat.Store` interface with command-issuing methods (`Create`,
  `OpenSegment`, `EndSegment`, `BindSession`, `StartTurn`, `StopTurn`,
  `SetTitle`, `Delete`) + reads (`GetChat`, `ListChats`,
  `GetByProviderSession`, `ListByWorkspace`), each via `sendWithOCC`. **The
  method set the usecase depends on (Task 10) is frozen here.**

- [ ] **Step 1: Failing test** — concurrency without timing (block on asynx):
```go
// Two OpenSegment sends racing on one chat: exactly one wins, the other's
// Validate rejects (active exists). Assert via SendWait/Get, never sleeps.
func TestAgentChat_ConcurrentOpenSegment_OneWins(t *testing.T) { /* build repo over a real in-mem asynx; errgroup 2x OpenSegment; assert 1 err ErrValidation, final chat has 1 active */ }
```

- [ ] **Step 2: Run — expect FAIL.**

- [ ] **Step 3: Implement** mirroring `reviewthread/reviewthread.go`: struct
`agentChat{ ax asynx.Asynx[domain.AgentChat]; store store.Store }`; `New(ax,
es, storeDB, broadcast)` delegates to `store.New(...)` to register projections;
`maxOCCAttempts = 8`; a private `sendWithOCC(ctx, cmd)` that retries on
`asynxModels.ErrPipelineFailed`, never on `ErrValidation` (map →
`apperr.ErrConflict`/`422`), maps `ErrQueueFull` → `apperr.ErrUnavailable`. Each
public method builds its command and calls `sendWithOCC`. Reads delegate to the
store projection. **Delete every gorm method and the `gorm.DB` field.**

- [ ] **Step 4: Run — expect PASS** (`-race`):
`go test -race ./api/internal/app/repositories/agentchat/...`

- [ ] **Step 5: Commit.** `git commit -am "feat(agent): asynx-backed agentchat repo (OCC retry), drop gorm store"`

---

## Task 7: Store projection (read model) + Replay rebuild

**Files:**
- Create: `.../agentchat/internal/store/{store,storage}.go`
- Test: `.../agentchat/internal/store/store_test.go`

**Interfaces:**
- Consumes: `asynx.Asynx[domain.AgentChat]`, `asynxModels.Store` (the event
  log), a read-model `*gorm.DB`.
- Produces: `store.New(db, es, ax, broadcast)` registering the save-only store
  projection + the Task 8 hub projection; queries `GetChat`, `ListChats`,
  `ListByWorkspace`, `GetByProviderSession`; lazy whole-model `Replay` rebuild on
  first read after loss. Serves the **same `dto/agent.go` shape** as today so the
  FE is unchanged.

- [ ] **Step 1: Failing test**: issue `Create` + `OpenSegment` via `ax`, assert
`GetChat` reflects both segments; then wipe the read-model DB and assert a read
rebuilds it via `Replay` (block on `SendWait`; no timing).

- [ ] **Step 2–4:** implement mirroring `reviewthread/internal/store/store.go`
(a `projections.NewStore` + `RegisterStore` save-only fold of `evt.Aggregate`
into an `agent_chats_read` row storing the JSON aggregate; `GetByProviderSession`
indexes segments' `ProviderSessionID`). Run — expect PASS. Commit.

---

## Task 8: Hub projection (WS broadcast)

**Files:**
- Create: `.../agentchat/internal/store/hub.go`
- Test: `.../agentchat/internal/store/hub_test.go`

**Interfaces:**
- Consumes: a `BroadcastFunc`.
- Produces: an independent `agentchat.*` subscriber broadcasting lifecycle
  frames (chat created / segment opened / segment ended / working toggled /
  renamed / deleted), replacing the usecase's manual `BroadcastAgentChat` calls.

- [ ] **Step 1: Failing test:** register hub with a capturing `BroadcastFunc`;
send a command via `ax`; assert one frame captured for the chat id (block on
`SendWait`).
- [ ] **Step 2–4:** implement mirroring `reviewthread/internal/store/hub.go`;
run — expect PASS; commit.

---

## Task 9: Container + adapter wiring

**Files:**
- Modify: `api/internal/adapter/container.go` (event-store + read-model DBs)
- Modify: `api/internal/app/container.go` (`axAgentChat`)
- Modify: `api/internal/app/repositories/container.go` (build `agentchat.New`)

**Interfaces:**
- Consumes: `newAsynx[domain.AgentChat](es)` helper (`app/asynx.go`).
- Produces: a wired `agentchat.Store` backed by `state/events/agent_chat.db` +
  `state/store/agent_chat.db`, handed to the usecase container.

- [ ] **Step 1: Failing test** (`adapter/container_test.go` add-on): opening the
container creates `state/events/agent_chat.db`; `AgentChatES()` non-nil.
- [ ] **Step 2:** add `AgentChatES()` (event store) + a read-model DB accessor in
`adapter/container.go`, mirroring `WorkspaceES()` / workspace read-model DB.
- [ ] **Step 3:** in `app/container.go`, `axAgentChat, err :=
newAsynx[domain.AgentChat](adapters.AgentChatES())`; retain on `Container` for
ordered shutdown drain; thread into `repositories.New`. In
`repositories/container.go`, build `agentchat.New(axAgentChat,
adapters.AgentChatES(), adapters.AgentChatReadDB(), broadcast)` and add it to
`WaitQuiescent` drain.
- [ ] **Step 4:** `go build -tags noEmbed ./...` + container tests — expect PASS.
- [ ] **Step 5: Commit.** `git commit -am "feat(agent): wire axAgentChat per-type event + read-model stores"`

---

## Task 10: Rewire the usecase (read→reduce→emit); delete `keyed_mutex.go`

**Files:**
- Modify: `api/internal/app/usecases/agent/agent.go`
- Delete: `api/internal/app/usecases/agent/keyed_mutex.go`
- Test: `.../agent/{agent_test,switch_test,handoff_test,race_test}.go` (adapt)

**Interfaces:**
- Consumes: the Task 6 `agentchat.Store` (now command-issuing).
- Produces: `IngestHook`, `SwitchProvider`, `Rename`, `SpawnChat` implemented as
  `store.GetChat` → engine reduce → IO (spawn/terminate PTY) → command call.
  **Continuity/resume preserved verbatim.**

- [ ] **Step 1: Adapt the failing tests.** Keep the existing behavioral tests
(switch-back resumes prior session; forward switch = handoff only; concurrent
switch tears down the orphan) but assert against aggregate reads via the store,
blocking on command completion — no sleeps.

- [ ] **Step 2: Replace persistence calls.** Every `SaveChat`/`SaveSegment`/
`GetActiveSegmentByCrowbarID` becomes a command call / `store.GetChat` /
`store.GetByProviderSession`. `IngestHook` per the reconcile pattern:
```go
chat, err := u.chats.GetChat(ctx, chatID)      // read
if err != nil { return err }
outcome := u.registry.Reduce(chat, ev)          // pure decide (which command)
// ...perform any IO the outcome requires (spawn on switch) OUTSIDE the command...
switch outcome.Kind {
case reduceStartTurn:
	return u.chats.StartTurn(ctx, chatID, ev.Now)
case reduceBindSession:
	return u.chats.BindSession(ctx, chatID, ev.CrowbarSegmentID, ev.SessionID)
// ...
}
```
The `SwitchProvider` prior-session lookup (`agent.go:646-649`) now scans
`chat.Segments` from the store read instead of a DB query — behavior identical.
**Concurrent-switch rule:** spawn the PTY, then `OpenSegment`; if it returns
`ErrValidation` (lost the race — active exists), tear down the just-spawned CLI
(`TerminateGraceful`) and return the conflict.

- [ ] **Step 3: Delete `keyed_mutex.go`** and the `segmentMutex` field/usage —
asynx per-aggregate OCC replaces it.

- [ ] **Step 4:** `go test -race ./api/internal/app/usecases/agent/...` — expect PASS.

- [ ] **Step 5: Commit.** `git commit -am "refactor(agent): usecase read→reduce→emit via asynx; delete keyed_mutex"`

---

## Task 11: Runtime process-exit + boot-reconcile reactors

**Files:**
- Create: `.../agentchat/internal/reactors/boot.go`
- Modify: `api/internal/app/usecases/agent/agent.go` (`onExit` callback)
- Test: reactor test + usecase onExit test

**Interfaces:**
- Produces: on CLI exit, `EndSegment` (+ `StopTurn` if `Working`); at boot, a
  reactor ends orphaned active segments / stops in-flight turns whose
  `TerminalSessionID` is not a live terminal session, and seeds the registry
  from each segment's `ProviderSessionID` (`agent.go:722-725`).

- [ ] **Step 1: Failing tests:** (a) invoking the `onExit` callback for an active
segment emits `EndSegment` and clears `Working`; (b) boot with a chat whose
active segment's terminal session is absent → after reconcile the segment is
`ended` and `Working==false`. Block on command completion; inject a fake
"is-terminal-alive" predicate — no timing.

- [ ] **Step 2: Implement.** Wire the `TerminalCommander.CreateCommand` `onExit`
to call `u.chats.EndSegment` (+ `StopTurn` when `Working`). Add
`reactors/boot.go`: `ReconcileOnBoot(ctx, chats, isAlive)` iterates
`chats.ListChats`, and for each with an active segment where `!isAlive(seg.TerminalSessionID)`
issues `EndSegment`/`StopTurn`; then seeds the registry. Call it from the app
container startup after the usecase is built.

- [ ] **Step 3:** `go test ./api/internal/app/repositories/agentchat/internal/reactors/... ./api/internal/app/usecases/agent/...` — PASS.

- [ ] **Step 4: Commit.** `git commit -am "feat(agent): runtime process-exit + boot-reconcile of live turn state"`

---

## Task 12: Workspace-delete cascade + AgentChat delete reactor

**Files:**
- Create: `.../agentchat/internal/reactors/delete.go`
- Modify: `api/internal/app/repositories/container.go` (cascade wiring)
- Test: cascade test

**Interfaces:**
- Produces: (a) an `agentchat.deleted.*` reactor that terminates the chat's
  active PTY; (b) the `Workspace` delete reactor also `Forget`s the workspace's
  `AgentChat`s (and their PTYs), mirroring its existing `ReviewThread` cascade
  (`repositories/container.go` `forgetReviewThreads`).

- [ ] **Step 1: Failing test:** deleting a workspace `Forget`s an `AgentChat`
bound to it (assert `Exists`→false after the cascade; block on `SendWait`).
- [ ] **Step 2: Implement** the delete reactor (PTY teardown) + a
`forgetAgentChats` callback wired into the workspace delete reactor alongside
`forgetReviewThreads`.
- [ ] **Step 3:** `go test ./api/internal/app/repositories/...` — PASS.
- [ ] **Step 4: Commit.** `git commit -am "feat(agent): cascade workspace delete to agent chats + PTY teardown"`

---

## Task 13: Real-CLI integration + live Tauri verification

**Files:**
- Modify/Create: `api/tests/integration/agent/{agent_test,agent_title_test,agent_gaps_test}.go`

**Interfaces:**
- Produces: black-box `TestRegression_*` proving the aggregate end-to-end, and a
  documented live check.

- [ ] **Step 1:** Update the integration suite (`//go:build integration`) to
drive real hooks through the HTTP endpoints and assert aggregate state via the
read API. Key case — the UC you raised: **Claude → Codex → back to Claude**:
assert 3 segments, segments 1 & 3 share `ProviderSessionID`, the switch-back CLI
argv contains the `session.resume` token AND the handoff. Block on
`WaitQuiescent`; no timing.
- [ ] **Step 2:** `go test -tags integration ./api/tests/integration/agent/...` — PASS.
- [ ] **Step 3: Live** on `make dev-desktop` (never prod): open an agent chat,
run a turn (icon → working, then idle), switch Claude↔Codex↔Claude, rename,
restart the daemon mid-turn and confirm the boot reactor clears the stuck
`Working`. Capture via DOM/screenshot per the Tauri live-test gotchas.
- [ ] **Step 4: Commit.** `git commit -am "test(agent): real-CLI integration for asynx AgentChat + live verification"`

---

## Self-Review

- **Spec coverage:** aggregate (T2), commands incl. all 8 events (T3–5),
  read→reduce→emit + keyed_mutex deletion (T10), store + hub projections (T7–8),
  boot-reconcile + runtime exit (T11), deletes/cascade (T12), continuity/resume
  (T10 + T13), ledger split (respected — no command carries message bodies;
  content stays in the ledger), no-migration (T6 drops gorm), wiring (T9),
  testing incl. the Claude→Codex→Claude UC (T13). Merge baseline (T1). ✓
- **Placeholder scan:** command code is complete for T3–4; T5/T6–8/T11–12 give
  the representative code + the exact `ReviewThread`/`Workspace` file to mirror
  and the precise method set / behavior — no "TODO"/"handle errors" hand-waves.
- **Type consistency:** method names frozen in T6's `Store` interface are the
  ones T10 calls (`Create/OpenSegment/EndSegment/BindSession/StartTurn/StopTurn/
  SetTitle/Delete` + reads); `AgentChatStatus*` literals consistent across T2/T5;
  event-name scheme `agentchat.<verb>.<id>` consistent across T3–5 and the T8 hub
  subscription (`agentchat.*`).

## Remaining judgment call (from the design)

Keep the **ledger** a separate append-only disk store (recommended) vs. later
making it a second asynx stream. This plan assumes **separate** — no task
touches ledger persistence; the aggregate only stores a `LedgerCursor`.
