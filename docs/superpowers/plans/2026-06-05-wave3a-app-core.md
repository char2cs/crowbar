# Wave 3A — App-Layer Aggregates, Read Models, Usecases & Hub Projections Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. **Invoke the `go-style` skill before writing any Go.**

**Goal:** Land the application-layer integration core — the full aggregate command sets, GORM-backed read-model projections, the project-import usecase, the engine-composing usecases, `RegisterHubProjections`, the AgentRun crash-recovery two-pass, and the provider-poll → `SyncProviderState` wiring — so a git mutation updates a Workspace row over WebSocket end-to-end.

**Architecture:** Event-sourced aggregates (Workspace, Chat, AgentRun, ReviewThread) via Asynx; each gets a GORM read-model projection table (`internal/store/{storage,projections}`) mirroring `quiver.core/internal/app/repositories/arrow`. Projections subscribe to Asynx events, upsert a JSON view-model row, and fan out through the typed `WebSocketHub`. Usecases compose the Wave-1/Wave-2 engines (git, fs, provider, terminal) with the aggregate repositories. Layer order is strict: `engine → adapter → app → api`.

**Tech Stack:** Go 1.26.2, `github.com/char2cs/asynx`, GORM + glebarez SQLite, gin, testify. Module `github.com/char2cs/crowbar/api`.

---

## ⛔ Rabbyte standards gate (a reviewer checks EACH item)

1. Mirror `quiver.core` exactly: `internal/app/repositories/<aggregate>/internal/{commands,store,projections,mocks}`.
2. One domain concept per file; one `_test.go` per source file (except struct-only files); source files **< 500 LOC**.
3. **One parameter per line, always** — signatures and multi-arg calls; closing paren on its own line.
4. **Early returns always**, no `else`; **max 2 indentation levels** in a function body (skill rule: level 3 must never exist).
5. Coverage **≥95%** (target 100%); no flaky tests; **no `time.Sleep` in tests** — use condition-based waits.
6. Benchmarks (`*_bench_test.go`) for hot paths: projection upsert, aggregate reconstruction, read-model `FindAll`.
7. CLEAN: guard clauses, `fmt.Errorf("op: ctx: %w", err)`, gofumpt + goimports; obey `.golangci.yml` (funlen 100/50, gocyclo 15, nestif ≤2).

**Verification command run after every task:** `cd api && gofumpt -l -w . && goimports -w . && go build ./... && go vet ./... && go test ./internal/app/...`

---

## File Structure

This plan creates/modifies (all under `api/`):

**Domain extensions (struct/enum files — no `_test.go` for struct-only):**
- Modify `internal/domain/chat.go` — add `Title`, `ParentID`, `Type`, `DeletedAt`.
- Create `internal/domain/chat_type.go` — `ChatType` enum.
- Modify `internal/domain/review_thread.go` — add `FilePath`, `LineNumber`, `Side`, `Messages`.
- Create `internal/domain/review_message.go` — `ReviewMessage` struct.
- Create `internal/domain/review_side.go` — `ReviewSide` enum.

**Workspace aggregate (`internal/app/repositories/workspace/`):**
- Create `internal/commands/{sync_provider_state,set_merge_strategy,touch_activity,reparent,update_fork_point,set_pending_merge,clear_pending_merge}.go` (+ shared `commands_test.go` additions).
- Create `internal/store/{store.go,storage.go,projections.go}` + tests + `store_bench_test.go`.
- Create `internal/mocks/mocks.go`.
- Modify `workspace.go` — add the new repository methods + `List`.

**Chat aggregate (`internal/app/repositories/chat/`):**
- Create `internal/commands/{fork,rename,delete}.go`; modify `create.go` (title/type).
- Create `internal/store/{store.go,storage.go,projections.go}` + tests + bench.
- Create `internal/mocks/mocks.go`.
- Modify `chat.go` — add `Fork`, `Rename`, `Delete`, `List`, `ListByWorkspace`.

**AgentRun aggregate (`internal/app/repositories/agentrun/`):**
- Create `internal/store/{store.go,storage.go,projections.go}` + tests + bench.
- Modify `agentrun.go` — add `List`, `ListRunning`.

**ReviewThread aggregate (`internal/app/repositories/reviewthread/`):**
- Modify `internal/commands/open.go` (full payload); create `internal/commands/reply.go`.
- Create `internal/store/{store.go,storage.go,projections.go}` + tests + bench.
- Create `internal/mocks/mocks.go`.
- Modify `reviewthread.go` — add `Reply`, `List`, `ListByWorkspace`.

**Repositories container + hub projections + recovery:**
- Modify `internal/app/repositories/container.go` — build read-model stores, wire `RegisterHubProjections`, real `RecoverOrphans`.
- Create `internal/app/repositories/agent_run_projection.go` — AgentRun → Chat + Workspace overlay.
- Modify `internal/app/repositories/recovery.go` — enumerate from read models.

**GORM CRUD usecases:**
- Create `internal/app/usecases/{container,project_import,project,repository,workspace,chat,file,git,terminal,provider_sync}.go` + tests + mocks.
- Create `internal/app/usecases/internal/avatar/avatar.go` — avatar label/color generation.

**Wiring:**
- Modify `internal/app/container.go` — construct usecases, expose them; start provider sweep.

---

## Phase 0 — Domain extensions

### Task 1: Extend the Chat domain struct

**Files:**
- Modify: `internal/domain/chat.go`
- Create: `internal/domain/chat_type.go`

- [ ] **Step 1: Add the ChatType enum**

Create `internal/domain/chat_type.go`:

```go
package domain

// ChatType marks a chat as an ordinary conversation or a workflow. v0 only ever
// writes "chat"; "workflow" is a forward-compat marker with no v0 writer (01 §2).
type ChatType string

const (
	ChatTypeChat     ChatType = "chat"
	ChatTypeWorkflow ChatType = "workflow"
)
```

- [ ] **Step 2: Extend the Chat struct**

Replace `internal/domain/chat.go` with:

```go
package domain

import "time"

// Chat is the conversation aggregate; only its lifecycle is event-sourced here —
// turn content is deferred to the Agentic Bridge spike (00 §5.4, 01 §2).
type Chat struct {
	ID        string     `json:"id"`
	WsID      string     `json:"wsId"`
	Title     string     `json:"title"`
	ParentID  string     `json:"parentId,omitempty"`
	Status    ChatStatus `json:"status"`
	Type      ChatType   `json:"type"`
	CreatedAt time.Time  `json:"createdAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}
```

- [ ] **Step 3: Build to verify struct compiles**

Run: `cd api && go build ./internal/domain/...`
Expected: success (existing `create.go` still compiles — it sets a subset of fields).

- [ ] **Step 4: Commit**

```bash
git add api/internal/domain/chat.go api/internal/domain/chat_type.go
git commit -m "feat(domain): extend Chat with title/parent/type/deletedAt (01 §2)"
```

### Task 2: Extend the ReviewThread domain struct

**Files:**
- Modify: `internal/domain/review_thread.go`
- Create: `internal/domain/review_message.go`
- Create: `internal/domain/review_side.go`

- [ ] **Step 1: Add the ReviewSide enum**

Create `internal/domain/review_side.go`:

```go
package domain

// ReviewSide is which side of a diff a review thread is anchored to (09 §3).
type ReviewSide string

const (
	ReviewSideLeft  ReviewSide = "left"
	ReviewSideRight ReviewSide = "right"
)
```

- [ ] **Step 2: Add the ReviewMessage struct**

Create `internal/domain/review_message.go`:

```go
package domain

import "time"

// ReviewMessage is one append-only message inside a ReviewThread (09 §3).
type ReviewMessage struct {
	ID        string    `json:"id"`
	Author    string    `json:"author,omitempty"`
	IsAgent   bool      `json:"isAgent"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}
```

- [ ] **Step 3: Extend the ReviewThread struct**

Replace `internal/domain/review_thread.go` with:

```go
package domain

import "time"

// ReviewThread is the branch-review comment-thread aggregate (00 §6.3, 09 §3).
// Messages are kept inside the aggregate because a thread is bounded.
type ReviewThread struct {
	ID         string             `json:"id"`
	WsID       string             `json:"wsId"`
	FilePath   string             `json:"filePath"`
	LineNumber int                `json:"lineNumber"`
	Side       ReviewSide         `json:"side"`
	Status     ReviewThreadStatus `json:"status"`
	Messages   []ReviewMessage    `json:"messages"`
	CreatedAt  time.Time          `json:"createdAt"`
}

// IsResolved reports whether the thread is resolved (09 §3 read model).
func (t ReviewThread) IsResolved() bool {
	return t.Status == ReviewThreadStatusResolved
}
```

- [ ] **Step 4: Build**

Run: `cd api && go build ./internal/domain/...`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add api/internal/domain/review_thread.go api/internal/domain/review_message.go api/internal/domain/review_side.go
git commit -m "feat(domain): extend ReviewThread with anchor + messages (09 §3)"
```

---

## Phase 1 — Workspace aggregate command set

> The Workspace aggregate owns every field on the sidebar row (00 §5.3). 3A lands the **full command set**; 3D builds the usecases that orchestrate these commands with git primitives. Each command is its own file (one concept per file). Field ownership is disjoint (00 §5.3 table).

### Task 3: `SyncProviderState` command

**Files:**
- Create: `internal/app/repositories/workspace/internal/commands/sync_provider_state.go`
- Test: `internal/app/repositories/workspace/internal/commands/commands_test.go` (append)

- [ ] **Step 1: Write the failing test (append to `commands_test.go`)**

```go
func TestSyncProviderState_SetsPROpenAndLocked(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusNew}
	cmd := SyncProviderState{
		ID:             "w1",
		Protected:      true,
		HasPR:          true,
		PRStatus:       "open",
		PRUrl:          "https://x/pr/1",
		PRTitle:        "t",
		PRTargetBranch: "main",
		Now:            time.Unix(3000, 0),
	}
	ws := cmd.EmitEvent(cur)
	assert.Equal(t, domain.WorkspaceStatusPROpen, ws.Status)
	assert.True(t, ws.Locked)
	assert.Equal(t, "https://x/pr/1", ws.PRUrl)
	assert.Equal(t, "main", ws.PRTargetBranch)
}

func TestSyncProviderState_NoPRLeavesStatusButSetsLocked(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: ""}
	cmd := SyncProviderState{ID: "w1", Protected: true, HasPR: false}
	ws := cmd.EmitEvent(cur)
	assert.Equal(t, domain.WorkspaceStatus(""), ws.Status)
	assert.True(t, ws.Locked)
	assert.Empty(t, ws.PRUrl)
}

func TestSyncProviderState_PRClosedToOpenAllowed(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusPRClosed}
	cmd := SyncProviderState{ID: "w1", HasPR: true, PRStatus: "open"}
	ws := cmd.EmitEvent(cur)
	assert.Equal(t, domain.WorkspaceStatusPROpen, ws.Status)
}

func TestSyncProviderState_Validate_RejectsMissing(t *testing.T) {
	err := SyncProviderState{ID: "w1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/commands/ -run TestSyncProviderState`
Expected: FAIL — `undefined: SyncProviderState`.

- [ ] **Step 3: Implement `sync_provider_state.go`**

```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SyncProviderState applies a provider poll result: PR status + protected flag
// (08 §5). It only ever writes pr-* statuses (never touches "new"; you cannot
// have a PR without commits) and `locked` (00 §6.1).
type SyncProviderState struct {
	ID             string
	Protected      bool
	HasPR          bool
	PRStatus       string
	PRUrl          string
	PRTitle        string
	PRTargetBranch string
	Now            time.Time
}

func (c SyncProviderState) AggregateID() string {
	return c.ID
}

func (c SyncProviderState) EventName() string {
	return "workspace.provider_synced." + c.ID
}

func (c SyncProviderState) ShouldSnapshot() bool {
	return true
}

func (c SyncProviderState) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("sync provider state: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SyncProviderState) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.Locked = c.Protected
	if !c.HasPR {
		return ws
	}
	ws.Status = prStatusToWorkspace(c.PRStatus)
	ws.PRUrl = c.PRUrl
	ws.PRTitle = c.PRTitle
	ws.PRTargetBranch = c.PRTargetBranch
	return ws
}

func prStatusToWorkspace(
	status string,
) domain.WorkspaceStatus {
	switch status {
	case "open":
		return domain.WorkspaceStatusPROpen
	case "merged":
		return domain.WorkspaceStatusPRMerged
	case "closed":
		return domain.WorkspaceStatusPRClosed
	default:
		return domain.WorkspaceStatusPROpen
	}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/commands/ -run TestSyncProviderState`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/repositories/workspace/internal/commands/
git commit -m "feat(workspace): SyncProviderState command (08 §5)"
```

### Task 4: `SetMergeStrategy` + `TouchActivity` commands

**Files:**
- Create: `internal/app/repositories/workspace/internal/commands/set_merge_strategy.go`
- Create: `internal/app/repositories/workspace/internal/commands/touch_activity.go`
- Test: append to `commands_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestSetMergeStrategy_OnlyWritesStrategy(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", Added: 5, MergeStrategy: gitdomain.MergeStrategyMerge}
	ws := SetMergeStrategy{ID: "w1", Strategy: gitdomain.MergeStrategyRebase}.EmitEvent(cur)
	assert.Equal(t, gitdomain.MergeStrategyRebase, ws.MergeStrategy)
	assert.Equal(t, 5, ws.Added)
}

func TestSetMergeStrategy_Validate_RejectsMissing(t *testing.T) {
	err := SetMergeStrategy{ID: "w1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestTouchActivity_OnlyBumpsLastActivity(t *testing.T) {
	now := time.Unix(9000, 0)
	cur := &domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusPROpen}
	ws := TouchActivity{ID: "w1", Now: now}.EmitEvent(cur)
	assert.Equal(t, now, ws.LastActivity)
	assert.Equal(t, domain.WorkspaceStatusPROpen, ws.Status)
}

func TestTouchActivity_Validate_RejectsMissing(t *testing.T) {
	err := TouchActivity{ID: "w1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/commands/ -run 'TestSetMergeStrategy|TestTouchActivity'`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `set_merge_strategy.go`**

```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// SetMergeStrategy writes only the mergeStrategy field (09 §4, 00 §5.3).
type SetMergeStrategy struct {
	ID       string
	Strategy gitdomain.MergeStrategy
}

func (c SetMergeStrategy) AggregateID() string {
	return c.ID
}

func (c SetMergeStrategy) EventName() string {
	return "workspace.merge_strategy_set." + c.ID
}

func (c SetMergeStrategy) ShouldSnapshot() bool {
	return false
}

func (c SetMergeStrategy) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("set merge strategy: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetMergeStrategy) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.MergeStrategy = c.Strategy
	return ws
}
```

- [ ] **Step 4: Implement `touch_activity.go`**

```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// TouchActivity bumps only lastActivity, representing chat/agent activity (01 §5).
type TouchActivity struct {
	ID  string
	Now time.Time
}

func (c TouchActivity) AggregateID() string {
	return c.ID
}

func (c TouchActivity) EventName() string {
	return "workspace.activity_touched." + c.ID
}

func (c TouchActivity) ShouldSnapshot() bool {
	return false
}

func (c TouchActivity) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("touch activity: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c TouchActivity) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.LastActivity = c.Now
	return ws
}
```

- [ ] **Step 5: Run to verify pass**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/commands/ -run 'TestSetMergeStrategy|TestTouchActivity'`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/app/repositories/workspace/internal/commands/
git commit -m "feat(workspace): SetMergeStrategy + TouchActivity commands"
```

### Task 5: Hierarchy commands — `Reparent`, `UpdateForkPoint`, `SetPendingMerge`, `ClearPendingMerge`

**Files:**
- Create: `internal/app/repositories/workspace/internal/commands/reparent.go`
- Create: `internal/app/repositories/workspace/internal/commands/update_fork_point.go`
- Create: `internal/app/repositories/workspace/internal/commands/set_pending_merge.go`
- Create: `internal/app/repositories/workspace/internal/commands/clear_pending_merge.go`
- Test: append to `commands_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestReparent_SetsParentAndForkPoint(t *testing.T) {
	now := time.Unix(4000, 0)
	cur := &domain.Workspace{ID: "w1", ParentID: "old", ForkPointSha: "oldsha"}
	ws := Reparent{ID: "w1", ParentID: "new", ForkPointSha: "newsha", Now: now}.EmitEvent(cur)
	assert.Equal(t, "new", ws.ParentID)
	assert.Equal(t, "newsha", ws.ForkPointSha)
	assert.Equal(t, now, ws.LastActivity)
}

func TestReparent_Validate_RejectsMissingParent(t *testing.T) {
	err := Reparent{ID: "w1"}.Validate(&domain.Workspace{ID: "w1"})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestUpdateForkPoint_OnlyWritesForkPoint(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", ForkPointSha: "a", ParentID: "p"}
	ws := UpdateForkPoint{ID: "w1", ForkPointSha: "b"}.EmitEvent(cur)
	assert.Equal(t, "b", ws.ForkPointSha)
	assert.Equal(t, "p", ws.ParentID)
}

func TestSetPendingMerge_SetsMarker(t *testing.T) {
	cur := &domain.Workspace{ID: "w1"}
	ws := SetPendingMerge{
		ID:             "w1",
		Strategy:       gitdomain.MergeStrategyRebase,
		TargetParentID: "p",
	}.EmitEvent(cur)
	require.NotNil(t, ws.PendingMerge)
	assert.Equal(t, "p", ws.PendingMerge.TargetParentID)
	assert.Equal(t, gitdomain.MergeStrategyRebase, ws.PendingMerge.Strategy)
}

func TestClearPendingMerge_ClearsMarker(t *testing.T) {
	cur := &domain.Workspace{ID: "w1", PendingMerge: &gitdomain.PendingMerge{TargetParentID: "p"}}
	ws := ClearPendingMerge{ID: "w1"}.EmitEvent(cur)
	assert.Nil(t, ws.PendingMerge)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/commands/ -run 'TestReparent|TestUpdateForkPoint|TestSetPendingMerge|TestClearPendingMerge'`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `reparent.go`**

```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Reparent re-points a workspace at a new parent and records the new fork point
// (07 §4). The leaf-guard lives in the usecase; the command only mutates fields.
type Reparent struct {
	ID           string
	ParentID     string
	ForkPointSha string
	Now          time.Time
}

func (c Reparent) AggregateID() string {
	return c.ID
}

func (c Reparent) EventName() string {
	return "workspace.reparented." + c.ID
}

func (c Reparent) ShouldSnapshot() bool {
	return true
}

func (c Reparent) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("reparent: %w", asynxModels.ErrValidation)
	}
	if c.ParentID == "" || c.ForkPointSha == "" {
		return fmt.Errorf("reparent: missing parent or fork point: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Reparent) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.ParentID = c.ParentID
	ws.ForkPointSha = c.ForkPointSha
	ws.LastActivity = c.Now
	return ws
}
```

- [ ] **Step 4: Implement `update_fork_point.go`**

```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// UpdateForkPoint resets a kept child's forkPointSha to the parent's post-merge
// tip after a local merge, for every strategy (07 §3.1).
type UpdateForkPoint struct {
	ID           string
	ForkPointSha string
}

func (c UpdateForkPoint) AggregateID() string {
	return c.ID
}

func (c UpdateForkPoint) EventName() string {
	return "workspace.fork_point_updated." + c.ID
}

func (c UpdateForkPoint) ShouldSnapshot() bool {
	return false
}

func (c UpdateForkPoint) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("update fork point: %w", asynxModels.ErrValidation)
	}
	if c.ForkPointSha == "" {
		return fmt.Errorf("update fork point: missing sha: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c UpdateForkPoint) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.ForkPointSha = c.ForkPointSha
	return ws
}
```

- [ ] **Step 5: Implement `set_pending_merge.go`**

```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// SetPendingMerge records a conflicted merge-into-parent awaiting resolution
// (07 §3.1, 04 §6.1).
type SetPendingMerge struct {
	ID             string
	Strategy       gitdomain.MergeStrategy
	TargetParentID string
}

func (c SetPendingMerge) AggregateID() string {
	return c.ID
}

func (c SetPendingMerge) EventName() string {
	return "workspace.pending_merge_set." + c.ID
}

func (c SetPendingMerge) ShouldSnapshot() bool {
	return false
}

func (c SetPendingMerge) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("set pending merge: %w", asynxModels.ErrValidation)
	}
	if c.TargetParentID == "" {
		return fmt.Errorf("set pending merge: missing target: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetPendingMerge) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.PendingMerge = &gitdomain.PendingMerge{
		Strategy:       c.Strategy,
		TargetParentID: c.TargetParentID,
	}
	return ws
}
```

- [ ] **Step 6: Implement `clear_pending_merge.go`**

```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ClearPendingMerge removes the pendingMerge marker on continue/abort (07 §3.1).
type ClearPendingMerge struct {
	ID string
}

func (c ClearPendingMerge) AggregateID() string {
	return c.ID
}

func (c ClearPendingMerge) EventName() string {
	return "workspace.pending_merge_cleared." + c.ID
}

func (c ClearPendingMerge) ShouldSnapshot() bool {
	return false
}

func (c ClearPendingMerge) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("clear pending merge: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ClearPendingMerge) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.PendingMerge = nil
	return ws
}
```

- [ ] **Step 7: Run to verify pass**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/commands/`
Expected: PASS (all command tests).

- [ ] **Step 8: Commit**

```bash
git add api/internal/app/repositories/workspace/internal/commands/
git commit -m "feat(workspace): hierarchy commands — reparent/forkpoint/pendingMerge (07)"
```

### Task 6: Expose the new commands on the Workspace repository facade

**Files:**
- Modify: `internal/app/repositories/workspace/workspace.go`
- Test: `internal/app/repositories/workspace/workspace_test.go` (append)

- [ ] **Step 1: Write failing tests (append to `workspace_test.go`)**

```go
func TestWorkspace_SyncProviderState_SetsPR(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)

	got, err := repo.SyncProviderState(ctx, workspace.ProviderInput{
		ID:        "w1",
		Protected: true,
		HasPR:     true,
		PRStatus:  "open",
		PRUrl:     "u",
	}, now)
	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceStatusPROpen, got.Status)
	assert.True(t, got.Locked)
}

func TestWorkspace_SetMergeStrategy(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)
	got, err := repo.SetMergeStrategy(ctx, "w1", gitdomain.MergeStrategySquash)
	require.NoError(t, err)
	assert.Equal(t, gitdomain.MergeStrategySquash, got.MergeStrategy)
}

func TestWorkspace_Reparent_TouchActivity_ForkPoint_Pending(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)

	_, err = repo.TouchActivity(ctx, "w1", now)
	require.NoError(t, err)
	rp, err := repo.Reparent(ctx, "w1", "p2", "sha2", now)
	require.NoError(t, err)
	assert.Equal(t, "p2", rp.ParentID)
	fp, err := repo.UpdateForkPoint(ctx, "w1", "sha3")
	require.NoError(t, err)
	assert.Equal(t, "sha3", fp.ForkPointSha)
	pm, err := repo.SetPendingMerge(ctx, "w1", gitdomain.MergeStrategyMerge, "p2")
	require.NoError(t, err)
	require.NotNil(t, pm.PendingMerge)
	cl, err := repo.ClearPendingMerge(ctx, "w1")
	require.NoError(t, err)
	assert.Nil(t, cl.PendingMerge)
}

func TestWorkspace_Delete_Forgets(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)
	require.NoError(t, repo.Delete(ctx, "w1"))
	_, err = repo.Get(ctx, "w1")
	assert.Error(t, err)
}
```

Add the import `gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"` to the test file.

- [ ] **Step 2: Run to verify failure**

Run: `cd api && go test ./internal/app/repositories/workspace/ -run 'SyncProviderState|SetMergeStrategy|Reparent|Delete_Forgets'`
Expected: FAIL — undefined methods.

- [ ] **Step 3: Extend the `Workspace` interface and impl in `workspace.go`**

Add `ProviderInput` near `SyncInput`:

```go
// ProviderInput carries a provider poll result (08 §5).
type ProviderInput struct {
	ID             string
	Protected      bool
	HasPR          bool
	PRStatus       string
	PRUrl          string
	PRTitle        string
	PRTargetBranch string
}
```

Add to the `Workspace` interface (one param per line each):

```go
	SyncProviderState(
		ctx context.Context,
		in ProviderInput,
		now time.Time,
	) (domain.Workspace, error)
	SetMergeStrategy(
		ctx context.Context,
		id string,
		strategy gitdomain.MergeStrategy,
	) (domain.Workspace, error)
	TouchActivity(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
	Reparent(
		ctx context.Context,
		id string,
		parentID string,
		forkPointSha string,
		now time.Time,
	) (domain.Workspace, error)
	UpdateForkPoint(
		ctx context.Context,
		id string,
		forkPointSha string,
	) (domain.Workspace, error)
	SetPendingMerge(
		ctx context.Context,
		id string,
		strategy gitdomain.MergeStrategy,
		targetParentID string,
	) (domain.Workspace, error)
	ClearPendingMerge(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	Delete(
		ctx context.Context,
		id string,
	) error
```

> **Do NOT add `List` to the interface in this task** — `List` reads the read-model store, which does not exist until Task 9. Add the `List` interface method and its implementation in Task 9 (no stubs in between).

Add the method implementations (each calls `SendWait` and wraps the error `fmt.Errorf("workspace: <op>: %w", err)`):

```go
func (w *workspace) SyncProviderState(
	ctx context.Context,
	in ProviderInput,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.SyncProviderState{
		ID:             in.ID,
		Protected:      in.Protected,
		HasPR:          in.HasPR,
		PRStatus:       in.PRStatus,
		PRUrl:          in.PRUrl,
		PRTitle:        in.PRTitle,
		PRTargetBranch: in.PRTargetBranch,
		Now:            now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: sync provider: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) SetMergeStrategy(
	ctx context.Context,
	id string,
	strategy gitdomain.MergeStrategy,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.SetMergeStrategy{ID: id, Strategy: strategy})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set merge strategy: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) TouchActivity(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.TouchActivity{ID: id, Now: now})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: touch activity: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) Reparent(
	ctx context.Context,
	id string,
	parentID string,
	forkPointSha string,
	now time.Time,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.Reparent{
		ID:           id,
		ParentID:     parentID,
		ForkPointSha: forkPointSha,
		Now:          now,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: reparent: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) UpdateForkPoint(
	ctx context.Context,
	id string,
	forkPointSha string,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.UpdateForkPoint{ID: id, ForkPointSha: forkPointSha})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: update fork point: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) SetPendingMerge(
	ctx context.Context,
	id string,
	strategy gitdomain.MergeStrategy,
	targetParentID string,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.SetPendingMerge{
		ID:             id,
		Strategy:       strategy,
		TargetParentID: targetParentID,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set pending merge: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) ClearPendingMerge(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.ClearPendingMerge{ID: id})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: clear pending merge: %w", err)
	}
	return evt.Aggregate, nil
}

func (w *workspace) Delete(
	ctx context.Context,
	id string,
) error {
	if err := w.ax.Forget(ctx, id); err != nil {
		return fmt.Errorf("workspace: delete: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run to verify pass**

Run: `cd api && go test ./internal/app/repositories/workspace/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/repositories/workspace/
git commit -m "feat(workspace): expose provider/strategy/hierarchy/delete on repo facade"
```

---

## Phase 2 — Workspace read model (GORM projection)

> Mirrors `quiver.core/internal/app/repositories/arrow/internal/store`. The store: a `storage` layer (GORM JSON row), a `projections.Register` subscribing to Asynx events to upsert the row + broadcast through the hub, and a `Store` facade exposing `List`/`Get`. This is the read model the sidebar, crash-recovery enumeration, and provider sweep all read.

### Task 7: Workspace storage layer (GORM JSON view-model)

**Files:**
- Create: `internal/app/repositories/workspace/internal/store/storage.go`
- Test: `internal/app/repositories/workspace/internal/store/storage_test.go`

- [ ] **Step 1: Write the failing test**

```go
package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newStorage(
	t *testing.T,
) (context.Context, storage) {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := newStorageStore(db)
	require.NoError(t, err)
	return context.Background(), st
}

func TestStorage_SaveFindDelete(t *testing.T) {
	ctx, st := newStorage(t)
	ws := domain.Workspace{ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b", CreatedAt: time.Unix(1, 0).UTC()}
	require.NoError(t, st.Save(ctx, ws))

	got, err := st.FindByKey(ctx, "w1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "b", got.Branch)

	all, err := st.FindAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)

	require.NoError(t, st.Delete(ctx, "w1"))
	got, err = st.FindByKey(ctx, "w1")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestStorage_FindByKey_MissingReturnsNil(t *testing.T) {
	ctx, st := newStorage(t)
	got, err := st.FindByKey(ctx, "nope")
	require.NoError(t, err)
	assert.Nil(t, got)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/store/`
Expected: FAIL — `undefined: storage` / `newStorageStore`.

- [ ] **Step 3: Implement `storage.go`**

```go
package store

import (
	"context"
	"encoding/json"
	"fmt"

	gormdb "gorm.io/gorm"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type workspaceRow struct {
	ID   string `gorm:"primaryKey;column:id"`
	Data []byte `gorm:"column:data"`
}

func (workspaceRow) TableName() string {
	return "read_workspaces"
}

type storage interface {
	Save(
		ctx context.Context,
		ws domain.Workspace,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Workspace, error)
	FindAll(
		ctx context.Context,
	) ([]domain.Workspace, error)
}

type storageStore struct {
	inner interface {
		Save(ctx context.Context, row workspaceRow) error
		Delete(ctx context.Context, key string) error
		FindByKey(ctx context.Context, key string) (*workspaceRow, error)
		FindAll(ctx context.Context) ([]workspaceRow, error)
	}
}

func newStorageStore(
	db *gormdb.DB,
) (storage, error) {
	inner, err := storesqlite.NewFromDB[workspaceRow, string](db)
	if err != nil {
		return nil, fmt.Errorf("workspace storage: %w", err)
	}
	return &storageStore{inner: inner}, nil
}

func (s *storageStore) Save(
	ctx context.Context,
	ws domain.Workspace,
) error {
	data, err := json.Marshal(ws)
	if err != nil {
		return fmt.Errorf("workspace storage: marshal: %w", err)
	}
	return s.inner.Save(ctx, workspaceRow{ID: ws.ID, Data: data})
}

func (s *storageStore) Delete(
	ctx context.Context,
	id string,
) error {
	return s.inner.Delete(ctx, id)
}

func (s *storageStore) FindByKey(
	ctx context.Context,
	id string,
) (*domain.Workspace, error) {
	row, err := s.inner.FindByKey(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("workspace storage: find: %w", err)
	}
	if row == nil {
		return nil, nil
	}
	return unmarshalWorkspace(row.Data)
}

func (s *storageStore) FindAll(
	ctx context.Context,
) ([]domain.Workspace, error) {
	rows, err := s.inner.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace storage: find all: %w", err)
	}
	result := make([]domain.Workspace, 0, len(rows))
	for _, row := range rows {
		ws, err := unmarshalWorkspace(row.Data)
		if err != nil {
			return nil, err
		}
		result = append(result, *ws)
	}
	return result, nil
}

func unmarshalWorkspace(
	data []byte,
) (*domain.Workspace, error) {
	var ws domain.Workspace
	if err := json.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("workspace storage: unmarshal: %w", err)
	}
	return &ws, nil
}
```

> Note: `adapter/store/sqlite.NewFromDB` is an adapter-layer symbol; importing it from the app layer is allowed (`app → adapter`). This matches quiver.core's storage importing `adapter/store/sqlite`.

- [ ] **Step 4: Run to verify pass**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/store/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/repositories/workspace/internal/store/
git commit -m "feat(workspace): GORM read-model storage layer"
```

### Task 8: Workspace projection (Asynx events → read model + hub broadcast)

**Files:**
- Create: `internal/app/repositories/workspace/internal/store/projections.go`
- Test: `internal/app/repositories/workspace/internal/store/projections_test.go`
- Bench: `internal/app/repositories/workspace/internal/store/store_bench_test.go`

- [ ] **Step 1: Write the failing test**

```go
package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	wscmds "github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type captureHub struct {
	mu   sync.Mutex
	rows []domain.Workspace
}

func (h *captureHub) push(ws domain.Workspace) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rows = append(h.rows, ws)
}

func (h *captureHub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.rows)
}

func newProjected(
	t *testing.T,
) (context.Context, asynx.Asynx[domain.Workspace], storage, *captureHub) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := newStorageStore(db)
	require.NoError(t, err)

	h := &captureHub{}
	require.NoError(t, registerProjections(st, ax, h.push))
	return context.Background(), ax, st, h
}

func TestProjection_CreateUpsertsRowAndBroadcasts(t *testing.T) {
	ctx, ax, st, h := newProjected(t)
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	row, err := st.FindByKey(ctx, "w1")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "b", row.Branch)
	assert.GreaterOrEqual(t, h.count(), 1)
}

func TestProjection_ForgetDeletesRow(t *testing.T) {
	ctx, ax, st, _ := newProjected(t)
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{ID: "w1", RepoID: "r1", ProjectID: "p1", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)
	require.NoError(t, ax.Forget(ctx, "w1"))

	row, err := st.FindByKey(ctx, "w1")
	require.NoError(t, err)
	assert.Nil(t, row)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/store/ -run TestProjection`
Expected: FAIL — `undefined: registerProjections`.

- [ ] **Step 3: Implement `projections.go`**

```go
package store

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// BroadcastFunc receives every projected Workspace row for hub fan-out (03 §2).
type BroadcastFunc func(
	ws domain.Workspace,
)

// registerProjections subscribes to all workspace events, upserts the read model,
// and broadcasts the complete row. One producer ever emits a Workspace (03 §2).
func registerProjections(
	st storage,
	ax asynx.Asynx[domain.Workspace],
	broadcast BroadcastFunc,
) error {
	p := &projector{store: st, broadcast: broadcast}
	if _, err := ax.Subscribe(asynx.Topic("workspace.*"), p.onEvent); err != nil {
		return fmt.Errorf("workspace projection: subscribe: %w", err)
	}
	if _, err := ax.OnForget(p.onForget); err != nil {
		return fmt.Errorf("workspace projection: on forget: %w", err)
	}
	return nil
}

type projector struct {
	store     storage
	broadcast BroadcastFunc
}

func (p *projector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.Workspace],
) {
	if err := p.store.Save(ctx, evt.Aggregate); err != nil {
		slog.ErrorContext(ctx, "workspace projection: save", "id", evt.Aggregate.ID, "err", err)
		return
	}
	p.broadcast(evt.Aggregate)
}

func (p *projector) onForget(
	ctx context.Context,
	evt asynxModels.Event[domain.Workspace],
) {
	if err := p.store.Delete(ctx, evt.Aggregate.ID); err != nil {
		slog.ErrorContext(ctx, "workspace projection: delete", "id", evt.Aggregate.ID, "err", err)
	}
}
```

- [ ] **Step 4: Run to verify pass**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/store/ -run TestProjection`
Expected: PASS.

- [ ] **Step 5: Add the projection-upsert benchmark `store_bench_test.go`**

```go
package store

import (
	"context"
	"testing"
	"time"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func BenchmarkStorage_Save(b *testing.B) {
	db, err := storesqlite.OpenDB(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	st, err := newStorageStore(db)
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	ws := domain.Workspace{ID: "w1", RepoID: "r1", ProjectID: "p1", CreatedAt: time.Unix(1, 0)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := st.Save(ctx, ws); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStorage_FindAll(b *testing.B) {
	db, err := storesqlite.OpenDB(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	st, err := newStorageStore(db)
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		ws := domain.Workspace{ID: string(rune('a'+i%26)) + string(rune('0'+i/26)), RepoID: "r1", ProjectID: "p1"}
		_ = st.Save(ctx, ws)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := st.FindAll(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
```

- [ ] **Step 6: Run the benchmark to confirm it executes**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/store/ -bench . -benchtime 10x -run '^$'`
Expected: benchmarks run, no failures.

- [ ] **Step 7: Commit**

```bash
git add api/internal/app/repositories/workspace/internal/store/
git commit -m "feat(workspace): read-model projection + broadcast + bench"
```

### Task 9: Workspace store facade + wire `List`/`Get` into the repository

**Files:**
- Create: `internal/app/repositories/workspace/internal/store/store.go`
- Test: `internal/app/repositories/workspace/internal/store/store_test.go`
- Modify: `internal/app/repositories/workspace/workspace.go`

- [ ] **Step 1: Write the failing test for the store facade**

```go
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	wscmds "github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestStore_ListReflectsProjection(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	st, err := store.New(db, ax, func(domain.Workspace) {})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ax.SendWait(ctx, wscmds.CreateWorkspace{ID: "w1", RepoID: "r1", ProjectID: "p1", Now: time.Unix(1, 0).UTC()})
	require.NoError(t, err)

	all, err := st.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/store/ -run TestStore_List`
Expected: FAIL — `undefined: store.New`.

- [ ] **Step 3: Implement `store.go`**

```go
package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Store is the workspace read model: a projected, queryable view of the aggregate.
type Store interface {
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	Get(
		ctx context.Context,
		id string,
	) (*domain.Workspace, error)
}

type storeService struct {
	storage storage
}

// New builds the read-model store, registering the projection that keeps it in
// sync with the aggregate and fans every row out through broadcast.
func New(
	db *gormdb.DB,
	ax asynx.Asynx[domain.Workspace],
	broadcast BroadcastFunc,
) (Store, error) {
	st, err := newStorageStore(db)
	if err != nil {
		return nil, fmt.Errorf("workspace store: %w", err)
	}
	if err := registerProjections(st, ax, broadcast); err != nil {
		return nil, fmt.Errorf("workspace store: projections: %w", err)
	}
	return &storeService{storage: st}, nil
}

func (s *storeService) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	return s.storage.FindAll(ctx)
}

func (s *storeService) Get(
	ctx context.Context,
	id string,
) (*domain.Workspace, error) {
	return s.storage.FindByKey(ctx, id)
}
```

- [ ] **Step 4: Wire the store into the `Workspace` repository facade**

In `workspace.go`, change the constructor and struct so the repository owns the read-model store. Replace the `New` function and `workspace` struct:

```go
type workspace struct {
	ax    asynx.Asynx[domain.Workspace]
	store store.Store
}

// New builds a Workspace repository over the asynx instance and a GORM DB. The
// broadcast func is the hub fan-out for projected rows (03 §2).
func New(
	ax asynx.Asynx[domain.Workspace],
	db *gormdb.DB,
	broadcast store.BroadcastFunc,
) (Workspace, error) {
	st, err := store.New(db, ax, broadcast)
	if err != nil {
		return nil, fmt.Errorf("workspace: store: %w", err)
	}
	return &workspace{ax: ax, store: st}, nil
}
```

Add imports `gormdb "gorm.io/gorm"` and `"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store"`.

Add the `List` method to the interface and impl:

```go
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
```

```go
func (w *workspace) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	return w.store.List(ctx)
}
```

> `New` now returns `(Workspace, error)`. The repositories container (Task 22) and all `workspace.New(...)` test helpers must be updated. Update `workspace_test.go`'s `newRepo` helper to open an in-memory DB and pass a no-op broadcast:

```go
func newRepo(
	t *testing.T,
) (context.Context, workspace.Workspace) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	repo, err := workspace.New(ax, db, func(domain.Workspace) {})
	require.NoError(t, err)
	return context.Background(), repo
}
```

Add import `storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"` to the test file.

- [ ] **Step 5: Add a repo-level List test (append to `workspace_test.go`)**

```go
func TestWorkspace_List(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1000, 0).UTC()
	_, err := repo.Create(ctx, workspace.CreateInput{ID: "w1", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)
	_, err = repo.Create(ctx, workspace.CreateInput{ID: "w2", RepoID: "r1", ProjectID: "p1"}, now)
	require.NoError(t, err)
	all, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}
```

- [ ] **Step 6: Run all workspace tests**

Run: `cd api && go test ./internal/app/repositories/workspace/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add api/internal/app/repositories/workspace/
git commit -m "feat(workspace): read-model store facade + List on repository"
```

### Task 10: Workspace mocks

**Files:**
- Create: `internal/app/repositories/workspace/internal/mocks/mocks.go`

- [ ] **Step 1: Implement the mock (function-field test double for `workspace.Workspace`)**

```go
package mocks

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// MockWorkspace is a test double for workspace.Workspace.
type MockWorkspace struct {
	CreateFn            func(ctx context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error)
	SyncWorkingFn       func(ctx context.Context, in workspace.SyncInput, now time.Time) (domain.Workspace, error)
	SyncProviderFn      func(ctx context.Context, in workspace.ProviderInput, now time.Time) (domain.Workspace, error)
	SetMergeStrategyFn  func(ctx context.Context, id string, s gitdomain.MergeStrategy) (domain.Workspace, error)
	TouchActivityFn     func(ctx context.Context, id string, now time.Time) (domain.Workspace, error)
	ReparentFn          func(ctx context.Context, id, parentID, forkPointSha string, now time.Time) (domain.Workspace, error)
	UpdateForkPointFn   func(ctx context.Context, id, forkPointSha string) (domain.Workspace, error)
	SetPendingMergeFn   func(ctx context.Context, id string, s gitdomain.MergeStrategy, target string) (domain.Workspace, error)
	ClearPendingMergeFn func(ctx context.Context, id string) (domain.Workspace, error)
	DeleteFn            func(ctx context.Context, id string) error
	GetFn               func(ctx context.Context, id string) (domain.Workspace, error)
	ListFn              func(ctx context.Context) ([]domain.Workspace, error)
}

func (m *MockWorkspace) Create(
	ctx context.Context,
	in workspace.CreateInput,
	now time.Time,
) (domain.Workspace, error) {
	return m.CreateFn(ctx, in, now)
}

func (m *MockWorkspace) SyncWorkingTreeState(
	ctx context.Context,
	in workspace.SyncInput,
	now time.Time,
) (domain.Workspace, error) {
	return m.SyncWorkingFn(ctx, in, now)
}

func (m *MockWorkspace) SyncProviderState(
	ctx context.Context,
	in workspace.ProviderInput,
	now time.Time,
) (domain.Workspace, error) {
	return m.SyncProviderFn(ctx, in, now)
}

func (m *MockWorkspace) SetMergeStrategy(
	ctx context.Context,
	id string,
	s gitdomain.MergeStrategy,
) (domain.Workspace, error) {
	return m.SetMergeStrategyFn(ctx, id, s)
}

func (m *MockWorkspace) TouchActivity(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Workspace, error) {
	return m.TouchActivityFn(ctx, id, now)
}

func (m *MockWorkspace) Reparent(
	ctx context.Context,
	id string,
	parentID string,
	forkPointSha string,
	now time.Time,
) (domain.Workspace, error) {
	return m.ReparentFn(ctx, id, parentID, forkPointSha, now)
}

func (m *MockWorkspace) UpdateForkPoint(
	ctx context.Context,
	id string,
	forkPointSha string,
) (domain.Workspace, error) {
	return m.UpdateForkPointFn(ctx, id, forkPointSha)
}

func (m *MockWorkspace) SetPendingMerge(
	ctx context.Context,
	id string,
	s gitdomain.MergeStrategy,
	target string,
) (domain.Workspace, error) {
	return m.SetPendingMergeFn(ctx, id, s, target)
}

func (m *MockWorkspace) ClearPendingMerge(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	return m.ClearPendingMergeFn(ctx, id)
}

func (m *MockWorkspace) Delete(
	ctx context.Context,
	id string,
) error {
	return m.DeleteFn(ctx, id)
}

func (m *MockWorkspace) Get(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	return m.GetFn(ctx, id)
}

func (m *MockWorkspace) List(
	ctx context.Context,
) ([]domain.Workspace, error) {
	return m.ListFn(ctx)
}

var _ workspace.Workspace = (*MockWorkspace)(nil)
```

- [ ] **Step 2: Build to verify the mock satisfies the interface**

Run: `cd api && go build ./internal/app/repositories/workspace/...`
Expected: success (the `var _ workspace.Workspace` assertion compiles).

- [ ] **Step 3: Commit**

```bash
git add api/internal/app/repositories/workspace/internal/mocks/
git commit -m "test(workspace): repository mock"
```

---

## Phase 3 — Chat aggregate command set + read model

### Task 11: Chat lifecycle commands — extend `create`, add `fork`/`rename`/`delete`

**Files:**
- Modify: `internal/app/repositories/chat/internal/commands/create.go`
- Create: `internal/app/repositories/chat/internal/commands/fork.go`
- Create: `internal/app/repositories/chat/internal/commands/rename.go`
- Create: `internal/app/repositories/chat/internal/commands/delete.go`
- Test: append to `internal/app/repositories/chat/internal/commands/commands_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestCreateChat_SeedsTitleAndType(t *testing.T) {
	now := time.Unix(1, 0)
	cmd := CreateChat{ID: "c1", WsID: "w1", Title: "Hello", Now: now}
	c := cmd.EmitEvent(nil)
	assert.Equal(t, "Hello", c.Title)
	assert.Equal(t, domain.ChatTypeChat, c.Type)
	assert.Equal(t, domain.ChatStatusIdle, c.Status)
}

func TestForkChat_CopiesParentTitleWithParentID(t *testing.T) {
	now := time.Unix(2, 0)
	cmd := ForkChat{ID: "c2", WsID: "w1", ParentID: "c1", Title: "Hello (fork)", Now: now}
	c := cmd.EmitEvent(nil)
	assert.Equal(t, "c1", c.ParentID)
	assert.Equal(t, "Hello (fork)", c.Title)
	assert.Equal(t, domain.ChatStatusIdle, c.Status)
}

func TestForkChat_Validate_RejectsExistingAndMissing(t *testing.T) {
	assert.True(t, errors.Is(ForkChat{ID: "c2", WsID: "w1", ParentID: "c1"}.Validate(&domain.Chat{ID: "c2"}), asynxModels.ErrValidation))
	assert.True(t, errors.Is(ForkChat{ID: "c2"}.Validate(nil), asynxModels.ErrValidation))
}

func TestRenameChat_UpdatesTitle(t *testing.T) {
	cur := &domain.Chat{ID: "c1", Title: "old"}
	c := RenameChat{ID: "c1", Title: "new"}.EmitEvent(cur)
	assert.Equal(t, "new", c.Title)
}

func TestRenameChat_Validate_RejectsMissing(t *testing.T) {
	assert.True(t, errors.Is(RenameChat{ID: "c1"}.Validate(nil), asynxModels.ErrValidation))
}

func TestDeleteChat_SetsDeletedAt(t *testing.T) {
	now := time.Unix(5, 0)
	cur := &domain.Chat{ID: "c1", Status: domain.ChatStatusIdle}
	c := DeleteChat{ID: "c1", Now: now}.EmitEvent(cur)
	require.NotNil(t, c.DeletedAt)
	assert.Equal(t, now, *c.DeletedAt)
}

func TestDeleteChat_Idempotent(t *testing.T) {
	first := time.Unix(5, 0)
	cur := &domain.Chat{ID: "c1", DeletedAt: &first}
	c := DeleteChat{ID: "c1", Now: time.Unix(9, 0)}.EmitEvent(cur)
	require.NotNil(t, c.DeletedAt)
	assert.Equal(t, first, *c.DeletedAt)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd api && go test ./internal/app/repositories/chat/internal/commands/`
Expected: FAIL — undefined / wrong shape.

- [ ] **Step 3: Update `create.go`**

```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// CreateChat creates a root Chat aggregate in the idle state (01 §3).
type CreateChat struct {
	ID    string
	WsID  string
	Title string
	Now   time.Time
}

func (c CreateChat) AggregateID() string {
	return c.ID
}

func (c CreateChat) EventName() string {
	return "chat.created." + c.ID
}

func (c CreateChat) ShouldSnapshot() bool {
	return false
}

func (c CreateChat) Validate(
	current *domain.Chat,
) error {
	if current != nil {
		return fmt.Errorf("create chat: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.WsID == "" {
		return fmt.Errorf("create chat: missing ids: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c CreateChat) EmitEvent(
	_ *domain.Chat,
) domain.Chat {
	return domain.Chat{
		ID:        c.ID,
		WsID:      c.WsID,
		Title:     c.Title,
		Status:    domain.ChatStatusIdle,
		Type:      domain.ChatTypeChat,
		CreatedAt: c.Now,
	}
}
```

- [ ] **Step 4: Implement `fork.go`**

```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ForkChat creates a child Chat copying the parent as it currently stands, with
// parentId set (01 §4). v0 forks from the current tip; no fromTurnId.
type ForkChat struct {
	ID       string
	WsID     string
	ParentID string
	Title    string
	Now      time.Time
}

func (c ForkChat) AggregateID() string {
	return c.ID
}

func (c ForkChat) EventName() string {
	return "chat.forked." + c.ID
}

func (c ForkChat) ShouldSnapshot() bool {
	return false
}

func (c ForkChat) Validate(
	current *domain.Chat,
) error {
	if current != nil {
		return fmt.Errorf("fork chat: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.WsID == "" || c.ParentID == "" {
		return fmt.Errorf("fork chat: missing ids: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ForkChat) EmitEvent(
	_ *domain.Chat,
) domain.Chat {
	return domain.Chat{
		ID:        c.ID,
		WsID:      c.WsID,
		ParentID:  c.ParentID,
		Title:     c.Title,
		Status:    domain.ChatStatusIdle,
		Type:      domain.ChatTypeChat,
		CreatedAt: c.Now,
	}
}
```

- [ ] **Step 5: Implement `rename.go`**

```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// RenameChat updates a chat's title (01 §3).
type RenameChat struct {
	ID    string
	Title string
}

func (c RenameChat) AggregateID() string {
	return c.ID
}

func (c RenameChat) EventName() string {
	return "chat.renamed." + c.ID
}

func (c RenameChat) ShouldSnapshot() bool {
	return false
}

func (c RenameChat) Validate(
	current *domain.Chat,
) error {
	if current == nil {
		return fmt.Errorf("rename chat: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c RenameChat) EmitEvent(
	current *domain.Chat,
) domain.Chat {
	chat := *current
	chat.Title = c.Title
	return chat
}
```

- [ ] **Step 6: Implement `delete.go`**

```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// DeleteChat soft-deletes a chat (sets deletedAt). Idempotent: re-issuing on an
// already-deleted chat is a no-op, so cascades replay safely (01 §8).
type DeleteChat struct {
	ID  string
	Now time.Time
}

func (c DeleteChat) AggregateID() string {
	return c.ID
}

func (c DeleteChat) EventName() string {
	return "chat.deleted." + c.ID
}

func (c DeleteChat) ShouldSnapshot() bool {
	return false
}

func (c DeleteChat) Validate(
	current *domain.Chat,
) error {
	if current == nil {
		return fmt.Errorf("delete chat: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c DeleteChat) EmitEvent(
	current *domain.Chat,
) domain.Chat {
	chat := *current
	if chat.DeletedAt != nil {
		return chat
	}
	deletedAt := c.Now
	chat.DeletedAt = &deletedAt
	return chat
}
```

- [ ] **Step 7: Run to verify pass**

Run: `cd api && go test ./internal/app/repositories/chat/internal/commands/`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add api/internal/app/repositories/chat/internal/commands/
git commit -m "feat(chat): fork/rename/delete commands + title/type on create (01)"
```

### Task 12: Chat read model (storage + projection + store), mirroring Tasks 7-9

**Files:**
- Create: `internal/app/repositories/chat/internal/store/{storage.go,projections.go,store.go,store_bench_test.go}` + `storage_test.go` + `projections_test.go` + `store_test.go`

> Follow the exact same structure as the Workspace store (Tasks 7-9). Differences: row table `read_chats`, the broadcast func emits a `ChatStatusEvent`-shaped projection (the projection broadcasts `domain.Chat`; the hub mapping to `ChatStatusEvent` happens in `RegisterHubProjections`, Task 23 — the store-layer `BroadcastFunc` takes `domain.Chat`). The store exposes `List(ctx)` and `ListByWorkspace(ctx, wsID)` filtering out soft-deleted chats? **No** — the read model keeps soft-deleted rows (01 §8: parent row remains queryable after soft-delete for cascade). `ListByWorkspace` returns all non-deleted chats for a wsID; add `Get` returning the row even if deleted.

- [ ] **Step 1: Implement `storage.go`** (identical shape to workspace storage, swapping `domain.Chat`, table `read_chats`, type names `chatRow`/`storage`/`storageStore`/`newStorageStore`/`unmarshalChat`). Full code:

```go
package store

import (
	"context"
	"encoding/json"
	"fmt"

	gormdb "gorm.io/gorm"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type chatRow struct {
	ID   string `gorm:"primaryKey;column:id"`
	Data []byte `gorm:"column:data"`
}

func (chatRow) TableName() string {
	return "read_chats"
}

type storage interface {
	Save(
		ctx context.Context,
		c domain.Chat,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Chat, error)
	FindAll(
		ctx context.Context,
	) ([]domain.Chat, error)
}

type storageStore struct {
	inner interface {
		Save(ctx context.Context, row chatRow) error
		Delete(ctx context.Context, key string) error
		FindByKey(ctx context.Context, key string) (*chatRow, error)
		FindAll(ctx context.Context) ([]chatRow, error)
	}
}

func newStorageStore(
	db *gormdb.DB,
) (storage, error) {
	inner, err := storesqlite.NewFromDB[chatRow, string](db)
	if err != nil {
		return nil, fmt.Errorf("chat storage: %w", err)
	}
	return &storageStore{inner: inner}, nil
}

func (s *storageStore) Save(
	ctx context.Context,
	c domain.Chat,
) error {
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("chat storage: marshal: %w", err)
	}
	return s.inner.Save(ctx, chatRow{ID: c.ID, Data: data})
}

func (s *storageStore) Delete(
	ctx context.Context,
	id string,
) error {
	return s.inner.Delete(ctx, id)
}

func (s *storageStore) FindByKey(
	ctx context.Context,
	id string,
) (*domain.Chat, error) {
	row, err := s.inner.FindByKey(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("chat storage: find: %w", err)
	}
	if row == nil {
		return nil, nil
	}
	return unmarshalChat(row.Data)
}

func (s *storageStore) FindAll(
	ctx context.Context,
) ([]domain.Chat, error) {
	rows, err := s.inner.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("chat storage: find all: %w", err)
	}
	result := make([]domain.Chat, 0, len(rows))
	for _, row := range rows {
		c, err := unmarshalChat(row.Data)
		if err != nil {
			return nil, err
		}
		result = append(result, *c)
	}
	return result, nil
}

func unmarshalChat(
	data []byte,
) (*domain.Chat, error) {
	var c domain.Chat
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("chat storage: unmarshal: %w", err)
	}
	return &c, nil
}
```

`storage_test.go` mirrors `TestStorage_SaveFindDelete` / `_MissingReturnsNil` from Task 7 using `domain.Chat`.

- [ ] **Step 2: Implement `projections.go`** — subscribe to `chat.*`, save + broadcast `domain.Chat`; `OnForget` deletes. Full code:

```go
package store

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// BroadcastFunc receives every projected Chat for hub fan-out (01 §5).
type BroadcastFunc func(
	c domain.Chat,
)

func registerProjections(
	st storage,
	ax asynx.Asynx[domain.Chat],
	broadcast BroadcastFunc,
) error {
	p := &projector{store: st, broadcast: broadcast}
	if _, err := ax.Subscribe(asynx.Topic("chat.*"), p.onEvent); err != nil {
		return fmt.Errorf("chat projection: subscribe: %w", err)
	}
	if _, err := ax.OnForget(p.onForget); err != nil {
		return fmt.Errorf("chat projection: on forget: %w", err)
	}
	return nil
}

type projector struct {
	store     storage
	broadcast BroadcastFunc
}

func (p *projector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.Chat],
) {
	if err := p.store.Save(ctx, evt.Aggregate); err != nil {
		slog.ErrorContext(ctx, "chat projection: save", "id", evt.Aggregate.ID, "err", err)
		return
	}
	p.broadcast(evt.Aggregate)
}

func (p *projector) onForget(
	ctx context.Context,
	evt asynxModels.Event[domain.Chat],
) {
	if err := p.store.Delete(ctx, evt.Aggregate.ID); err != nil {
		slog.ErrorContext(ctx, "chat projection: delete", "id", evt.Aggregate.ID, "err", err)
	}
}
```

`projections_test.go` mirrors Task 8 using `chatcmds.CreateChat`.

- [ ] **Step 3: Implement `store.go`** with `List`, `ListByWorkspace`, `Get`:

```go
package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Store is the chat read model: a projected, queryable view of the aggregate.
type Store interface {
	List(
		ctx context.Context,
	) ([]domain.Chat, error)
	ListByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.Chat, error)
	Get(
		ctx context.Context,
		id string,
	) (*domain.Chat, error)
}

type storeService struct {
	storage storage
}

// New builds the chat read-model store and registers its projection.
func New(
	db *gormdb.DB,
	ax asynx.Asynx[domain.Chat],
	broadcast BroadcastFunc,
) (Store, error) {
	st, err := newStorageStore(db)
	if err != nil {
		return nil, fmt.Errorf("chat store: %w", err)
	}
	if err := registerProjections(st, ax, broadcast); err != nil {
		return nil, fmt.Errorf("chat store: projections: %w", err)
	}
	return &storeService{storage: st}, nil
}

func (s *storeService) List(
	ctx context.Context,
) ([]domain.Chat, error) {
	return s.storage.FindAll(ctx)
}

func (s *storeService) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.Chat, error) {
	all, err := s.storage.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	return filterByWorkspace(all, wsID), nil
}

func (s *storeService) Get(
	ctx context.Context,
	id string,
) (*domain.Chat, error) {
	return s.storage.FindByKey(ctx, id)
}

func filterByWorkspace(
	all []domain.Chat,
	wsID string,
) []domain.Chat {
	result := make([]domain.Chat, 0, len(all))
	for _, c := range all {
		if c.WsID == wsID && c.DeletedAt == nil {
			result = append(result, c)
		}
	}
	return result
}
```

`store_test.go` verifies `ListByWorkspace` filters by wsID and excludes soft-deleted. `store_bench_test.go` mirrors Task 8's benchmarks with `domain.Chat`.

- [ ] **Step 4: Run all chat-store tests + bench**

Run: `cd api && go test ./internal/app/repositories/chat/internal/store/... && go test ./internal/app/repositories/chat/internal/store/ -bench . -benchtime 10x -run '^$'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/repositories/chat/internal/store/
git commit -m "feat(chat): GORM read model (storage/projection/store) + bench"
```

### Task 13: Wire Chat repository facade — `Fork`/`Rename`/`Delete`/`List`/`ListByWorkspace` + store

**Files:**
- Modify: `internal/app/repositories/chat/chat.go`
- Create: `internal/app/repositories/chat/internal/mocks/mocks.go`
- Test: modify `internal/app/repositories/chat/chat_test.go`

- [ ] **Step 1: Write failing tests** mirroring the workspace pattern: `newRepo` opens in-memory ES + DB and calls `chat.New(ax, db, func(domain.Chat){})`. Tests: `Create` sets title; `Fork` sets parentId; `Rename`; `Delete` sets deletedAt and is idempotent on re-call; `ListByWorkspace` returns only that ws's non-deleted chats. Example:

```go
func TestChat_ForkRenameDeleteList(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	_, err := repo.Create(ctx, "c1", "w1", "root", now)
	require.NoError(t, err)
	forked, err := repo.Fork(ctx, "c2", "w1", "c1", "root (fork)", now)
	require.NoError(t, err)
	assert.Equal(t, "c1", forked.ParentID)
	renamed, err := repo.Rename(ctx, "c1", "renamed")
	require.NoError(t, err)
	assert.Equal(t, "renamed", renamed.Title)
	list, err := repo.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	_, err = repo.Delete(ctx, "c2", now)
	require.NoError(t, err)
	list, err = repo.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	assert.Len(t, list, 1)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd api && go test ./internal/app/repositories/chat/ -run TestChat_ForkRenameDeleteList`
Expected: FAIL.

- [ ] **Step 3: Update `chat.go`** — change `Create` signature to accept `title`; add `Fork`, `Rename`, `Delete`, `List`, `ListByWorkspace`; change `New` to `(ax, db, broadcast) (Chat, error)` building the store. Interface additions (one param per line); impl wraps `SendWait` with `fmt.Errorf("chat: <op>: %w", err)`. For `Delete`, call `SendWait(ctx, commands.DeleteChat{ID:id, Now:now})`. `Create`:

```go
func (c *chat) Create(
	ctx context.Context,
	id string,
	wsID string,
	title string,
	now time.Time,
) (domain.Chat, error) {
	evt, err := c.ax.SendWait(ctx, commands.CreateChat{ID: id, WsID: wsID, Title: title, Now: now})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: create: %w", err)
	}
	return evt.Aggregate, nil
}
```

Add `Fork`, `Rename`, `Delete`, `List`, `ListByWorkspace` following the same wrapping pattern, delegating `List`/`ListByWorkspace`/store-`Get` to the embedded `store.Store`. Keep `Get` reading from Asynx (`c.ax.Get`) — recovery needs the live aggregate. Build the store in `New`:

```go
func New(
	ax asynx.Asynx[domain.Chat],
	db *gormdb.DB,
	broadcast store.BroadcastFunc,
) (Chat, error) {
	st, err := store.New(db, ax, broadcast)
	if err != nil {
		return nil, fmt.Errorf("chat: store: %w", err)
	}
	return &chat{ax: ax, store: st}, nil
}
```

- [ ] **Step 4: Implement `internal/mocks/mocks.go`** for `chat.Chat` (function-field double, `var _ chat.Chat = (*MockChat)(nil)`), covering every interface method.

- [ ] **Step 5: Run all chat tests**

Run: `cd api && go test ./internal/app/repositories/chat/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/app/repositories/chat/
git commit -m "feat(chat): repository facade fork/rename/delete/list + store + mock"
```

---

## Phase 4 — AgentRun read model + ReviewThread completion

### Task 14: AgentRun read model + `List`/`ListRunning`

**Files:**
- Create: `internal/app/repositories/agentrun/internal/store/{storage.go,projections.go,store.go,store_bench_test.go}` + tests
- Modify: `internal/app/repositories/agentrun/agentrun.go`
- Create: `internal/app/repositories/agentrun/internal/mocks/mocks.go`

- [ ] **Step 1: Implement the store** mirroring Task 12 with `domain.AgentRun`, table `read_agent_runs`. `store.go` exposes `List(ctx)`, `ListRunning(ctx)` (filter `Status == domain.AgentRunStatusRunning`), `ListByChat(ctx, chatID)`, `Get(ctx, id)`. The `BroadcastFunc` takes `domain.AgentRun` (AgentRun has no direct broadcaster, but the projection still drives Chat/Workspace overlays via the subscription wired in Task 23 — so here `New` accepts a no-op-capable broadcast; keep the param for symmetry). Write `storage_test.go`, `projections_test.go`, `store_test.go`, `store_bench_test.go`.

- [ ] **Step 2: Write failing repo test** — `ListRunning` returns only running runs:

```go
func TestAgentRun_ListRunning(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	_, err := repo.Create(ctx, "a1", "w1", "c1", now)
	require.NoError(t, err)
	_, err = repo.MarkRunning(ctx, "a1")
	require.NoError(t, err)
	_, err = repo.Create(ctx, "a2", "w1", "c2", now)
	require.NoError(t, err)
	running, err := repo.ListRunning(ctx)
	require.NoError(t, err)
	require.Len(t, running, 1)
	assert.Equal(t, "a1", running[0].ID)
}
```

- [ ] **Step 3: Run to verify failure**, then update `agentrun.go` — `New(ax, db, broadcast) (AgentRun, error)` building the store; add `List`, `ListRunning`, `ListByChat` delegating to the store. Run to verify pass.

- [ ] **Step 4: Implement mocks.go** for `agentrun.AgentRun`.

- [ ] **Step 5: Run + commit**

Run: `cd api && go test ./internal/app/repositories/agentrun/...`
```bash
git add api/internal/app/repositories/agentrun/
git commit -m "feat(agentrun): GORM read model + List/ListRunning + mock"
```

### Task 15: Complete ReviewThread — `OpenThread` full payload + `ReplyThread`

**Files:**
- Modify: `internal/app/repositories/reviewthread/internal/commands/open.go`
- Create: `internal/app/repositories/reviewthread/internal/commands/reply.go`
- Test: append to `internal/app/repositories/reviewthread/internal/commands/commands_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestOpenReviewThread_SeedsAnchorAndFirstMessage(t *testing.T) {
	now := time.Unix(1, 0)
	cmd := OpenReviewThread{
		ID: "t1", WsID: "w1", FilePath: "a.go", LineNumber: 12,
		Side: domain.ReviewSideRight, MessageID: "m1", Body: "hi", Now: now,
	}
	th := cmd.EmitEvent(nil)
	assert.Equal(t, "a.go", th.FilePath)
	assert.Equal(t, 12, th.LineNumber)
	assert.Equal(t, domain.ReviewSideRight, th.Side)
	require.Len(t, th.Messages, 1)
	assert.Equal(t, "hi", th.Messages[0].Body)
	assert.Equal(t, domain.ReviewThreadStatusOpen, th.Status)
}

func TestReplyReviewThread_AppendsMessage(t *testing.T) {
	now := time.Unix(2, 0)
	cur := &domain.ReviewThread{
		ID: "t1", Status: domain.ReviewThreadStatusOpen,
		Messages: []domain.ReviewMessage{{ID: "m1", Body: "first"}},
	}
	th := ReplyReviewThread{ID: "t1", MessageID: "m2", Body: "second", Now: now}.EmitEvent(cur)
	require.Len(t, th.Messages, 2)
	assert.Equal(t, "second", th.Messages[1].Body)
}

func TestReplyReviewThread_Validate_RejectsMissing(t *testing.T) {
	assert.True(t, errors.Is(ReplyReviewThread{ID: "t1"}.Validate(nil), asynxModels.ErrValidation))
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd api && go test ./internal/app/repositories/reviewthread/internal/commands/`
Expected: FAIL.

- [ ] **Step 3: Update `open.go`** to carry the full payload and seed the first message:

```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// OpenReviewThread creates a ReviewThread anchored to a diff line, with its first
// message (09 §3).
type OpenReviewThread struct {
	ID         string
	WsID       string
	FilePath   string
	LineNumber int
	Side       domain.ReviewSide
	MessageID  string
	Author     string
	IsAgent    bool
	Body       string
	Now        time.Time
}

func (c OpenReviewThread) AggregateID() string {
	return c.ID
}

func (c OpenReviewThread) EventName() string {
	return "review_thread.opened." + c.ID
}

func (c OpenReviewThread) ShouldSnapshot() bool {
	return false
}

func (c OpenReviewThread) Validate(
	current *domain.ReviewThread,
) error {
	if current != nil {
		return fmt.Errorf("open review thread: %w", asynxModels.ErrValidation)
	}
	if c.ID == "" || c.WsID == "" || c.MessageID == "" {
		return fmt.Errorf("open review thread: missing ids: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c OpenReviewThread) EmitEvent(
	_ *domain.ReviewThread,
) domain.ReviewThread {
	return domain.ReviewThread{
		ID:         c.ID,
		WsID:       c.WsID,
		FilePath:   c.FilePath,
		LineNumber: c.LineNumber,
		Side:       c.Side,
		Status:     domain.ReviewThreadStatusOpen,
		Messages: []domain.ReviewMessage{{
			ID:        c.MessageID,
			Author:    c.Author,
			IsAgent:   c.IsAgent,
			Body:      c.Body,
			CreatedAt: c.Now,
		}},
		CreatedAt: c.Now,
	}
}
```

- [ ] **Step 4: Implement `reply.go`**

```go
package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ReplyReviewThread appends a message to an existing thread (09 §3).
type ReplyReviewThread struct {
	ID        string
	MessageID string
	Author    string
	IsAgent   bool
	Body      string
	Now       time.Time
}

func (c ReplyReviewThread) AggregateID() string {
	return c.ID
}

func (c ReplyReviewThread) EventName() string {
	return "review_thread.replied." + c.ID
}

func (c ReplyReviewThread) ShouldSnapshot() bool {
	return false
}

func (c ReplyReviewThread) Validate(
	current *domain.ReviewThread,
) error {
	if current == nil {
		return fmt.Errorf("reply review thread: %w", asynxModels.ErrValidation)
	}
	if c.MessageID == "" {
		return fmt.Errorf("reply review thread: missing message id: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ReplyReviewThread) EmitEvent(
	current *domain.ReviewThread,
) domain.ReviewThread {
	thread := *current
	thread.Messages = append(thread.Messages, domain.ReviewMessage{
		ID:        c.MessageID,
		Author:    c.Author,
		IsAgent:   c.IsAgent,
		Body:      c.Body,
		CreatedAt: c.Now,
	})
	return thread
}
```

- [ ] **Step 5: Run to verify pass**

Run: `cd api && go test ./internal/app/repositories/reviewthread/internal/commands/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/app/repositories/reviewthread/internal/commands/
git commit -m "feat(reviewthread): full OpenThread payload + ReplyThread (09 §3)"
```

### Task 16: ReviewThread read model + facade `Reply`/`Open`/`List`/`ListByWorkspace` + mock

**Files:**
- Create: `internal/app/repositories/reviewthread/internal/store/{storage.go,projections.go,store.go,store_bench_test.go}` + tests
- Modify: `internal/app/repositories/reviewthread/reviewthread.go`
- Create: `internal/app/repositories/reviewthread/internal/mocks/mocks.go`

- [ ] **Step 1: Implement the store** mirroring Task 12 with `domain.ReviewThread`, table `read_review_threads`. `store.go` exposes `List(ctx)`, `ListByWorkspace(ctx, wsID)`, `Get(ctx, id)`. Tests + bench.

- [ ] **Step 2: Update `reviewthread.go`** — extend the `Open` signature to the full anchor payload, add `Reply`, `List`, `ListByWorkspace`; `New(ax, db, broadcast) (ReviewThread, error)` building the store. New `Open` + `Reply` (wrap `SendWait`):

```go
// OpenInput carries the anchor + first message for a new thread (09 §3).
type OpenInput struct {
	ID         string
	WsID       string
	FilePath   string
	LineNumber int
	Side       domain.ReviewSide
	MessageID  string
	Body       string
}
```

Add interface methods `Open(ctx, OpenInput, now)`, `Reply(ctx, id, messageID, body, now)`, `List(ctx)`, `ListByWorkspace(ctx, wsID)`; keep `Resolve`/`Reopen`/`Get`. Write the failing test first (open → reply → list reflects two messages), run red, implement, run green.

- [ ] **Step 3: Implement mocks.go** for `reviewthread.ReviewThread`.

- [ ] **Step 4: Run + commit**

Run: `cd api && go test ./internal/app/repositories/reviewthread/...`
```bash
git add api/internal/app/repositories/reviewthread/
git commit -m "feat(reviewthread): read model + Open/Reply/List facade + mock"
```

---

## Phase 5 — Repositories container, hub projections, crash recovery

### Task 17: AgentRun → Chat + Workspace overlay projection

**Files:**
- Create: `internal/app/repositories/agent_run_projection.go`
- Test: `internal/app/repositories/agent_run_projection_test.go`

> When an AgentRun transitions, the Chat aggregate must reflect `agent-running`/`idle` (01 §5) and the Workspace `agent-running` overlay is derived from `hasLiveAgentRun(ws)` (00 §6.1). The overlay is **not** a stored Workspace field — it is computed at broadcast/snapshot time. So this projection: on AgentRun `running` → `chat.SetAgentRunning(chatID)`; on `done|error|interrupted` → `chat.ResetIdle(chatID)`; and in both cases re-broadcast the workspace row so its derived overlay refreshes (the overlay predicate lives in the broadcaster snapshot — Wave-3 broadcaster work; here we re-emit the current Workspace via `workspace.TouchActivity` is wrong because it bumps time. Instead, re-broadcast by calling a `refreshWorkspace(wsID)` hook that reads the workspace row and pushes it through the hub unchanged).

- [ ] **Step 1: Write the failing test** — a fake chat repo records `SetAgentRunning`/`ResetIdle` calls; drive AgentRun events through a real Asynx instance with the projection registered; assert the chat repo saw the right calls. Use a condition-based wait helper (poll a mutex-guarded slice until len≥1 with a deadline; **no `time.Sleep`** — use `require.Eventually` from testify, which polls without fixed sleeps).

```go
func TestAgentRunProjection_DrivesChatStatus(t *testing.T) {
	// ... build real AgentRun asynx, fake chat repo capturing calls,
	// register RegisterAgentRunProjection(ax, fakeChat, refreshFn)
	// send Create + MarkRunning -> expect SetAgentRunning("c1")
	// send Complete -> expect ResetIdle("c1")
	require.Eventually(t, func() bool { return fakeChat.running("c1") }, time.Second, 5*time.Millisecond)
}
```

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement `agent_run_projection.go`**

```go
package repositories

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// RefreshWorkspaceFunc re-broadcasts a workspace row so its derived agent-running
// overlay refreshes after an AgentRun transition (00 §6.1).
type RefreshWorkspaceFunc func(
	ctx context.Context,
	wsID string,
)

// RegisterAgentRunProjection drives Chat status from AgentRun lifecycle (01 §5)
// and refreshes the owning workspace's overlay.
func RegisterAgentRunProjection(
	ax asynx.Asynx[domain.AgentRun],
	chats chat.Chat,
	refresh RefreshWorkspaceFunc,
) error {
	p := &agentRunProjector{chats: chats, refresh: refresh}
	if _, err := ax.Subscribe(asynx.Topic("agent_run.*"), p.onEvent); err != nil {
		return fmt.Errorf("agent run projection: subscribe: %w", err)
	}
	return nil
}

type agentRunProjector struct {
	chats   chat.Chat
	refresh RefreshWorkspaceFunc
}

func (p *agentRunProjector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.AgentRun],
) {
	p.applyChatStatus(ctx, evt.Aggregate)
	p.refresh(ctx, evt.Aggregate.WsID)
}

func (p *agentRunProjector) applyChatStatus(
	ctx context.Context,
	run domain.AgentRun,
) {
	if run.Status == domain.AgentRunStatusRunning {
		if _, err := p.chats.SetAgentRunning(ctx, run.ChatID); err != nil {
			slog.ErrorContext(ctx, "agent run projection: set running", "chat", run.ChatID, "err", err)
		}
		return
	}
	if isTerminal(run.Status) {
		if _, err := p.chats.ResetIdle(ctx, run.ChatID); err != nil {
			slog.ErrorContext(ctx, "agent run projection: reset idle", "chat", run.ChatID, "err", err)
		}
	}
}

func isTerminal(
	status domain.AgentRunStatus,
) bool {
	return status == domain.AgentRunStatusDone ||
		status == domain.AgentRunStatusError ||
		status == domain.AgentRunStatusInterrupted
}
```

> Verify the exact `AgentRunStatus` constant names in `internal/domain/agent_run_status.go` and use them; adjust `isTerminal` if names differ.

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/repositories/agent_run_projection.go api/internal/app/repositories/agent_run_projection_test.go
git commit -m "feat(app): AgentRun → Chat status + workspace overlay projection (01 §5)"
```

### Task 18: Real crash-recovery enumeration from read models

**Files:**
- Modify: `internal/app/repositories/recovery.go`
- Test: modify `internal/app/repositories/recovery_test.go`

- [ ] **Step 1: Write failing tests** — `RecoverAgentRuns` now takes the AgentRun repo and enumerates `ListRunning()`; `ReconcileChats` enumerates chats in `agent-running` with no live run. Drive with real Asynx + read models: create a run, MarkRunning, then a *fresh* recovery pass flips it to error and the chat to idle. Assert via `ListRunning()` empty and chat status idle.

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Update `recovery.go`** — replace the `candidateIDs []string` parameters with enumeration:

```go
// RecoverAgentRuns drives every read-model AgentRun still in running to error,
// using SendWait so recovery events drain before the caller proceeds (00 §6.2).
func RecoverAgentRuns(
	ctx context.Context,
	runs agentrun.AgentRun,
) {
	running, err := runs.ListRunning(ctx)
	if err != nil {
		slog.WarnContext(ctx, "crash recovery: list running runs", "err", err)
		return
	}
	for _, run := range running {
		recoverOneRun(ctx, run.ID, runs)
	}
}

// ReconcileChats resets every chat stuck in agent-running with no live run back to
// idle. ResetIdle is idempotent, so it is safe after AgentRun recovery (00 §6.2).
func ReconcileChats(
	ctx context.Context,
	chats chat.Chat,
	runs agentrun.AgentRun,
) {
	stuck, err := chats.List(ctx)
	if err != nil {
		slog.WarnContext(ctx, "crash recovery: list chats", "err", err)
		return
	}
	live := liveChatSet(ctx, runs)
	for _, c := range stuck {
		reconcileOneChat(ctx, c.ID, func(id string) bool { return live[id] }, chats)
	}
}

func liveChatSet(
	ctx context.Context,
	runs agentrun.AgentRun,
) map[string]bool {
	live := map[string]bool{}
	running, err := runs.ListRunning(ctx)
	if err != nil {
		slog.WarnContext(ctx, "crash recovery: live set", "err", err)
		return live
	}
	for _, run := range running {
		live[run.ChatID] = true
	}
	return live
}
```

Keep `recoverOneRun`/`reconcileOneChat` as-is.

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/repositories/recovery.go api/internal/app/repositories/recovery_test.go
git commit -m "feat(app): crash recovery enumerates from read models (00 §6.2)"
```

### Task 19: Rebuild the repositories container — construct read-model stores, wire projections, real recovery

**Files:**
- Modify: `internal/app/repositories/container.go`
- Test: modify `internal/app/repositories/container_test.go`

- [ ] **Step 1: Write the failing test** — `New(...)` now needs the GORM DB and the hub; building it registers projections so that a `CreateWorkspace` ends up in `Workspace.List`. Assert end-to-end: build container, create a workspace via the repo, `Workspace.List` returns it, and the hub captured a broadcast.

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Rewrite `container.go`**

```go
package repositories

import (
	"context"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Container holds the four aggregate repositories (each owning its read model).
type Container struct {
	Workspace    workspace.Workspace
	Chat         chat.Chat
	AgentRun     agentrun.AgentRun
	ReviewThread reviewthread.ReviewThread
	hub          hub.WebSocketHub
}

// New builds all aggregate repositories, wiring each projection's broadcast into
// the hub. Read models live in the shared GORM DB.
func New(
	db *gormdb.DB,
	h hub.WebSocketHub,
	axWorkspace asynx.Asynx[domain.Workspace],
	axChat asynx.Asynx[domain.Chat],
	axAgentRun asynx.Asynx[domain.AgentRun],
	axReviewThread asynx.Asynx[domain.ReviewThread],
) (*Container, error) {
	ws, err := workspace.New(axWorkspace, db, h.BroadcastWorkspace)
	if err != nil {
		return nil, err
	}
	ch, err := chat.New(axChat, db, broadcastChat(h))
	if err != nil {
		return nil, err
	}
	ar, err := agentrun.New(axAgentRun, db, func(domain.AgentRun) {})
	if err != nil {
		return nil, err
	}
	rt, err := reviewthread.New(axReviewThread, db, func(domain.ReviewThread) {})
	if err != nil {
		return nil, err
	}
	return &Container{Workspace: ws, Chat: ch, AgentRun: ar, ReviewThread: rt, hub: h}, nil
}

func broadcastChat(
	h hub.WebSocketHub,
) chat.BroadcastFunc {
	return func(c domain.Chat) {
		h.BroadcastChat(hub.ChatStatusEvent{ChatID: c.ID, WsID: c.WsID, Status: c.Status})
	}
}
```

> `chat.BroadcastFunc` is `internal/store`'s type re-exported on the `chat` package. Add a thin alias in `chat.go`: `type BroadcastFunc = store.BroadcastFunc`.

`RegisterHubProjections` now wires the AgentRun projection (Workspace/Chat already broadcast via their store projections built in `New`):

```go
// RegisterHubProjections wires the AgentRun subscription that drives Chat status
// and the Workspace agent-running overlay (03 §7). Workspace and Chat broadcast
// directly from their read-model projections (built in New).
func (c *Container) RegisterHubProjections(
	axAgentRun asynx.Asynx[domain.AgentRun],
) error {
	return RegisterAgentRunProjection(axAgentRun, c.Chat, c.refreshWorkspace)
}

func (c *Container) refreshWorkspace(
	ctx context.Context,
	wsID string,
) {
	ws, err := c.Workspace.Get(ctx, wsID)
	if err != nil {
		return
	}
	c.hub.BroadcastWorkspace(ws)
}
```

`RecoverOrphans` runs the two-pass recovery:

```go
// RecoverOrphans runs AgentRun crash recovery (running→error) then the idempotent
// chat reconcile, in order, both draining via SendWait (00 §6.2).
func (c *Container) RecoverOrphans(
	ctx context.Context,
) {
	RecoverAgentRuns(ctx, c.AgentRun)
	ReconcileChats(ctx, c.Chat, c.AgentRun)
}
```

- [ ] **Step 4: Run green.** Run: `cd api && go test ./internal/app/repositories/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/repositories/
git commit -m "feat(app): repositories container builds read models + wires projections + recovery"
```

### Task 20: Update `app.New` to pass the DB + hub and register the AgentRun projection

**Files:**
- Modify: `internal/app/container.go`
- Test: modify `internal/app/container_test.go`

- [ ] **Step 1: Update `app.New`** — pass `gormStores`/`adapters.DB` and the hub into `repositories.New`, then call `repos.RegisterHubProjections(axAgentRun)` and `repos.RecoverOrphans(ctx)`. The hub must be constructed before repositories (its broadcast funcs are captured). Sketch:

```go
	h := hub.NewHub()
	repos, err := repositories.New(adapters.DB, h, axWorkspace, axChat, axAgentRun, axReviewThread)
	if err != nil {
		return nil, fmt.Errorf("app: repositories: %w", err)
	}
	if err := repos.RegisterHubProjections(axAgentRun); err != nil {
		return nil, fmt.Errorf("app: hub projections: %w", err)
	}
	repos.RecoverOrphans(ctx)
```

> Note the GORM read-model tables and the GORM CRUD tables share `adapters.DB`; `AutoMigrate` runs per `NewFromDB`. No conflict (distinct table names).

- [ ] **Step 2: Update the `app.Container` struct** to also hold the usecases container (filled in Task 28); for now keep `Hub`, `Repositories`, `GORM`.

- [ ] **Step 3: Run** `cd api && go build ./... && go test ./internal/app/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add api/internal/app/container.go api/internal/app/container_test.go
git commit -m "feat(app): wire DB+hub into repositories, register projections, run recovery"
```

---

## Phase 6 — Project import + GORM roll-up + usecases

### Task 21: Avatar generation helper

**Files:**
- Create: `internal/app/usecases/internal/avatar/avatar.go`
- Test: `internal/app/usecases/internal/avatar/avatar_test.go`

- [ ] **Step 1: Write failing tests**

```go
package avatar

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLabel_FirstAlnumUppercased(t *testing.T) {
	assert.Equal(t, "C", Label("crowbar"))
	assert.Equal(t, "9", Label("9front"))
	assert.Equal(t, "A", Label("  api"))
	assert.Equal(t, "?", Label(""))
	assert.Equal(t, "?", Label("---"))
}

func TestColor_StableForSameName(t *testing.T) {
	a := Color("crowbar")
	b := Color("crowbar")
	assert.Equal(t, a, b)
	assert.Contains(t, Palette(), a)
}

func TestColor_DistributesAcrossPalette(t *testing.T) {
	seen := map[string]bool{}
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		seen[Color(n)] = true
	}
	assert.GreaterOrEqual(t, len(seen), 2)
}
```

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement `avatar.go`**

```go
// Package avatar generates deterministic avatar labels and colors for repos from
// their names (00 §5.7). Pure functions — no state.
package avatar

import (
	"hash/fnv"
	"unicode"
)

func palette() []string {
	return []string{
		"avatar-rose",
		"avatar-amber",
		"avatar-emerald",
		"avatar-cyan",
		"avatar-indigo",
		"avatar-violet",
		"avatar-slate",
		"avatar-pink",
	}
}

// Palette returns the avatar color token set.
func Palette() []string {
	return palette()
}

// Label returns the single-char avatar badge: first alphanumeric char of name,
// uppercased; "?" when none.
func Label(
	name string,
) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return string(unicode.ToUpper(r))
		}
	}
	return "?"
}

// Color returns a stable palette token hashed from name.
func Color(
	name string,
) string {
	p := palette()
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return p[h.Sum32()%uint32(len(p))]
}
```

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/usecases/internal/avatar/
git commit -m "feat(usecases): deterministic avatar label/color (00 §5.7)"
```

### Task 22: Repo discovery helper (walk for `.git`, bounded depth)

**Files:**
- Create: `internal/app/usecases/internal/discover/discover.go`
- Test: `internal/app/usecases/internal/discover/discover_test.go`

- [ ] **Step 1: Write failing tests** — given a temp dir tree with two `.git` dirs at depth ≤ N and one beyond, `Repos(root, maxDepth)` returns the two in-bound repo roots. Build the fixture with `t.TempDir()` + `os.MkdirAll`.

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement `discover.go`** — `Repos(root string, maxDepth int) ([]string, error)` using `filepath.WalkDir`, recording the parent dir of any `.git` directory, skipping descent into a found repo and pruning beyond `maxDepth`. Keep functions ≤2 indent levels (extract the per-entry decision into a method on a `walker` struct holding results + maxDepth).

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/usecases/internal/discover/
git commit -m "feat(usecases): bounded-depth git repo discovery (00 §5.7)"
```

### Task 23: Default-branch resolver

**Files:**
- Create: `internal/app/usecases/internal/defaultbranch/defaultbranch.go`
- Test: `internal/app/usecases/internal/defaultbranch/defaultbranch_test.go`

- [ ] **Step 1: Write failing tests** with a fake git-runner func injected: when `symbolic-ref` yields `origin/HEAD` → that branch; else first of `[main, develop, master]` that `rev-parse --verify` confirms; else current HEAD. Inject a `type RefRunner func(args ...string) (string, bool)` so no real git needed.

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement `defaultbranch.go`** — `Resolve(runner RefRunner, configList []string) string` per 00 §5.2 ordering; never empty. Guard clauses, early returns.

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/usecases/internal/defaultbranch/
git commit -m "feat(usecases): default-branch resolver (00 §5.2)"
```

### Task 24: Project-import usecase

**Files:**
- Create: `internal/app/usecases/project_import.go`
- Test: `internal/app/usecases/project_import_test.go`
- Create: `internal/app/usecases/mocks/mocks.go` (GORM store + git engine mocks as needed)

> The import usecase (00 §5.7): create Project row; discover repos; per repo create a Repository row (defaultBranch resolved, avatar generated); adopt existing worktrees as Workspace rows (`forkPointSha` = `git merge-base <branch> <parentBranch>` — the one legitimate `merge-base` exception; `locked` via provider engine; `status` seeded from branch reality). It composes: `store.Store[domain.Project]`, `store.Store[domain.Repository]`, the `workspace.Workspace` repo, the git engine (`WorktreeList`, a `MergeBase` primitive — **verify it exists in the git engine**, else this task adds it to Wave-1 git engine; if absent, add `MergeBase(ctx, repoPath, a, b)` to `engine/git`), and the provider engine (`ProtectedBranches`).

- [ ] **Step 1: Check for a `MergeBase` git primitive**

Run: `cd api && grep -rn "MergeBase\|merge-base" internal/engine/git/`
If absent, first add `MergeBase(ctx, repoPath, a, b) (string, error)` to `engine/git` (new file `internal/engine/git/merge_base.go` + interface method + test using a real temp repo), commit separately:
```bash
git commit -m "feat(git): MergeBase primitive for adopted-worktree fork points (00 §5.7)"
```

- [ ] **Step 2: Write the failing usecase test** — with mocked stores + a fake git engine returning two worktrees and a fake provider returning `[main]` protected: `Import(ctx, name, path)` creates 1 project, 2 repos (avatars set, defaultBranch non-empty), and adopts the worktrees as workspaces (the `main` one locked). Assert via the mock stores' captured saves.

- [ ] **Step 3: Run red.**

- [ ] **Step 4: Implement `project_import.go`** — interface `ProjectImportUsecase` with `Import(ctx, name, path) (domain.Project, error)`. Decompose into small methods (`importRepos`, `importOneRepo`, `adoptWorktrees`, `adoptOneWorktree`) so each stays ≤2 indent levels and <100 LOC. Generate UUIDs via the project's existing UUID approach (check how IDs are minted elsewhere — likely `github.com/google/uuid`; **grep first**: `grep -rn "uuid" internal/ | head`). Use the discovered helpers (avatar, discover, defaultbranch) + `merge-base` for adopted fork points.

- [ ] **Step 5: Run green.** Run: `cd api && go test ./internal/app/usecases/ -run Import`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/app/usecases/project_import.go api/internal/app/usecases/project_import_test.go api/internal/app/usecases/mocks/
git commit -m "feat(usecases): project import — discover/adopt/avatar/forkpoint (00 §5.7)"
```

### Task 25: Project `lastActivity` roll-up helper

**Files:**
- Create: `internal/app/usecases/project.go`
- Test: `internal/app/usecases/project_test.go`

- [ ] **Step 1: Write failing tests** — `TouchProjectActivity(ctx, repoID, now)` looks up the repo's `projectId`, loads the project, updates `lastActivity`, saves. A GORM-update failure is logged, not returned (best-effort, 00 §5.1).

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement `project.go`** — `ProjectUsecase` with `List`, `Get`, and the internal `TouchProjectActivity` best-effort roll-up. Composes the Project + Repository stores.

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/usecases/project.go api/internal/app/usecases/project_test.go
git commit -m "feat(usecases): project read + best-effort lastActivity roll-up (00 §5.1)"
```

### Task 26: Workspace / Chat / File / Git / Terminal usecases (engine-composing, lifecycle only)

**Files:**
- Create: `internal/app/usecases/{workspace,chat,file,git,terminal}.go` + `_test.go` each

> These compose the engines with the aggregate repos for the **lifecycle/read** surface only. Worktree-hierarchy usecases (create-with-worktree, merge, reparent, delete-cascade) are **Plan 3D** — do NOT build them here. Branch-review composite is **Plan 3B**. Here:
> - `workspace.go`: `List`, `Get` (read model passthrough); `SetMergeStrategy` (used by review PATCH); the `SyncWorkingTreeState` wrapper the watcher/git-usecases call (recompute summary via `git.WorkingTreeSummary` then issue the command + best-effort project roll-up).
> - `chat.go`: `CreateChat` (mint id+title, command, TouchActivity on the workspace + project roll-up), `ForkChat` (load parent title, fork), `RenameChat`, `DeleteChat` (cascade to children via read-model `ListByWorkspace` filtered by parentId — idempotent).
> - `file.go`: thin pass-through to the fs engine for tree/content read/write/new/rename/delete; on write, trigger `SyncWorkingTreeState` + project roll-up.
> - `git.go`: thin pass-through to the git engine for the read+write surface (status/log/diff/branches/stashes/stage/commit/etc.); after any mutating op, recompute summary → `SyncWorkingTreeState` + project roll-up.
> - `terminal.go`: pass-through to the terminal engine (create/kill session, list/CRUD profiles via the `TerminalProfile` store).

For each usecase file:
- [ ] Write the failing test first (mock the repos + engines; assert composition — e.g. `DeleteChat` issues delete for the chat and each child; `CreateChat` issues `TouchActivity`).
- [ ] Run red.
- [ ] Implement the usecase as an interface + unexported impl (go-style rule 8), decomposed into ≤100-LOC, ≤2-indent methods.
- [ ] Run green.
- [ ] Commit each file separately, e.g. `feat(usecases): chat lifecycle usecase (cascade delete, activity touch)`.

> Cascade-delete child lookup: read-model `chat.ListByWorkspace(wsID)` then filter `ParentID == parent.ID`; recurse. `DeleteChat` is idempotent so replay is safe.

### Task 27: Provider-sync usecase (poll result → `SyncProviderState`)

**Files:**
- Create: `internal/app/usecases/provider_sync.go`
- Test: `internal/app/usecases/provider_sync_test.go`

- [ ] **Step 1: Write the failing test** — `SyncFromState(ctx, wsID, providerState)` maps a `provider.ProviderState` to `workspace.ProviderInput` and issues `SyncProviderState`; an on-view `PollWorkspace(ctx, wsID)` loads the workspace, calls `provider.PollOnView(worktreePath, branch)`, and applies the result. Mock the workspace repo + provider engine.

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement `provider_sync.go`** — `ProviderSyncUsecase` with `PollWorkspace(ctx, wsID)` and `SyncFromState(ctx, wsID, state)`; the latter is the callback the background sweep invokes (wired in Task 29). Map `state.PR` (nil-safe) → `HasPR`/`PRStatus`/url/title/target; `state.Protected` → `Protected`.

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/usecases/provider_sync.go api/internal/app/usecases/provider_sync_test.go
git commit -m "feat(usecases): provider poll → SyncProviderState (08 §5)"
```

### Task 28: Usecases container

**Files:**
- Create: `internal/app/usecases/container.go`
- Test: `internal/app/usecases/container_test.go`

- [ ] **Step 1: Write the failing test** — `New(repos, gormStores, engines)` returns a `*Container` exposing every usecase, non-nil.

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement `container.go`** — struct fields: `Project`, `ProjectImport`, `Workspace`, `Chat`, `File`, `Git`, `Terminal`, `ProviderSync`. `New` constructs each from `repos.Container`, the GORM stores, and `engine.Container`. One param per line.

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/usecases/container.go api/internal/app/usecases/container_test.go
git commit -m "feat(usecases): usecases container wiring"
```

### Task 29: Mount usecases in `app.New` + start the provider background sweep

**Files:**
- Modify: `internal/app/container.go`
- Test: modify `internal/app/container_test.go`

- [ ] **Step 1: Write the failing test** — `app.New` returns a container whose `Usecases` field is non-nil and whose workspace usecase `List` works end-to-end after creating a workspace through the repo.

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Update `app.New`** — after repositories + recovery, build `usecases.New(repos, gormStores, engines)` and store on `Container.Usecases`. Start the provider sweep:

```go
	engines.Provider.StartBackgroundSweep(
		ctx,
		sweepTargets(ctx, repos.Workspace),
		func(wsID string, state provider.ProviderState) {
			ucs.ProviderSync.SyncFromState(context.WithoutCancel(ctx), wsID, state)
		},
	)
```

where `sweepTargets` returns a `func() []poll.SweepTarget` that lists workspaces with an open PR (status `pr-open`) from the read model. Add `engines *engine.Container` is already passed to `app.New` (currently `_`); un-blank it.

> The sweep callback must not use the request-scoped ctx for the command; use `context.WithoutCancel(ctx)` or a background context derived from the app lifetime so a cancelled poll ctx doesn't abort the command.

- [ ] **Step 4: Run green.** Run: `cd api && go build ./... && go vet ./... && go test ./internal/app/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/container.go api/internal/app/container_test.go
git commit -m "feat(app): mount usecases + start provider background sweep (08 §4)"
```

---

## Phase 7 — Full verification

### Task 30: Whole-tree build, vet, coverage, lint

- [ ] **Step 1: Format + build + vet**

Run: `cd api && gofumpt -l -w . && goimports -w . && go build ./... && go vet ./...`
Expected: no output from gofumpt (already formatted), build + vet clean.

- [ ] **Step 2: Run the full app-layer test suite with coverage**

Run: `cd api && go test -coverpkg=./internal/app/... -coverprofile=cover.out ./internal/app/... && go tool cover -func=cover.out | tail -1`
Expected: total coverage **≥95%**. If any package is below, add tests for the uncovered branches (error paths on store marshal/unmarshal, projection save-error logging, recovery warn paths) before proceeding.

- [ ] **Step 3: Run benchmarks to confirm hot paths execute**

Run: `cd api && go test ./internal/app/repositories/... -bench . -benchtime 20x -run '^$'`
Expected: all projection/storage benchmarks run without error.

- [ ] **Step 4: Run the lint gate**

Run: `cd api && golangci-lint run ./internal/app/... ./internal/domain/...`
Expected: no findings (funlen/gocyclo/nestif/revive clean).

- [ ] **Step 5: Run the whole suite + race once**

Run: `cd api && go test -race ./...`
Expected: PASS, no data races.

- [ ] **Step 6: Final commit**

```bash
git add -A api/
git commit -m "test(wave3a): full build/vet/coverage/lint/race green"
```

---

## Self-Review checklist (run before handing off)

- **Spec coverage:** Workspace full command set (00 §5.3 table) ✓ Tasks 3-6; GORM read models for all four aggregates ✓ Tasks 7-16; project-import usecase (00 §5.7) ✓ Task 24; Project lastActivity roll-up (00 §5.1) ✓ Task 25; `RegisterHubProjections` + AgentRun→Chat/overlay (03 §7, 01 §5) ✓ Tasks 17, 19; crash-recovery two-pass (00 §6.2) ✓ Tasks 18-19; lifecycle usecases ✓ Task 26; provider-poll → `SyncProviderState` (08 §5) ✓ Tasks 27, 29.
- **Deferred correctly:** AgentRun mutation/send path (kept as shape only — Plan deferred `12`); worktree-hierarchy usecases (→ 3D); branch-review composite (→ 3B); LSP (→ 3C).
- **Type consistency:** `ProviderInput`, `OpenInput`, `CreateInput`, `SyncInput` field names stable across repo/command/usecase; `BroadcastFunc` per store package; `chat.BroadcastFunc = store.BroadcastFunc` alias.
- **No placeholders:** every command + store + projection has complete code; usecase Tasks 26 specify exact composition behavior and test assertions per file.

---

## Execution Handoff

This plan is one of four for Wave 3. **3A must land before 3D and the provider wiring it depends on.** Recommended: **Subagent-Driven** execution (fresh subagent per task, two-stage review between tasks). Dependent plans: `2026-06-05-wave3d-worktree-hierarchy.md`, `2026-06-05-wave3b-branch-review.md`, `2026-06-05-wave3c-lsp.md`.
