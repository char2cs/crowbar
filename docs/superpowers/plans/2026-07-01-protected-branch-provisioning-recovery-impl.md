# Protected-Branch Provisioning & Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make protected-branch provisioning robust and recoverable — no protected branch is ever silently dropped; every one becomes a healthy managed worktree or a visible, retryable placeholder workspace, and the user's real checkout is only detached with consent.

**Architecture:** A new shared `internal/holder` primitive prunes dead worktree registrations and classifies who holds a branch (home / managed / external / free). The project-import path stops force-detaching the repo home and routes every live-held protected branch through one owner that persists a **placeholder workspace** — a `locked` row with an empty `WorktreePath` and a new `HeldByPath` field. The worktree usecase gains **Retry-provision-in-place** and **Detach-holder** operations plus empty-`WorktreePath` parent guards. The frontend derives the placeholder from `status === 'locked' && !localPath`, renders a warning glyph + Retry/Detach actions, a consent modal, and a toast watcher.

**Tech Stack:** Go 1.x (backend usecases, Asynx event-sourced workspace repo, gin REST), TypeScript + React + Zustand + Vitest + base-ui dialog (frontend). Tests: `go test ./...` (backend), `npx vitest run` + `npx vite build` (frontend).

## Global Constraints

- **Placeholder representation:** a placeholder is a `WorkspaceStatusLocked` row with `WorktreePath == ""` and a set `HeldByPath`. **No new `WorkspaceStatus` enum value.** The predicate `status == locked && WorktreePath == "" (localPath falsy on FE)` identifies it.
- **No `LastError` is written for a placeholder at import.** `WorkspaceCreator` stays `Create`-only (deliberately NOT widened with `SetLastError`). The human-readable reason is reconstructed on the FE from `HeldByPath` + the branch name. `HeldByPath` is the single durable placeholder signal (a provider poll blanks `LastError` but never `HeldByPath`).
- **Single owner of placeholder creation:** `provisionProtectedBranchWorktree` is the only site that creates a placeholder. `adoptRepoHome` creates none.
- **Holder path comparison uses symlink-resolved `samePath`**, never a literal `==` (git emits fully-resolved paths, e.g. macOS `/var`→`/private/var`).
- **`WorktreePrune` runs before every resolution** (import and Retry). It only reaps dead-directory registrations, so it is safe to run unconditionally.
- **No new backend permission for child-create under a placeholder parent** — child-create is already enabled on locked parents.
- **No migration code** (pre-production; dev clears `~/.crowbar`). New field + DTO field are additive.
- **The FE `heldByPath` mapping is unconditional** (`heldByPath: ws.heldByPath ?? ''`), never conditional-spread, because `applyWorkspaceDTO` merges frames with `{...w, ...ws}` and a cleared holder path (absent under `omitempty`) must overwrite the stale value.
- **Toast is fired from a component watching store state via `useEffect`**, never from a store or the backend, using `toast.show({ message, description, type: 'error', key, action })` (NOT `toast.error(...)`, which has no `action` param).
- **This repo has entangled uncommitted work in the target files. DO NOT run `git commit` or `git add`.** Each task ends with a "verify suite still green" checkpoint instead of a commit.
- Backend module root is `api/`. Frontend root is `web/`. All test/build commands `cd` into the correct root.
- Test files live under `web/src/__tests__/` mirroring `web/src/`; use `@/` imports. Component files are kebab-case.

---

## File Structure

**Backend — created**
- `api/internal/app/usecases/internal/holder/holder.go` — the shared `Resolve(ctx, git, repoPath, branch, crowbarHome) (Outcome, error)` primitive: prune-then-list-then-classify (`Free`/`HeldByHome`/`HeldByManaged`/`HeldByExternal`); narrow `Engine { WorktreePrune; WorktreeList }` interface; symlink-aware `samePath`/`isUnder`.
- `api/internal/app/usecases/internal/holder/holder_test.go` — unit tests for `Resolve` over a fake engine.
- `api/internal/app/repositories/workspace/internal/commands/provision_in_place.go` — `ProvisionInPlace` command: set `WorktreePath`+`ForkPointSha`, clear `HeldByPath`, keep `Status`.
- `api/internal/app/repositories/workspace/internal/commands/clear_branch.go` — `ClearBranch` command: blank `Branch` to `""`, touch nothing else.

**Backend — modified**
- `api/internal/domain/workspace.go` — add `HeldByPath` field to `domain.Workspace`.
- `api/internal/api/v0/dto/workspace.go` — add `HeldByPath` to `WorkspaceDTO` + map it in `WorkspaceDTOFrom`.
- `api/internal/app/repositories/workspace/workspace.go` — add `HeldByPath` to `CreateInput`; thread into `CreateWorkspace`; add `ProvisionInPlace` + `ClearBranch` to the `Workspace` interface + implementations.
- `api/internal/app/repositories/workspace/internal/commands/create.go` — carry `HeldByPath` on `CreateWorkspace` + `EmitEvent`.
- `api/internal/app/usecases/project/project_import.go` — add `WorktreePrune` to `ImportGitEngine`; stop force-detaching in `adoptRepoHome`; route live-held protected branches through `provisionProtectedBranchWorktree` as placeholders; best-effort FF in `addProtectedWorktree`; delete now-unused `toSet`.
- `api/internal/app/usecases/worktree/worktree.go` — empty-`WorktreePath` parent guards in `guardMerge`/`guardReparent`/`RebaseOntoParent`; skip git teardown on empty path in `removeOne`; refit `CreateChild` holder detection onto `holder.Resolve`; add `RetryProvision`/`DetachHolder` ops + `Usecase` interface methods.
- `api/internal/app/usecases/worktree/worktree_errors.go` — add `ErrParentUnprovisioned` + `ErrBranchStillHeld`.
- `api/internal/api/libs/status.go` — map `ErrParentUnprovisioned` to 409.
- `api/internal/api/v0/endpoints/workspaces/handlers/handlers.go` — add `RetryProvision`/`DetachHolder` to the `Hierarchy` interface.
- `api/internal/api/v0/endpoints/workspaces/handlers/hierarchy.go` — add `RetryProvision`/`DetachHolder` handlers.
- `api/internal/api/v0/endpoints/workspaces/routes.go` — mount `retry-provision` + `detach-holder` routes.
- `api/internal/app/usecases/mocks/mocks.go` — add `WorktreePrune` to `mocks.GitEngine`; copy `HeldByPath` in `mocks.WorkspaceRepo.Create`.
- `api/internal/app/repositories/workspace/internal/mocks/mocks.go`, `api/internal/app/usecases/worktree/worktree_test.go`, `api/internal/app/usecases/branchreview/branch_review_test.go` — add `ProvisionInPlace`/`ClearBranch` to the three full-interface `workspace.Workspace` fakes.
- `api/internal/api/v0/endpoints/workspaces/handlers/handlers_test.go` — add `RetryProvision`/`DetachHolder` stubs to `fakeHierarchy`.

**Frontend — created**
- `web/src/lib/workspace/placeholder.ts` — `isPlaceholderWorkspace(ws)` + `placeholderReason(ws)`.
- `web/src/features/window/stores/detach-modal-store.ts` — global UI store holding the detach-modal target.
- `web/src/components/layout/detach-holder-modal.tsx` — consent modal (reads the store, calls `detachHolder`).
- `web/src/components/layout/placeholder-row-actions.tsx` — Retry + Detach buttons + reason line for a placeholder row.
- `web/src/components/layout/placeholder-toast-watcher.tsx` — watches sidebar state, fires the toast once per new placeholder.

**Frontend — modified**
- `web/src/lib/types.ts` — add `heldByPath?` to `WorkspaceDTO`.
- `web/src/lib/store/sidebar.ts` — add `heldByPath?` to `Workspace`.
- `web/src/lib/store/build-repo-tree.ts` — map `heldByPath` unconditionally in `toSidebarWorkspace`.
- `web/src/components/layout/workspace-branch-icon.tsx` — `isPlaceholder` prop → warning glyph ahead of the `locked` case.
- `web/src/components/layout/workspace-tree-item.tsx` — compute `isPlaceholder`, pass to the icon, render `PlaceholderRowActions`.
- `web/src/lib/api/workspace.ts` — `retryProvision(wsId)` + `detachHolder(wsId)`.

**Frontend — test files created (mirror structure)**
- `web/src/__tests__/lib/workspace/placeholder.test.ts`
- `web/src/__tests__/features/window/stores/detach-modal-store.test.ts`
- `web/src/__tests__/components/layout/detach-holder-modal.test.tsx`
- `web/src/__tests__/components/layout/placeholder-row-actions.test.tsx`
- `web/src/__tests__/components/layout/placeholder-toast-watcher.test.tsx`
- `web/src/__tests__/components/layout/workspace-branch-icon.test.tsx`
- `web/src/__tests__/lib/api/workspace.test.ts`
- (extend) `web/src/__tests__/lib/store/build-repo-tree.test.ts`

---

## Task 1: `HeldByPath` on the domain aggregate + DTO

**Files:**
- Modify: `api/internal/domain/workspace.go:56` (add field after `Kind`)
- Modify: `api/internal/api/v0/dto/workspace.go:37` (add field), `:70` (map field)
- Test: `api/internal/api/v0/dto/workspace_test.go` (append a test)

**Interfaces:**
- Consumes: `domain.Workspace`, `workspace.MergeEligibility{}`, `dto.WorkspaceDTOFrom(w domain.Workspace, elig workspace.MergeEligibility) dto.WorkspaceDTO`.
- Produces: `domain.Workspace.HeldByPath string` (json `heldByPath,omitempty`); `dto.WorkspaceDTO.HeldByPath string` (json `heldByPath,omitempty`), populated from `w.HeldByPath`.

- [ ] **Step 1: Write the failing test**

Append to `api/internal/api/v0/dto/workspace_test.go`:

```go
// TestWorkspaceDTOFrom_MapsHeldByPath proves the placeholder holder path
// (domain.Workspace.HeldByPath) is carried onto the wire DTO so the FE can
// reconstruct the placeholder reason from it (spec §4/B3).
func TestWorkspaceDTOFrom_MapsHeldByPath(t *testing.T) {
	got := dto.WorkspaceDTOFrom(
		domain.Workspace{ID: "w1", HeldByPath: "/Users/me/proj"},
		workspace.MergeEligibility{},
	)
	assert.Equal(t, "/Users/me/proj", got.HeldByPath)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/api/v0/dto/ -run TestWorkspaceDTOFrom_MapsHeldByPath`
Expected: FAIL — compile error `got.HeldByPath undefined` and `unknown field HeldByPath in struct literal of type domain.Workspace`.

- [ ] **Step 3: Add the domain field**

In `api/internal/domain/workspace.go`, add after the `Kind` field (line 56):

```go
	Kind WorkspaceKind `json:"kind,omitempty"`
	// HeldByPath is the worktree directory currently holding this workspace's
	// branch, set only on a PLACEHOLDER (a locked row with an empty WorktreePath)
	// that could not get a managed worktree because a live worktree — the repo
	// home or an external checkout — holds the branch. It is the single durable
	// signal from which the frontend reconstructs the placeholder reason; a
	// successful Retry clears it. Empty on every healthy workspace (00 §4, spec §4).
	HeldByPath string `json:"heldByPath,omitempty"`
```

- [ ] **Step 4: Add + map the DTO field**

In `api/internal/api/v0/dto/workspace.go`, add to `WorkspaceDTO` after `LocalPath` (line 37):

```go
	LocalPath string `json:"localPath,omitempty"`
	// HeldByPath is the worktree directory holding this branch when the workspace
	// is a placeholder (locked + empty LocalPath). The client reconstructs the
	// "checked out elsewhere" reason from it; absent on healthy workspaces.
	HeldByPath string `json:"heldByPath,omitempty"`
```

In `WorkspaceDTOFrom`, add after `LocalPath: w.WorktreePath,` (line 70):

```go
		LocalPath:       w.WorktreePath,
		HeldByPath:      w.HeldByPath,
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd api && go test ./internal/api/v0/dto/ -run TestWorkspaceDTOFrom_MapsHeldByPath`
Expected: PASS.

- [ ] **Step 6: Verify suite still green**

Run: `cd api && go test ./...`
Expected: PASS (all packages).

---

## Task 2: Thread `HeldByPath` through create

**Files:**
- Modify: `api/internal/app/repositories/workspace/workspace.go:33-45` (`CreateInput`), `:375-388` (`CreateWorkspace` dispatch)
- Modify: `api/internal/app/repositories/workspace/internal/commands/create.go:14-27` (command field), `:73-87` (`EmitEvent`)
- Test: `api/internal/app/repositories/workspace/internal/commands/commands_test.go` (append)

**Interfaces:**
- Consumes: `commands.CreateWorkspace`, `workspace.CreateInput`.
- Produces: `workspace.CreateInput.HeldByPath string`; `commands.CreateWorkspace.HeldByPath string` carried into `EmitEvent`'s returned `domain.Workspace.HeldByPath`.

- [ ] **Step 1: Write the failing test**

Append to `api/internal/app/repositories/workspace/internal/commands/commands_test.go`:

```go
func TestCreateWorkspace_EmitEvent_CarriesHeldByPath(t *testing.T) {
	now := time.Unix(1000, 0)
	ws := CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1",
		Protected: true, HeldByPath: "/repo", Now: now,
	}.EmitEvent(nil)
	assert.Equal(t, "/repo", ws.HeldByPath)
	assert.Equal(t, domain.WorkspaceStatusLocked, ws.Status,
		"a placeholder is still seeded locked from Protected")
	assert.Empty(t, ws.WorktreePath, "a placeholder carries no worktree path")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/commands/ -run TestCreateWorkspace_EmitEvent_CarriesHeldByPath`
Expected: FAIL — `unknown field HeldByPath in struct literal of type commands.CreateWorkspace`.

- [ ] **Step 3: Add the command field + emit it**

In `api/internal/app/repositories/workspace/internal/commands/create.go`, add to the `CreateWorkspace` struct after `Kind` (line 25):

```go
	Kind          domain.WorkspaceKind
	HeldByPath    string
	Now           time.Time
```

In `EmitEvent`, add to the returned struct literal after `Kind: kind,` (line 84):

```go
		Kind:          kind,
		HeldByPath:    c.HeldByPath,
		LastActivity:  c.Now,
```

- [ ] **Step 4: Thread it through the repository dispatch**

In `api/internal/app/repositories/workspace/workspace.go`, add to `CreateInput` after `Kind` (line 44):

```go
	Kind          domain.WorkspaceKind
	HeldByPath    string
}
```

In `Create`, add to the `commands.CreateWorkspace{...}` literal after `Kind: in.Kind,` (line 386):

```go
		Kind:          in.Kind,
		HeldByPath:    in.HeldByPath,
		Now:           now,
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/commands/ -run TestCreateWorkspace_EmitEvent_CarriesHeldByPath`
Expected: PASS.

- [ ] **Step 6: Verify suite still green**

Run: `cd api && go test ./...`
Expected: PASS.

---

## Task 3: `ProvisionInPlace` command + repository method

**Files:**
- Create: `api/internal/app/repositories/workspace/internal/commands/provision_in_place.go`
- Create test: `api/internal/app/repositories/workspace/internal/commands/provision_in_place_test.go`
- Modify: `api/internal/app/repositories/workspace/workspace.go:68-151` (interface), append a method impl
- Modify (fakes): `api/internal/app/repositories/workspace/internal/mocks/mocks.go`, `api/internal/app/usecases/worktree/worktree_test.go`, `api/internal/app/usecases/branchreview/branch_review_test.go`

**Interfaces:**
- Consumes: `domain.Workspace`, `asynxModels.ErrValidation`.
- Produces: `commands.ProvisionInPlace{ID, WorktreePath, ForkPointSha string}`; repository method `ProvisionInPlace(ctx context.Context, id, worktreePath, forkPointSha string) (domain.Workspace, error)` on `workspace.Workspace`.

- [ ] **Step 1: Write the failing test**

Create `api/internal/app/repositories/workspace/internal/commands/provision_in_place_test.go`:

```go
package commands_test

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestProvisionInPlace_SetsPathAndClearsHeldBy(t *testing.T) {
	cur := &domain.Workspace{
		ID: "w1", Branch: "develop", Status: domain.WorkspaceStatusLocked,
		HeldByPath: "/repo", WorktreePath: "",
	}
	got := commands.ProvisionInPlace{ID: "w1", WorktreePath: "/managed", ForkPointSha: "sha"}.EmitEvent(cur)
	assert.Equal(t, "/managed", got.WorktreePath)
	assert.Equal(t, "sha", got.ForkPointSha)
	assert.Empty(t, got.HeldByPath, "a successful provision clears the holder path")
	assert.Equal(t, domain.WorkspaceStatusLocked, got.Status, "status stays locked")
	assert.Equal(t, "develop", got.Branch, "branch is untouched")
}

func TestProvisionInPlace_Validate_RejectsMissing(t *testing.T) {
	err := commands.ProvisionInPlace{ID: "w1", WorktreePath: "/m"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/commands/ -run TestProvisionInPlace`
Expected: FAIL — `undefined: commands.ProvisionInPlace`.

- [ ] **Step 3: Write the command**

Create `api/internal/app/repositories/workspace/internal/commands/provision_in_place.go`:

```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ProvisionInPlace flips a placeholder (a locked row with an empty WorktreePath)
// into a healthy managed worktree WITHOUT creating a new aggregate: it records
// the now-attached worktree path + fork point and clears HeldByPath, leaving
// Status = locked and every other field untouched (spec §3.3 Retry-in-place).
type ProvisionInPlace struct {
	ID           string
	WorktreePath string
	ForkPointSha string
}

func (c ProvisionInPlace) AggregateID() string {
	return c.ID
}

func (c ProvisionInPlace) EventName() string {
	return "workspace.provisioned_in_place." + c.ID
}

func (c ProvisionInPlace) ShouldSnapshot() bool {
	return true
}

func (c ProvisionInPlace) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("provision in place: %w", asynxModels.ErrValidation)
	}
	if c.WorktreePath == "" {
		return fmt.Errorf("provision in place: missing worktree path: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ProvisionInPlace) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.WorktreePath = c.WorktreePath
	ws.ForkPointSha = c.ForkPointSha
	ws.HeldByPath = ""
	return ws
}
```

- [ ] **Step 4: Run the command test to verify it passes**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/commands/ -run TestProvisionInPlace`
Expected: PASS.

- [ ] **Step 5: Add the repository method to the interface + impl**

In `api/internal/app/repositories/workspace/workspace.go`, add to the `Workspace` interface after `UpdateForkPoint` (line 114):

```go
	UpdateForkPoint(
		ctx context.Context,
		id string,
		forkPointSha string,
	) (domain.Workspace, error)
	// ProvisionInPlace attaches a worktree to a placeholder row (spec §3.3): it
	// records worktreePath + forkPointSha and clears HeldByPath, keeping Status.
	ProvisionInPlace(
		ctx context.Context,
		id string,
		worktreePath string,
		forkPointSha string,
	) (domain.Workspace, error)
```

Add the implementation after the `UpdateForkPoint` method (after line 555):

```go
func (w *workspace) ProvisionInPlace(
	ctx context.Context,
	id string,
	worktreePath string,
	forkPointSha string,
) (domain.Workspace, error) {
	entity, release, err := w.entityFor(ctx, id)
	if err != nil {
		return domain.Workspace{}, err
	}
	defer release()
	evt, err := entity.send(ctx, commands.ProvisionInPlace{
		ID:           id,
		WorktreePath: worktreePath,
		ForkPointSha: forkPointSha,
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: provision in place: %w", err)
	}
	return evt.Aggregate, nil
}
```

- [ ] **Step 6: Add the method to the three full-interface fakes**

In `api/internal/app/usecases/worktree/worktree_test.go`, add after `UpdateForkPoint` (after line 88):

```go
func (f *fakeWorkspace) ProvisionInPlace(
	_ context.Context,
	id string,
	worktreePath string,
	forkPointSha string,
) (domain.Workspace, error) {
	if f.ProvisionInPlaceFn != nil {
		return f.ProvisionInPlaceFn(id, worktreePath, forkPointSha)
	}
	return domain.Workspace{ID: id, WorktreePath: worktreePath, ForkPointSha: forkPointSha}, nil
}
```

Add the field to the `fakeWorkspace` struct (after `SyncFn` at line 34):

```go
	SyncFn             func(ctx context.Context, in workspace.SyncInput, now time.Time) (domain.Workspace, error)
	ProvisionInPlaceFn func(id, worktreePath, forkPointSha string) (domain.Workspace, error)
	ClearBranchFn      func(id string) (domain.Workspace, error)
```

(The `ClearBranchFn` field is added now so Task 4 only appends the method.)

In `api/internal/app/repositories/workspace/internal/mocks/mocks.go`, add a no-op method to `MockWorkspace`:

```go
func (m *MockWorkspace) ProvisionInPlace(
	_ context.Context,
	id string,
	worktreePath string,
	forkPointSha string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, WorktreePath: worktreePath, ForkPointSha: forkPointSha}, nil
}
```

In `api/internal/app/usecases/branchreview/branch_review_test.go`, add a no-op method to `mockWorkspace`:

```go
func (m *mockWorkspace) ProvisionInPlace(
	_ context.Context,
	id string,
	worktreePath string,
	forkPointSha string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, WorktreePath: worktreePath, ForkPointSha: forkPointSha}, nil
}
```

- [ ] **Step 7: Verify suite still green**

Run: `cd api && go test ./...`
Expected: PASS (all three fakes now satisfy the widened interface).

---

## Task 4: `ClearBranch` command + repository method

**Files:**
- Create: `api/internal/app/repositories/workspace/internal/commands/clear_branch.go`
- Create test: `api/internal/app/repositories/workspace/internal/commands/clear_branch_test.go`
- Modify: `api/internal/app/repositories/workspace/workspace.go` (interface + impl)
- Modify (fakes): the three full-interface `workspace.Workspace` fakes

**Interfaces:**
- Consumes: `domain.Workspace`, `asynxModels.ErrValidation`.
- Produces: `commands.ClearBranch{ID string}`; repository method `ClearBranch(ctx context.Context, id string) (domain.Workspace, error)` on `workspace.Workspace`.

- [ ] **Step 1: Write the failing test**

Create `api/internal/app/repositories/workspace/internal/commands/clear_branch_test.go`:

```go
package commands_test

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestClearBranch_BlanksBranchOnly(t *testing.T) {
	cur := &domain.Workspace{
		ID: "home", Branch: "develop", WorktreePath: "/repo",
		Status: domain.WorkspaceStatusNew, IsDefault: true,
	}
	got := commands.ClearBranch{ID: "home"}.EmitEvent(cur)
	assert.Empty(t, got.Branch, "branch is blanked")
	assert.Equal(t, "/repo", got.WorktreePath, "worktree path is untouched")
	assert.Equal(t, domain.WorkspaceStatusNew, got.Status, "status is untouched")
	assert.True(t, got.IsDefault, "identity is untouched (chats/threads preserved)")
}

func TestClearBranch_Validate_RejectsMissing(t *testing.T) {
	err := commands.ClearBranch{ID: "home"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/commands/ -run TestClearBranch`
Expected: FAIL — `undefined: commands.ClearBranch`.

- [ ] **Step 3: Write the command**

Create `api/internal/app/repositories/workspace/internal/commands/clear_branch.go`:

```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ClearBranch blanks an existing aggregate's Branch to "" and touches nothing
// else. It is genuinely new capability: no other command mutates Branch
// (CreateWorkspace sets it once). Used by the consented Detach-holder op when
// the holder is the repo home, replacing the homeBranch="" blanking the old
// force-detaching adoptRepoHome did — without a delete-and-recreate that would
// drop the home aggregate's chats/threads (spec §3.5/§3.7/B6).
type ClearBranch struct {
	ID string
}

func (c ClearBranch) AggregateID() string {
	return c.ID
}

func (c ClearBranch) EventName() string {
	return "workspace.branch_cleared." + c.ID
}

func (c ClearBranch) ShouldSnapshot() bool {
	return false
}

func (c ClearBranch) Validate(
	current *domain.Workspace,
) error {
	if current == nil {
		return fmt.Errorf("clear branch: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ClearBranch) EmitEvent(
	current *domain.Workspace,
) domain.Workspace {
	ws := *current
	ws.Branch = ""
	return ws
}
```

- [ ] **Step 4: Run the command test to verify it passes**

Run: `cd api && go test ./internal/app/repositories/workspace/internal/commands/ -run TestClearBranch`
Expected: PASS.

- [ ] **Step 5: Add the repository method to the interface + impl**

In `api/internal/app/repositories/workspace/workspace.go`, add to the `Workspace` interface after the `ProvisionInPlace` block added in Task 3:

```go
	// ClearBranch blanks an existing aggregate's Branch to "" (spec §4/B6),
	// leaving every other field untouched. Used by the Detach-holder op when the
	// holder is the repo home.
	ClearBranch(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
```

Add the implementation after `ProvisionInPlace` (from Task 3):

```go
func (w *workspace) ClearBranch(
	ctx context.Context,
	id string,
) (domain.Workspace, error) {
	entity, release, err := w.entityFor(ctx, id)
	if err != nil {
		return domain.Workspace{}, err
	}
	defer release()
	evt, err := entity.send(ctx, commands.ClearBranch{ID: id})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: clear branch: %w", err)
	}
	return evt.Aggregate, nil
}
```

- [ ] **Step 6: Add the method to the three full-interface fakes**

In `api/internal/app/usecases/worktree/worktree_test.go`, add after the `ProvisionInPlace` method:

```go
func (f *fakeWorkspace) ClearBranch(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	if f.ClearBranchFn != nil {
		return f.ClearBranchFn(id)
	}
	return domain.Workspace{ID: id}, nil
}
```

In `api/internal/app/repositories/workspace/internal/mocks/mocks.go`, add:

```go
func (m *MockWorkspace) ClearBranch(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id}, nil
}
```

In `api/internal/app/usecases/branchreview/branch_review_test.go`, add:

```go
func (m *mockWorkspace) ClearBranch(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id}, nil
}
```

- [ ] **Step 7: Verify suite still green**

Run: `cd api && go test ./...`
Expected: PASS.

---

## Task 5: `holder` shared primitive

**Files:**
- Create: `api/internal/app/usecases/internal/holder/holder.go`
- Create test: `api/internal/app/usecases/internal/holder/holder_test.go`

**Interfaces:**
- Consumes: `gitengine.WorktreeEntry` (`github.com/char2cs/crowbar/api/internal/engine/git`).
- Produces:
  - `holder.Kind` with `Free`, `HeldByHome`, `HeldByManaged`, `HeldByExternal`.
  - `holder.Outcome{Kind Kind; HeldByPath string}`.
  - `holder.Engine interface { WorktreePrune(ctx, repoPath) error; WorktreeList(ctx, repoPath) ([]gitengine.WorktreeEntry, error) }`.
  - `holder.Resolve(ctx context.Context, git Engine, repoPath, branch, crowbarHome string) (Outcome, error)` — prunes first (best-effort), lists, classifies via symlink-aware `samePath`/`isUnder`.

- [ ] **Step 1: Write the failing test**

Create `api/internal/app/usecases/internal/holder/holder_test.go`:

```go
package holder_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/holder"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

type fakeEngine struct {
	pruned  []string
	entries []gitengine.WorktreeEntry
	listErr error
}

func (f *fakeEngine) WorktreePrune(_ context.Context, repoPath string) error {
	f.pruned = append(f.pruned, repoPath)
	return nil
}

func (f *fakeEngine) WorktreeList(_ context.Context, _ string) ([]gitengine.WorktreeEntry, error) {
	return f.entries, f.listErr
}

func TestResolve_PrunesFirst(t *testing.T) {
	e := &fakeEngine{}
	_, err := holder.Resolve(context.Background(), e, "/repo", "develop", "/home")
	require.NoError(t, err)
	assert.Equal(t, []string{"/repo"}, e.pruned, "prune runs before listing (dead-reg case)")
}

func TestResolve_FreeWhenNoHolder(t *testing.T) {
	e := &fakeEngine{entries: []gitengine.WorktreeEntry{{Path: "/repo", Branch: "main"}}}
	out, err := holder.Resolve(context.Background(), e, "/repo", "develop", "/home")
	require.NoError(t, err)
	assert.Equal(t, holder.Free, out.Kind)
	assert.Empty(t, out.HeldByPath)
}

func TestResolve_HeldByHome(t *testing.T) {
	e := &fakeEngine{entries: []gitengine.WorktreeEntry{{Path: "/repo", Branch: "develop"}}}
	out, err := holder.Resolve(context.Background(), e, "/repo", "develop", "/home")
	require.NoError(t, err)
	assert.Equal(t, holder.HeldByHome, out.Kind)
	assert.Equal(t, "/repo", out.HeldByPath)
}

func TestResolve_HeldByManaged(t *testing.T) {
	e := &fakeEngine{entries: []gitengine.WorktreeEntry{
		{Path: "/home/projects/p/r/workspaces/w/worktree", Branch: "develop"},
	}}
	out, err := holder.Resolve(context.Background(), e, "/repo", "develop", "/home")
	require.NoError(t, err)
	assert.Equal(t, holder.HeldByManaged, out.Kind)
}

func TestResolve_HeldByExternal(t *testing.T) {
	e := &fakeEngine{entries: []gitengine.WorktreeEntry{
		{Path: "/somewhere/else", Branch: "develop"},
	}}
	out, err := holder.Resolve(context.Background(), e, "/repo", "develop", "/home")
	require.NoError(t, err)
	assert.Equal(t, holder.HeldByExternal, out.Kind)
	assert.Equal(t, "/somewhere/else", out.HeldByPath)
}

func TestResolve_ListError(t *testing.T) {
	e := &fakeEngine{listErr: errors.New("boom")}
	_, err := holder.Resolve(context.Background(), e, "/repo", "develop", "/home")
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/app/usecases/internal/holder/`
Expected: FAIL — `no required module provides package .../holder` / `undefined: holder.Resolve`.

- [ ] **Step 3: Write the primitive**

Create `api/internal/app/usecases/internal/holder/holder.go`:

```go
// Package holder resolves who currently holds a git branch across a repo's
// worktrees, so both the project-import path and the worktree usecase can decide
// whether a protected branch is free to materialise, already managed, or held by
// a live worktree that must be freed with user consent (spec §3.1). Resolution
// never detaches — it only prunes dead registrations and classifies the holder.
package holder

import (
	"context"
	"path/filepath"
	"strings"

	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

// Kind classifies who holds a branch.
type Kind int

const (
	// Free: no worktree holds the branch (after pruning dead registrations).
	Free Kind = iota
	// HeldByHome: the repo's main folder (the unmanaged default workspace).
	HeldByHome
	// HeldByManaged: a Crowbar-managed worktree under <crowbarHome> — already a
	// workspace; never double-provision.
	HeldByManaged
	// HeldByExternal: a live worktree the user made outside the crowbar home.
	HeldByExternal
)

// Outcome is a resolved holder classification. HeldByPath is the holder's
// worktree directory (empty for Free).
type Outcome struct {
	Kind       Kind
	HeldByPath string
}

// Engine is the narrow git surface Resolve needs — satisfied by both the import
// usecase's ImportGitEngine (once WorktreePrune is added) and the worktree
// usecase's full enginegit.Engine. DetachWorktree is deliberately absent:
// resolution only prunes + lists; the detach is a separate consented op.
type Engine interface {
	WorktreePrune(ctx context.Context, repoPath string) error
	WorktreeList(ctx context.Context, repoPath string) ([]gitengine.WorktreeEntry, error)
}

// Resolve prunes dead-directory registrations, then finds the worktree holding
// branch and classifies it. Pruning is best-effort (it only reaps dead regs, so
// a failure just means classification runs against a possibly-stale list); a
// WorktreeList failure is fatal and returned.
func Resolve(
	ctx context.Context,
	git Engine,
	repoPath string,
	branch string,
	crowbarHome string,
) (Outcome, error) {
	_ = git.WorktreePrune(ctx, repoPath)
	entries, err := git.WorktreeList(ctx, repoPath)
	if err != nil {
		return Outcome{}, err
	}
	for _, e := range entries {
		if e.Branch != branch {
			continue
		}
		switch {
		case samePath(e.Path, repoPath):
			return Outcome{Kind: HeldByHome, HeldByPath: e.Path}, nil
		case isUnder(e.Path, crowbarHome):
			return Outcome{Kind: HeldByManaged, HeldByPath: e.Path}, nil
		default:
			return Outcome{Kind: HeldByExternal, HeldByPath: e.Path}, nil
		}
	}
	return Outcome{Kind: Free}, nil
}

// samePath reports whether two paths refer to the same location, resolving
// symlinks first (git worktree list emits fully-resolved paths, e.g. macOS
// /var -> /private/var), matching the codebase's other holder checks.
func samePath(a string, b string) bool {
	return resolvePath(a) == resolvePath(b)
}

func resolvePath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
}

// isUnder reports whether path is at or below root (symlink-resolved).
func isUnder(path string, root string) bool {
	if root == "" {
		return false
	}
	rp := resolvePath(path)
	rr := resolvePath(root)
	return rp == rr || strings.HasPrefix(rp, rr+string(filepath.Separator))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./internal/app/usecases/internal/holder/`
Expected: PASS.

> Note: `TestResolve_HeldByManaged` / `TestResolve_HeldByExternal` use non-existent paths, so `filepath.EvalSymlinks` fails and falls back to `filepath.Clean` — the string comparison still classifies correctly.

- [ ] **Step 5: Verify suite still green**

Run: `cd api && go test ./...`
Expected: PASS.

---

## Task 6: Import path — home-in-place + holder-resolved placeholder provisioning

**Files:**
- Modify: `api/internal/app/usecases/project/project_import.go` — `ImportGitEngine` interface (add `WorktreePrune`), `adoptRepoHome` (stop detach), `importOneRepo` call site, `provisionProtectedBranchWorktree` (holder.Resolve + placeholder), `addProtectedWorktree` (best-effort FF), delete `toSet`
- Modify: `api/internal/app/usecases/mocks/mocks.go` — add `WorktreePrune` to `GitEngine`; copy `HeldByPath` in `WorkspaceRepo.Create`
- Modify (existing tests): `api/internal/app/usecases/project/project_import_test.go` — `TestImport_CreatesProjectReposAndAdoptsWorktrees`, `TestImport_SkipsNonProtectedLocalWorktrees`, `TestImport_ProvisionsManagedWorktreesForProtectedBranches`, `TestImport_DefaultProtectedBranch_HomeDetachedAndManaged` (renamed → `..._HomeInPlaceAndPlaceholder`), `TestImport_HomeRowFailureAfterDetach_ReattachesRepo` (renamed → `TestImport_HomeRowFailure_NoDetachNoReattach`), `TestImportRepo_AdoptsDefaultBranchWorkspace`
- Test (new cases): append to `api/internal/app/usecases/project/project_import_test.go`

**Interfaces:**
- Consumes: `holder.Resolve`, `holder.HeldByHome/HeldByExternal/HeldByManaged/Free`, `workspace.CreateInput` (with `HeldByPath`), `worktreepath.For`, the widened `ImportGitEngine`.
- Produces: `ImportGitEngine.WorktreePrune(ctx, repoPath) error`; a placeholder `domain.Workspace` (`Status==locked`, `WorktreePath==""`, `HeldByPath` set) persisted via `WorkspaceCreator.Create` for every live-held protected branch. `adoptRepoHome(ctx, repo domain.Repository) error` (parameter `locked` dropped).

- [ ] **Step 1: Write the failing tests (new behavior)**

Append to `api/internal/app/usecases/project/project_import_test.go`:

```go
// TestImport_HomeHeldProtectedBranch_YieldsSinglePlaceholder is the exact
// char2cs/asynx case: home on develop, protected develop+master. The home is
// adopted IN PLACE (not detached), develop becomes exactly ONE placeholder
// (locked, empty WorktreePath, heldByPath == repoPath), master (unheld) becomes
// a managed worktree — never two develop rows (spec §3.5/B5).
func TestImport_HomeHeldProtectedBranch_YieldsSinglePlaceholder(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "develop", Head: "h1"}}
	prov.Protected = []string{"develop", "master"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	assert.Empty(t, git.Detached, "the repo home is NOT detached; it is adopted in place")

	var developRows, placeholders, managed []domain.Workspace
	for _, w := range ws.Created {
		if w.Branch == "develop" && !w.IsDefault {
			developRows = append(developRows, w)
		}
		if w.Status == domain.WorkspaceStatusLocked && w.WorktreePath == "" {
			placeholders = append(placeholders, w)
		}
		if w.Status == domain.WorkspaceStatusLocked && w.WorktreePath != "" {
			managed = append(managed, w)
		}
	}
	require.Len(t, developRows, 1, "the home-held protected branch yields exactly ONE develop row")
	require.Len(t, placeholders, 1, "develop is a single placeholder")
	assert.Equal(t, "develop", placeholders[0].Branch)
	assert.Equal(t, "/repoA", placeholders[0].HeldByPath, "placeholder records the home as holder")
	require.Len(t, managed, 1, "master (unheld) gets a managed worktree")
	assert.Equal(t, "master", managed[0].Branch)
}

// TestImport_ExternalHolder_YieldsPlaceholder: a protected branch held by a live
// worktree OUTSIDE the crowbar home becomes a placeholder recording that path.
func TestImport_ExternalHolder_YieldsPlaceholder(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
		{Path: "/some/external/wt", Branch: "release", Head: "h2"},
	}
	prov.Protected = []string{"release"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	var placeholder *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].Branch == "release" {
			placeholder = &ws.Created[i]
		}
	}
	require.NotNil(t, placeholder)
	assert.Equal(t, domain.WorkspaceStatusLocked, placeholder.Status)
	assert.Empty(t, placeholder.WorktreePath)
	assert.Equal(t, "/some/external/wt", placeholder.HeldByPath)
}

// TestImport_DeadRegistrationPruned_ProvisionsCleanly: a protected branch whose
// only "holder" is a dead worktree registration is freed by the prune-before in
// holder.Resolve and provisions a managed worktree — never dropped. The mock
// GitEngine drops dead regs on WorktreePrune (see step 3).
func TestImport_DeadRegistrationPruned_ProvisionsCleanly(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}
	// A dead registration: develop registered at a now-deleted dir; prune reaps it.
	git.DeadRegistrations = map[string]string{"/deleted/wt-develop": "develop"}
	prov.Protected = []string{"develop"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	var managed *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].Branch == "develop" {
			managed = &ws.Created[i]
		}
	}
	require.NotNil(t, managed, "develop is provisioned, not dropped")
	assert.NotEmpty(t, managed.WorktreePath, "develop got a managed worktree after prune freed it")
	assert.Contains(t, git.Pruned, "/repoA", "prune ran before provisioning")
}

// TestImport_ParentFetchFails_ProvisionsFromLocalTip: FastForwardBranch failing
// (offline / refused) must NOT skip the branch — the worktree is added from the
// local tip (best-effort FF, matching addWorktree).
func TestImport_ParentFetchFails_ProvisionsFromLocalTip(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "main", Head: "h1"}}
	git.RemoteBranches = map[string]bool{"develop": true}
	git.FastForwardErr = errors.New("fatal: refusing to fetch")
	prov.Protected = []string{"develop"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	var managed *domain.Workspace
	for i := range ws.Created {
		if ws.Created[i].Branch == "develop" {
			managed = &ws.Created[i]
		}
	}
	require.NotNil(t, managed, "develop is provisioned despite the FF failure")
	assert.NotEmpty(t, managed.WorktreePath)
}
```

> The classification keys on `Status == locked` + `WorktreePath` (empty ⇒ placeholder, set ⇒ managed) — `domain.Workspace` has no `Protected()` method.

- [ ] **Step 2: Extend the mock GitEngine and WorkspaceRepo**

In `api/internal/app/usecases/mocks/mocks.go`, extend the `GitEngine` struct fields (after `WorktreeAddErrByBranch` at line 195):

```go
	WorktreeAddErrByBranch map[string]error
	// Pruned records repo paths WorktreePrune was called on.
	Pruned []string
	// DeadRegistrations maps a dead worktree dir -> branch it "holds"; WorktreePrune
	// removes them and merges the survivors into the list holder.Resolve sees.
	DeadRegistrations map[string]string
	// FastForwardErr forces FastForwardBranch to fail (best-effort FF path).
	FastForwardErr error
```

Change `WorktreeList` to fold in not-yet-pruned dead registrations, and `FastForwardBranch` to honor the error. Replace the existing `WorktreeList` body (lines 209-220) with:

```go
func (g *GitEngine) WorktreeList(
	ctx context.Context,
	repoPath string,
) ([]gitengine.WorktreeEntry, error) {
	if g.WorktreeListFn != nil {
		return g.WorktreeListFn(repoPath)
	}
	if g.WorktreeListErr != nil {
		return nil, g.WorktreeListErr
	}
	out := append([]gitengine.WorktreeEntry(nil), g.Worktrees...)
	for path, branch := range g.DeadRegistrations {
		out = append(out, gitengine.WorktreeEntry{Path: path, Branch: branch})
	}
	return out, nil
}
```

Replace `FastForwardBranch` (lines 271-278) with:

```go
func (g *GitEngine) FastForwardBranch(
	ctx context.Context,
	repoPath string,
	branch string,
) error {
	if g.FastForwardErr != nil {
		return g.FastForwardErr
	}
	g.FastForwardedBranches = append(g.FastForwardedBranches, branch)
	return nil
}
```

Add a `WorktreePrune` method (place near `WorktreeRemove`):

```go
func (g *GitEngine) WorktreePrune(
	ctx context.Context,
	repoPath string,
) error {
	g.Pruned = append(g.Pruned, repoPath)
	g.DeadRegistrations = nil // prune reaps every dead registration
	return nil
}
```

In `WorkspaceRepo.Create`, carry `HeldByPath` so placeholder tests can assert it. In the default stub-built `ws` literal (after `Kind: in.Kind,` at line 166), add:

```go
		Kind:          in.Kind,
		HeldByPath:    in.HeldByPath,
		CreatedAt:     now,
```

- [ ] **Step 3: Add `WorktreePrune` to the `ImportGitEngine` interface**

In `api/internal/app/usecases/project/project_import.go`, add to the `ImportGitEngine` interface after `WorktreeAdd` (after line 127):

```go
	// WorktreePrune reaps worktree registrations whose on-disk directory is gone
	// (`git worktree prune`), so a branch held only by a deleted worktree dir is
	// freed before provisioning (the rm -rf ~/.crowbar case). Satisfied by the
	// container's engine (spec §5/B4).
	WorktreePrune(
		ctx context.Context,
		repoPath string,
	) error
```

- [ ] **Step 4: Stop force-detaching in `adoptRepoHome`; delete `toSet`**

Replace `adoptRepoHome` (lines 454-505) with:

```go
// adoptRepoHome adopts the repo's main worktree (repo.Path) as the special
// default workspace, IN PLACE on whatever branch it currently sits on. Crowbar
// never runs git on this directory — it is the user-facing "home" that carries
// chats/threads — so it is created non-protected. The home is NO LONGER
// force-detached off a protected branch: that branch is materialised as a
// placeholder by provisionProtectedWorktrees (the single owner), whose
// holder.Resolve returns held-by-home for it, and freed only later with user
// consent via the Detach-holder op (spec §3.5). The home is the one essential
// workspace; its failure rolls the repo back.
func (u *projectImport) adoptRepoHome(
	ctx context.Context,
	repo domain.Repository,
) error {
	worktrees, err := u.deps.Git.WorktreeList(ctx, repo.Path)
	if err != nil {
		return fmt.Errorf("project import: list worktrees: %w", err)
	}
	in := workspace.CreateInput{
		ID:           uuid.NewString(),
		RepoID:       repo.ID,
		ProjectID:    repo.ProjectID,
		Branch:       mainWorktreeBranch(worktrees, repo.Path),
		WorktreePath: repo.Path,
		IsDefault:    true,
		// ForkPointSha stays empty and Protected stays false: the home is the base
		// the branch tree hangs off, and Crowbar does not operate on it.
	}
	if _, err := u.deps.Workspaces.Create(ctx, in, u.deps.Now()); err != nil {
		return fmt.Errorf("project import: adopt repo home: %w", err)
	}
	return nil
}
```

Update the call site in `importOneRepo` (line 387):

```go
	if err := u.adoptRepoHome(ctx, repo); err != nil {
		return domain.Repository{}, err
	}
```

Delete the now-unused `toSet` helper (lines 679-687).

- [ ] **Step 5: Route live holders to placeholders; make the parent FF best-effort**

Replace `provisionProtectedBranchWorktree` (lines 571-602) with:

```go
// provisionProtectedBranchWorktree materialises one protected branch. It first
// resolves who holds the branch (pruning dead registrations): a live holder
// Crowbar may not seize without consent (the repo home, or an external worktree)
// yields a PLACEHOLDER row (locked, empty WorktreePath, HeldByPath = holder);
// an already-managed holder is skipped; a free branch gets its own managed
// worktree. This is the SINGLE owner of placeholder creation (spec §3.2/§3.5).
func (u *projectImport) provisionProtectedBranchWorktree(
	ctx context.Context,
	repo domain.Repository,
	branch string,
	crowbarHome string,
) error {
	outcome, err := holder.Resolve(ctx, u.deps.Git, repo.Path, branch, crowbarHome)
	if err != nil {
		return fmt.Errorf("resolve holder for %q: %w", branch, err)
	}
	switch outcome.Kind {
	case holder.HeldByManaged:
		// Already represented by a managed workspace — never double-provision.
		return nil
	case holder.HeldByHome, holder.HeldByExternal:
		return u.createPlaceholderWorkspace(ctx, repo, branch, outcome.HeldByPath)
	}
	// Free: provision the managed worktree.
	wsID := uuid.NewString()
	path := worktreepath.For(crowbarHome, repo.ProjectID, repo.ID, wsID)
	startSha, err := u.addProtectedWorktree(ctx, repo, branch, path)
	if err != nil {
		return err
	}
	in := workspace.CreateInput{
		ID:           wsID,
		RepoID:       repo.ID,
		ProjectID:    repo.ProjectID,
		Branch:       branch,
		WorktreePath: path,
		ForkPointSha: startSha,
		Protected:    true,
	}
	if _, err := u.deps.Workspaces.Create(ctx, in, u.deps.Now()); err != nil {
		// The row failed after the worktree was created on disk — remove the
		// orphaned worktree so a later retry can recreate it cleanly.
		if rmErr := u.deps.Git.WorktreeRemove(ctx, repo.Path, path); rmErr != nil {
			slog.WarnContext(ctx, "project import: failed to clean up orphaned worktree",
				"path", path, "error", rmErr)
		}
		return fmt.Errorf("create protected workspace row: %w", err)
	}
	return nil
}

// createPlaceholderWorkspace persists a locked, worktree-less row recording the
// live holder of a protected branch, so the branch is never silently dropped and
// the user gets a visible, retryable surface (spec §3.3). No LastError is written
// — the FE reconstructs the reason from HeldByPath (spec §4/B7).
func (u *projectImport) createPlaceholderWorkspace(
	ctx context.Context,
	repo domain.Repository,
	branch string,
	heldByPath string,
) error {
	in := workspace.CreateInput{
		ID:         uuid.NewString(),
		RepoID:     repo.ID,
		ProjectID:  repo.ProjectID,
		Branch:     branch,
		Protected:  true, // seeds locked; keeps every protection guard for free (B1)
		HeldByPath: heldByPath,
		// WorktreePath + ForkPointSha stay empty — this is the placeholder signal.
	}
	if _, err := u.deps.Workspaces.Create(ctx, in, u.deps.Now()); err != nil {
		return fmt.Errorf("create placeholder workspace for %q: %w", branch, err)
	}
	return nil
}
```

Replace `addProtectedWorktree` (lines 608-630) — make the FF best-effort:

```go
// addProtectedWorktree checks branch out into a fresh worktree at path and
// returns the branch tip SHA (the workspace's fork point). The parent fetch is
// BEST-EFFORT (matching addWorktree): a refused fetch (a dead reg still
// "holding" the branch, or an offline remote) must NOT skip the branch — branch
// from the local tip instead (spec §3.2). WorktreeAdd already prunes-and-retries
// the stale "already used by worktree" conflict internally.
func (u *projectImport) addProtectedWorktree(
	ctx context.Context,
	repo domain.Repository,
	branch string,
	path string,
) (string, error) {
	if exists, err := u.deps.Git.RemoteBranchExists(ctx, repo.Path, branch); err == nil && exists {
		if err := u.deps.Git.FastForwardBranch(ctx, repo.Path, branch); err != nil {
			slog.WarnContext(ctx, "project import: could not fast-forward protected branch; branching from local tip",
				"repo", repo.Name, "branch", branch, "error", err)
		}
	}
	if err := u.deps.Git.WorktreeAdd(ctx, repo.Path, path, branch); err != nil {
		return "", fmt.Errorf("add worktree for protected branch %q: %w", branch, err)
	}
	// Resolve the BRANCH head explicitly: `git rev-parse <name>` resolves a tag of
	// the same name before the branch, which would record a wrong fork point.
	sha, err := u.deps.Git.RevParse(ctx, repo.Path, "refs/heads/"+branch)
	if err != nil {
		return "", nil // fork point is non-essential; the worktree is valid
	}
	return sha, nil
}
```

Add the `holder` import to the import block (after the `worktreepath` import at line 20):

```go
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/holder"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
```

- [ ] **Step 6: Update the two existing import tests to the new intended behavior**

In `TestImport_CreatesProjectReposAndAdoptsWorktrees`, the fixture home sits on `main` and `main` is protected — under the new model the home stays on `main` and `main` becomes a placeholder. Rework it so a protected branch is free (→ managed) AND the home-held branch is a placeholder. Replace the fixture + assertions (lines 86-132) with:

```go
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
		{Path: "/repoA/wt-feature", Branch: "feature", Head: "h2"},
	}
	git.MergeBaseSha = "forksha"
	// main is the home's branch (held-by-home → placeholder); develop is remote-only
	// and unheld (→ managed worktree).
	git.RemoteBranches = map[string]bool{"develop": true}
	prov.Protected = []string{"main", "develop"}

	project, err := uc.Import(context.Background(), "My Project", "/root")
	require.NoError(t, err)

	assert.Equal(t, "My Project", project.Name)
	assert.Equal(t, "/root", project.Path)
	assert.Len(t, projects.Saved, 1)

	require.Len(t, repos.Saved, 1)
	repo := repos.Saved[0]
	assert.Equal(t, "repoA", repo.Name)
	assert.Equal(t, project.ID, repo.ProjectID)
	assert.Equal(t, "main", repo.DefaultBranch)

	// project home (Kind=home) + repo home (IsDefault, stays on main, NOT detached)
	// + a main PLACEHOLDER (held by the home) + a develop MANAGED worktree.
	require.Len(t, ws.Created, 4)
	assert.Equal(t, domain.WorkspaceKindHome, ws.Created[0].Kind)

	home := ws.Created[1]
	assert.True(t, home.IsDefault, "the repo home is the default workspace")
	assert.Equal(t, "/repoA", home.WorktreePath, "the repo home stays the repo folder")
	assert.Equal(t, "main", home.Branch, "the repo home stays on its branch (NOT detached)")
	assert.NotEqual(t, domain.WorkspaceStatusLocked, home.Status)
	assert.Empty(t, git.Detached, "the repo home is never force-detached")

	var placeholder, managed domain.Workspace
	for _, w := range ws.Created {
		if w.Branch == "main" && !w.IsDefault {
			placeholder = w
		}
		if w.Branch == "develop" {
			managed = w
		}
	}
	assert.Equal(t, domain.WorkspaceStatusLocked, placeholder.Status)
	assert.Empty(t, placeholder.WorktreePath, "the home-held branch is a placeholder")
	assert.Equal(t, "/repoA", placeholder.HeldByPath)

	assert.Equal(t, domain.WorkspaceStatusLocked, managed.Status)
	assert.NotEqual(t, "/repoA", managed.WorktreePath)
	assert.Contains(t, managed.WorktreePath, "/crowbar-home/projects/")
	assert.Contains(t, managed.WorktreePath, "/worktree")
```

In `TestImport_SkipsNonProtectedLocalWorktrees`, the home sits on `develop` (protected). Under the new model `develop`→placeholder, `main`→managed. Replace the assertion block (lines 152-169) with:

```go
	// develop is the home's branch (held-by-home → placeholder); main is unheld
	// (→ managed). Non-protected local worktrees are still not imported.
	byBranch := map[string]domain.Workspace{}
	for _, w := range ws.Created {
		if w.Branch != "" && !w.IsDefault {
			byBranch[w.Branch] = w
		}
	}
	require.Contains(t, byBranch, "develop")
	require.Contains(t, byBranch, "main")
	assert.NotContains(t, byBranch, "feature/x", "non-protected local worktree is NOT imported")
	assert.NotContains(t, byBranch, "spike/y")
	assert.NotContains(t, byBranch, "worktree-agent-abc")

	assert.Equal(t, domain.WorkspaceStatusLocked, byBranch["develop"].Status)
	assert.Empty(t, byBranch["develop"].WorktreePath, "the home-held develop is a placeholder")
	assert.Equal(t, "/repoA", byBranch["develop"].HeldByPath)

	assert.Equal(t, domain.WorkspaceStatusLocked, byBranch["main"].Status)
	assert.NotEqual(t, "/repoA", byBranch["main"].WorktreePath, "unheld main gets a managed worktree")

	assert.Len(t, ws.Created, 4, "project home + repo home + develop placeholder + main managed")
```

Four OTHER existing unit tests in this file (package `project_test`, compiled by plain `go test ./...`) still assert the removed force-detach / empty-home-branch behavior and MUST be moved to the new model here — otherwise `go test ./...` at Step 8 (and every later checkpoint) is red.

Replace `TestImport_ProvisionsManagedWorktreesForProtectedBranches` (lines 528-557) with — home on protected `main` (held-by-home → placeholder), `develop` unheld (→ managed):

```go
// TestImport_ProvisionsManagedWorktreesForProtectedBranches: home on protected
// main, develop also protected but unheld. Under the recovery model the home is
// adopted IN PLACE on main (never detached), main becomes exactly ONE placeholder
// (locked, empty WorktreePath, heldByPath == repo folder), and develop (free) gets
// its own Crowbar-managed locked worktree.
func TestImport_ProvisionsManagedWorktreesForProtectedBranches(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)

	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
	}
	prov.Protected = []string{"main", "develop"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	require.Len(t, ws.Created, 4, "project home + repo home + main placeholder + develop managed worktree")
	assert.Empty(t, git.Detached, "the repo home is adopted in place, never force-detached")

	byBranch := map[string]domain.Workspace{}
	for _, w := range ws.Created {
		if w.Branch != "" && !w.IsDefault {
			byBranch[w.Branch] = w
		}
	}
	require.Contains(t, byBranch, "main")
	require.Contains(t, byBranch, "develop")

	// main is held by the home → a placeholder (locked, no worktree, holder recorded).
	assert.Equal(t, domain.WorkspaceStatusLocked, byBranch["main"].Status, "main is a locked placeholder")
	assert.Empty(t, byBranch["main"].WorktreePath, "the home-held main is a placeholder with no worktree")
	assert.Equal(t, "/repoA", byBranch["main"].HeldByPath, "the placeholder records the home as holder")

	// develop is unheld → its own managed worktree (never a stub at the repo folder).
	assert.Equal(t, domain.WorkspaceStatusLocked, byBranch["develop"].Status, "develop is locked")
	assert.NotEqual(t, "/repoA", byBranch["develop"].WorktreePath, "develop gets a managed worktree, not the repo folder")
	assert.Contains(t, byBranch["develop"].WorktreePath, "/worktree")
	assert.Empty(t, byBranch["develop"].HeldByPath, "a managed worktree has no holder")
}
```

Replace `TestImport_DefaultProtectedBranch_HomeDetachedAndManaged` (lines 559-590) with the renamed, new-model test — home on protected `develop`, adopted in place, `develop` becomes a single placeholder (no managed worktree seized):

```go
// TestImport_DefaultProtectedBranch_HomeInPlaceAndPlaceholder: the checked-out
// default ("develop") is protected. The repo home is adopted IN PLACE (keeps
// develop, never detached) and develop becomes exactly ONE placeholder (locked,
// empty WorktreePath, heldByPath == repo folder) — no duplicate, no stub, no
// managed worktree seized from under the user's live checkout.
func TestImport_DefaultProtectedBranch_HomeInPlaceAndPlaceholder(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)

	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "develop", Head: "h1"},
	}
	prov.Protected = []string{"develop"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, ws.Created, 3, "project home + repo home (in place) + develop placeholder")

	home := ws.Created[1]
	assert.True(t, home.IsDefault)
	assert.Equal(t, "develop", home.Branch, "the repo home keeps develop (adopted in place, not detached)")
	assert.Equal(t, "/repoA", home.WorktreePath)
	assert.Empty(t, git.Detached, "the repo home is never force-detached")

	develop := ws.Created[2]
	assert.Equal(t, "develop", develop.Branch)
	assert.Equal(t, domain.WorkspaceStatusLocked, develop.Status)
	assert.Empty(t, develop.WorktreePath, "the home-held develop is a placeholder, not a managed worktree")
	assert.Equal(t, "/repoA", develop.HeldByPath)

	count := 0
	for _, w := range ws.Created {
		if w.Branch == "develop" && !w.IsDefault {
			count++
		}
	}
	assert.Equal(t, 1, count, "develop appears exactly once as a placeholder (never duplicated)")
}
```

Replace `TestImport_HomeRowFailureAfterDetach_ReattachesRepo` (lines 700-722) with the renamed, new-model test — a home-row failure now performs NO detach and NO reattach (there is nothing to undo because the home is never detached):

```go
// TestImport_HomeRowFailure_NoDetachNoReattach: if the repo home row fails to
// persist, the user's real checkout must be left exactly as it was. Under the
// recovery model the home is adopted IN PLACE (never detached), so there is no
// detach to undo and no reattach to perform. The per-repo failure is tolerated by
// the bulk import.
func TestImport_HomeRowFailure_NoDetachNoReattach(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{{Path: "/repoA", Branch: "develop", Head: "h1"}}
	prov.Protected = []string{"develop"}
	ws.CreateFn = func(_ context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error) {
		if in.IsDefault {
			return domain.Workspace{}, errors.New("home row boom")
		}
		created := domain.Workspace{ID: in.ID, Kind: in.Kind, IsDefault: in.IsDefault}
		ws.Created = append(ws.Created, created)
		return created, nil
	}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err) // the per-repo failure is tolerated by the bulk import
	assert.Empty(t, git.Detached, "the home is adopted in place, so nothing is ever detached")
	assert.Empty(t, git.CheckedOut, "no reattach: there was no detach to undo")
}
```

In `TestImportRepo_AdoptsDefaultBranchWorkspace`, the home sits on protected `develop`; under the new model it is adopted in place and `develop` becomes a placeholder. Replace the assertion block (lines 824-837) with:

```go
	// ImportRepo (add-repo-to-existing-project) creates no project home; it adopts
	// the repo home IN PLACE (keeps develop, not detached) plus develop as a locked
	// PLACEHOLDER (held by the home) — never a stub or a seized managed worktree.
	require.Len(t, ws.Created, 2, "repo home (in place) + develop placeholder")
	home := ws.Created[0]
	assert.True(t, home.IsDefault)
	assert.Equal(t, "develop", home.Branch, "the repo home keeps develop (adopted in place, not detached)")
	assert.Equal(t, repo.ID, home.RepoID)
	assert.Empty(t, git.Detached, "the repo home is never force-detached")

	develop := ws.Created[1]
	assert.Equal(t, "develop", develop.Branch)
	assert.Equal(t, repo.ID, develop.RepoID)
	assert.Equal(t, domain.WorkspaceStatusLocked, develop.Status)
	assert.Empty(t, develop.WorktreePath, "the home-held develop is a placeholder")
	assert.Equal(t, "/root/repoA", develop.HeldByPath, "the placeholder records the home as holder")
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd api && go test ./internal/app/usecases/project/`
Expected: PASS (updated + new import tests).

- [ ] **Step 8: Verify suite still green**

Run: `cd api && go test ./...`
Expected: PASS.

---

## Task 7: Empty-`WorktreePath` parent guards

**Files:**
- Modify: `api/internal/app/usecases/worktree/worktree_errors.go` (add `ErrParentUnprovisioned`, `ErrBranchStillHeld`)
- Modify: `api/internal/app/usecases/worktree/worktree.go` — `guardMerge:474-494`, `guardReparent:755-774`, `RebaseOntoParent:712-753`
- Modify: `api/internal/api/libs/status.go:100-118` (map `ErrParentUnprovisioned` → 409)
- Test: append to `api/internal/app/usecases/worktree/worktree_test.go`

**Interfaces:**
- Consumes: `domain.Workspace`, `worktree.CreateChildInput`, `MergeResult`.
- Produces: `worktree.ErrParentUnprovisioned`, `worktree.ErrBranchStillHeld`; `MergeIntoParent`/`Reparent`/`RebaseOntoParent` reject a placeholder (empty-`WorktreePath`) parent with `ErrParentUnprovisioned` before any `RevParse("", "HEAD")`.

- [ ] **Step 1: Write the failing tests**

Append to `api/internal/app/usecases/worktree/worktree_test.go`:

```go
// TestMergeIntoParent_RejectsUnprovisionedParent proves a placeholder parent
// (locked + empty WorktreePath) is rejected before any git runs — no
// RevParse("", "HEAD"). It is ALSO rejected as locked; the empty-path guard is
// the explicit backstop the spec adds (§3.4/B2).
func TestMergeIntoParent_RejectsUnprovisionedParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw"}
	parent := domain.Workspace{ID: "p", Status: domain.WorkspaceStatusLocked, WorktreePath: ""}
	g := &fakeGit{}
	uc := worktree.New(mergeWS(child, parent, nil), g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.MergeIntoParent(context.Background(), "c", gitdomain.MergeStrategyMerge)
	require.Error(t, err)
	assert.Empty(t, g.calls, "no git runs against an unprovisioned parent")
}

// TestReparent_RejectsUnprovisionedNewParent: reparenting onto a placeholder
// parent is rejected before RevParse.
func TestReparent_RejectsUnprovisionedNewParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "old", Branch: "feat", WorktreePath: "/cw"}
	newParent := domain.Workspace{ID: "np", Status: domain.WorkspaceStatusLocked, WorktreePath: ""}
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "c" {
				return child, nil
			}
			return newParent, nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{child, newParent}, nil
		},
	}
	g := &fakeGit{}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.Reparent(context.Background(), "c", "np")
	require.ErrorIs(t, err, worktree.ErrParentUnprovisioned)
	assert.Empty(t, g.calls)
}

// TestRebaseOntoParent_RejectsUnprovisionedParent: finishing the move against a
// placeholder parent is rejected before RevParse.
func TestRebaseOntoParent_RejectsUnprovisionedParent(t *testing.T) {
	child := domain.Workspace{ID: "c", ParentID: "p", Branch: "feat", WorktreePath: "/cw", ForkPointSha: "f"}
	parent := domain.Workspace{ID: "p", Status: domain.WorkspaceStatusLocked, WorktreePath: ""}
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "c" {
				return child, nil
			}
			return parent, nil
		},
	}
	g := &fakeGit{}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	_, err := uc.RebaseOntoParent(context.Background(), "c")
	require.ErrorIs(t, err, worktree.ErrParentUnprovisioned)
	assert.Empty(t, g.calls)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/app/usecases/worktree/ -run 'Unprovisioned'`
Expected: FAIL — `undefined: worktree.ErrParentUnprovisioned` and (for merge) the current code reaching git via locked check returns `ErrParentLocked`, so `require.ErrorIs(..., ErrParentUnprovisioned)` fails.

- [ ] **Step 3: Add the sentinels**

Append to `api/internal/app/usecases/worktree/worktree_errors.go`:

```go
// ErrParentUnprovisioned is returned when a parent-tip op (merge-into-parent,
// reparent-onto, rebase-onto-parent) targets a placeholder parent — a locked row
// whose WorktreePath is still empty because its protected branch has not been
// materialised yet. The guard runs before any RevParse so no git op can run
// against "". Handlers map it to HTTP 409 (spec §3.4/B2).
var ErrParentUnprovisioned = errors.New("usecases: parent branch is not yet provisioned")

// ErrBranchStillHeld is returned by RetryProvision when the protected branch is
// still held by a live worktree (the repo home or an external checkout) that was
// not freed first: the user must detach the holder before a retry can succeed
// (spec §3.3/§3.7).
var ErrBranchStillHeld = errors.New("usecases: branch is still held; detach the holder first")
```

- [ ] **Step 4: Add the guards**

In `guardMerge` (worktree.go), add before the locked check (line 480):

```go
	if parent.WorktreePath == "" {
		return ErrParentUnprovisioned
	}
	if parent.Status == domain.WorkspaceStatusLocked {
		return ErrParentLocked
	}
```

In `guardReparent`, add after the self-parent check (line 762):

```go
	if child.ID == newParent.ID {
		return ErrSelfParent
	}
	if newParent.WorktreePath == "" {
		return ErrParentUnprovisioned
	}
	if newParent.Status == domain.WorkspaceStatusLocked {
		return ErrNewParentLocked
	}
```

In `RebaseOntoParent`, add after loading `parent`, before the `RevParse` (between line 726 and 727):

```go
	parent, err := u.workspaces.Get(ctx, child.ParentID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("rebase onto parent: get parent: %w", err)
	}
	if parent.WorktreePath == "" {
		return domain.Workspace{}, ErrParentUnprovisioned
	}
	tip, err := u.git.RevParse(ctx, parent.WorktreePath, "HEAD")
```

- [ ] **Step 5: Map the error to 409**

In `api/internal/api/libs/status.go`, add to `isConflict` after the `ErrWorkspaceLocked` clause (line 107):

```go
	if errors.Is(err, apperr.ErrLocked) ||
		errors.Is(err, enginesearch.ErrLocked) ||
		errors.Is(err, worktree.ErrParentLocked) ||
		errors.Is(err, worktree.ErrNewParentLocked) ||
		errors.Is(err, worktree.ErrWorkspaceLocked) ||
		errors.Is(err, worktree.ErrParentUnprovisioned) {
		return true
	}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd api && go test ./internal/app/usecases/worktree/ -run 'Unprovisioned'`
Expected: PASS.

- [ ] **Step 7: Verify suite still green**

Run: `cd api && go test ./...`
Expected: PASS.

---

## Task 8: `removeOne` skips git teardown on an empty worktree path

**Files:**
- Modify: `api/internal/app/usecases/worktree/worktree.go:801-844` (`removeOne`)
- Test: append to `api/internal/app/usecases/worktree/worktree_test.go`

**Interfaces:**
- Consumes: `domain.Workspace`, `DeleteCascade`.
- Produces: `removeOne` drops the read-model row for an empty-`WorktreePath` workspace without running `WorktreeRemove`/`ForceDeleteBranch`/`CheckoutBranch` (defense-in-depth so a placeholder's real branch is never touched even if it were ever reachable by a delete).

- [ ] **Step 1: Write the failing test**

Append to `api/internal/app/usecases/worktree/worktree_test.go`:

```go
// TestRemoveOne_PlaceholderSkipsGitTeardown proves a placeholder (empty
// WorktreePath) whose branch is a protected NON-default branch is torn down as a
// pure read-model drop: no WorktreeRemove, no ForceDeleteBranch, no CheckoutBranch
// — the real branch is never git-touched (spec §5 defense-in-depth). The
// placeholder is created NON-locked here only to exercise removeOne directly
// (DeleteCascade's locked guard is proven separately by TestDeleteCascade).
func TestRemoveOne_PlaceholderSkipsGitTeardown(t *testing.T) {
	g := &fakeGit{}
	repos := &fakeRepoStore{path: "/repo", defaultBranch: "develop"}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "ph", RepoID: "r1", Branch: "master", WorktreePath: ""},
			}, nil
		},
		DeleteFn: func(_ context.Context, _ string) error { return nil },
	}
	uc := worktree.New(ws, g, &fakeProvider{}, repos, newNow(), fakeHome())

	require.NoError(t, uc.DeleteCascade(context.Background(), "ph"))

	assert.NotContains(t, g.ops(), "WorktreeRemove")
	assert.NotContains(t, g.ops(), "ForceDeleteBranch", "the real protected branch must never be force-deleted")
	assert.NotContains(t, g.ops(), "CheckoutBranch")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/app/usecases/worktree/ -run TestRemoveOne_PlaceholderSkipsGitTeardown`
Expected: FAIL — current `removeOne` runs `ForceDeleteBranch(repoPath, "master")` (master != develop), so the assertion `NotContains ForceDeleteBranch` fails.

- [ ] **Step 3: Add the empty-path short-circuit**

In `removeOne`, after resolving `repoPath` (line 820), add before the `WorktreeRemove` call:

```go
	repoPath := repo.Path
	// A placeholder (empty WorktreePath) has no worktree, no managed branch
	// checkout, and its real branch must never be git-touched: drop the row only.
	// Defense-in-depth — the locked status already blocks DeleteCascade, but a
	// direct removeOne must not run git against "" or -D the protected branch.
	if ws.WorktreePath == "" {
		return u.workspaces.Delete(ctx, ws.ID)
	}
	// Best-effort git teardown: a failure here (branch checked out elsewhere, a
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./internal/app/usecases/worktree/ -run TestRemoveOne_PlaceholderSkipsGitTeardown`
Expected: PASS.

- [ ] **Step 5: Verify suite still green**

Run: `cd api && go test ./...`
Expected: PASS.

---

## Task 9: Refit `CreateChild` holder detection onto the shared helper

**Files:**
- Modify: `api/internal/app/usecases/worktree/worktree.go:179-196` (`CreateChild` error branch), delete `mainWorktreeHoldsBranch:374-389`, delete `samePath:413-415` + `resolvePath:417-422`
- Modify imports: add `holder`

**Interfaces:**
- Consumes: `holder.Resolve`, `holder.HeldByHome`, the usecase's `u.git` (full `enginegit.Engine`, satisfies `holder.Engine`), `home` (resolved crowbar home).
- Produces: unchanged behavior — CreateChild still detaches the main folder only when it (the repo home) holds the branch, then retries once. Existing detach tests (`Contains`-based) stay green.

- [ ] **Step 1: Write the failing test**

Append to `api/internal/app/usecases/worktree/worktree_test.go`:

```go
// TestCreateChild_UsesHolderResolveForDetach proves the detach path goes through
// the shared holder primitive: on the "already used by worktree" conflict it
// prunes (holder.Resolve step 1) and lists, sees the main folder holds the
// branch (held-by-home), detaches, and retries — one unified mechanism (spec §5).
func TestCreateChild_UsesHolderResolveForDetach(t *testing.T) {
	inUse := errors.New("fatal: 'develop' is already used by worktree at '/repo'")
	g := &fakeGit{
		remoteExists:           true,
		revParseSha:            "forksha",
		addConflictUntilDetach: inUse,
		worktrees:              []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}},
	}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "def", RepoID: "r1", Branch: "develop", WorktreePath: "/repo", IsDefault: true},
			}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git",
		Branch: "develop", ParentID: "", ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Contains(t, g.ops(), "WorktreePrune", "holder.Resolve prunes dead regs before classifying")
	assert.Contains(t, g.ops(), "DetachWorktree")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/app/usecases/worktree/ -run TestCreateChild_UsesHolderResolveForDetach`
Expected: FAIL — current CreateChild detaches via `mainWorktreeHoldsBranch` (no `WorktreePrune`), so `Contains "WorktreePrune"` fails.

- [ ] **Step 3: Refit the error branch**

In `CreateChild`, replace the holder-detection block (lines 186-191):

```go
			if held, hErr := u.mainWorktreeHoldsBranch(ctx, in.RepoPath, in.Branch); hErr == nil && held {
				if dErr := u.git.DetachWorktree(ctx, in.RepoPath); dErr == nil {
					detached = true
					startSha, err = u.addWorktree(ctx, in, path)
				}
			}
```

with:

```go
			if outcome, hErr := holder.Resolve(ctx, u.git, in.RepoPath, in.Branch, home); hErr == nil && outcome.Kind == holder.HeldByHome {
				if dErr := u.git.DetachWorktree(ctx, in.RepoPath); dErr == nil {
					detached = true
					startSha, err = u.addWorktree(ctx, in, path)
				}
			}
```

Delete `mainWorktreeHoldsBranch` (lines 374-389). Delete the now-unused `samePath` (lines 413-415) and `resolvePath` (lines 417-422).

Add the `holder` import to `worktree.go` (after the `cascade` import, line 16):

```go
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/cascade"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/holder"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
```

Remove the now-unused `path/filepath` import (line 8) — `samePath`/`resolvePath` were its only users in this file.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd api && go test ./internal/app/usecases/worktree/ -run 'TestCreateChild'`
Expected: PASS (new test + all existing CreateChild detach tests, which assert via `Contains`, stay green).

- [ ] **Step 5: Verify suite still green**

Run: `cd api && go test ./...`
Expected: PASS.

---

## Task 10: `RetryProvision` usecase op

**Files:**
- Modify: `api/internal/app/usecases/worktree/worktree.go` — add `RetryProvision` to the `Usecase` interface (after line 62) + implement + `materializeProtectedWorktree` helper
- Test: append to `api/internal/app/usecases/worktree/worktree_test.go`

**Interfaces:**
- Consumes: `holder.Resolve`, `u.workspaces.Get/ProvisionInPlace`, `u.repos.FindByKey`, `u.crowbarHome`, `worktreepath.For`, `ErrBranchStillHeld`.
- Produces: `Usecase.RetryProvision(ctx context.Context, wsID string) (domain.Workspace, error)` — re-provisions a placeholder in place (same id): materialises the worktree, records the fork point, clears `HeldByPath` (status stays locked); returns `ErrBranchStillHeld` when the branch is still live-held.

- [ ] **Step 1: Write the failing tests**

Append to `api/internal/app/usecases/worktree/worktree_test.go`:

```go
// TestRetryProvision_FreeBranch_ProvisionsInPlace proves Retry on a placeholder
// (free after resolution) materialises a worktree, records the fork point, and
// provisions the SAME id in place (spec §3.3).
func TestRetryProvision_FreeBranch_ProvisionsInPlace(t *testing.T) {
	g := &fakeGit{revParseSha: "forksha", worktrees: nil} // no holder → Free
	var gotID, gotPath, gotSha string
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{ID: id, RepoID: "r1", ProjectID: "p1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo"}, nil
		},
		ProvisionInPlaceFn: func(id, path, sha string) (domain.Workspace, error) {
			gotID, gotPath, gotSha = id, path, sha
			return domain.Workspace{ID: id, WorktreePath: path, ForkPointSha: sha, Status: domain.WorkspaceStatusLocked}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	out, err := uc.RetryProvision(context.Background(), "ph")
	require.NoError(t, err)
	assert.Equal(t, "ph", gotID, "provisions the SAME id in place")
	assert.NotEmpty(t, gotPath)
	assert.Equal(t, "forksha", gotSha)
	assert.Equal(t, domain.WorkspaceStatusLocked, out.Status)
	assert.Contains(t, g.ops(), "WorktreeAdd")
}

// TestRetryProvision_StillHeld_ReturnsError proves a Retry while the branch is
// still held by the home returns ErrBranchStillHeld and does NOT provision.
func TestRetryProvision_StillHeld_ReturnsError(t *testing.T) {
	g := &fakeGit{worktrees: []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}}}
	provisionCalled := false
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{ID: id, RepoID: "r1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo"}, nil
		},
		ProvisionInPlaceFn: func(_, _, _ string) (domain.Workspace, error) {
			provisionCalled = true
			return domain.Workspace{}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.RetryProvision(context.Background(), "ph")
	require.ErrorIs(t, err, worktree.ErrBranchStillHeld)
	assert.False(t, provisionCalled, "no provision while the branch is still held")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/app/usecases/worktree/ -run TestRetryProvision`
Expected: FAIL — `uc.RetryProvision undefined (type worktree.Usecase has no field or method RetryProvision)`.

- [ ] **Step 3: Add to the interface + implement**

In the `Usecase` interface, add after `RebaseOntoParent` (line 62):

```go
	RebaseOntoParent(
		ctx context.Context,
		childID string,
	) (domain.Workspace, error)
	// RetryProvision re-runs holder resolution + provisioning for an existing
	// placeholder (same id): on success it attaches the worktree, records the
	// fork point, and clears HeldByPath (status stays locked). Returns
	// ErrBranchStillHeld when the branch is still live-held (spec §3.3).
	RetryProvision(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	// DetachHolder frees a live holder off the placeholder's branch (with the
	// caller's consent), clears the home row's branch when the holder is the repo
	// home, then Retry-provisions in place (spec §3.5/§3.7).
	DetachHolder(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
```

(The `DetachHolder` interface entry is added now; its implementation lands in Task 11.)

Add the implementation (place after `RebaseOntoParent`, before `guardReparent`):

```go
// RetryProvision re-provisions a placeholder workspace in place (spec §3.3).
func (u *worktreeUsecase) RetryProvision(
	ctx context.Context,
	wsID string,
) (domain.Workspace, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("retry provision: get workspace: %w", err)
	}
	repoPath, err := u.repoPathFor(ctx, ws)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("retry provision: repo path: %w", err)
	}
	home, err := u.crowbarHome()
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("retry provision: crowbar home: %w", err)
	}
	outcome, err := holder.Resolve(ctx, u.git, repoPath, ws.Branch, home)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("retry provision: resolve holder: %w", err)
	}
	if outcome.Kind == holder.HeldByHome || outcome.Kind == holder.HeldByExternal {
		return domain.Workspace{}, fmt.Errorf("%w (%s at %s)", ErrBranchStillHeld, ws.Branch, outcome.HeldByPath)
	}
	path := worktreepath.For(home, ws.ProjectID, ws.RepoID, ws.ID)
	startSha, err := u.materializeProtectedWorktree(ctx, repoPath, ws.Branch, path)
	if err != nil {
		return domain.Workspace{}, err
	}
	provisioned, err := u.workspaces.ProvisionInPlace(ctx, ws.ID, path, startSha)
	if err != nil {
		// The worktree is on disk but the row never landed — clean it up so a
		// later retry isn't blocked by the orphaned worktree.
		if rmErr := u.git.WorktreeRemove(ctx, repoPath, path); rmErr != nil {
			slog.WarnContext(ctx, "retry provision: cleanup worktree after failed provision",
				"worktree", path, "err", rmErr)
		}
		return domain.Workspace{}, fmt.Errorf("retry provision: persist: %w", err)
	}
	return provisioned, nil
}

// materializeProtectedWorktree fast-forwards the protected branch best-effort
// then checks it out into a fresh worktree at path, returning the branch tip SHA.
// Mirrors the import path's addProtectedWorktree.
func (u *worktreeUsecase) materializeProtectedWorktree(
	ctx context.Context,
	repoPath string,
	branch string,
	path string,
) (string, error) {
	if exists, err := u.git.RemoteBranchExists(ctx, repoPath, branch); err == nil && exists {
		if ffErr := u.git.FastForwardBranch(ctx, repoPath, branch); ffErr != nil {
			slog.WarnContext(ctx, "retry provision: could not fast-forward protected branch; using local tip",
				"branch", branch, "err", ffErr)
		}
	}
	if err := u.git.WorktreeAdd(ctx, repoPath, path, branch); err != nil {
		return "", fmt.Errorf("retry provision: worktree add: %w", err)
	}
	sha, err := u.git.RevParse(ctx, repoPath, "refs/heads/"+branch)
	if err != nil {
		return "", nil // fork point non-essential; the worktree is valid
	}
	return sha, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/app/usecases/worktree/ -run TestRetryProvision`
Expected: PASS.

- [ ] **Step 5: Verify suite still green**

Run: `cd api && go test ./...`
Expected: FAIL to COMPILE only if `DetachHolder` interface entry has no impl yet — it does not until Task 11. **Therefore add a temporary compile guard is NOT needed:** the `worktreeUsecase` struct must satisfy `Usecase`. To keep the suite green at this checkpoint, implement `DetachHolder` in this same task's Step 3 as well (its full body is in Task 11 Step 3). Copy the Task 11 `DetachHolder` + `repoHomeWorkspaceID` implementations in now, then run:

Run: `cd api && go test ./...`
Expected: PASS.

> Rationale: adding both interface methods in one edit keeps `worktreeUsecase` a complete `Usecase` at every checkpoint. Task 11 then only adds the DetachHolder-specific tests.

---

## Task 11: `DetachHolder` usecase op (tests)

**Files:**
- Modify: `api/internal/app/usecases/worktree/worktree.go` — `DetachHolder` + `repoHomeWorkspaceID` (implemented in Task 10 Step 5; verified here)
- Test: append to `api/internal/app/usecases/worktree/worktree_test.go`

**Interfaces:**
- Consumes: `holder.Resolve`, `u.git.DetachWorktree`, `u.workspaces.ClearBranch/List`, `RetryProvision`.
- Produces: `Usecase.DetachHolder(ctx, wsID) (domain.Workspace, error)` — detaches the resolved holder (data-safe), clears the home row's branch when the holder is the repo home, then Retry-provisions in place. A detach failure returns cleanly with no partial state (ClearBranch + Retry only run after a clean detach).

The implementation (added in Task 10 Step 5):

```go
// DetachHolder frees a live holder off a placeholder's branch with consent, then
// re-provisions in place. When the holder is the repo home it also clears the
// home row's branch (spec §3.5/§3.7). A detach failure returns cleanly — no
// ClearBranch, no Retry — so there is never partial state.
func (u *worktreeUsecase) DetachHolder(
	ctx context.Context,
	wsID string,
) (domain.Workspace, error) {
	ws, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("detach holder: get workspace: %w", err)
	}
	repoPath, err := u.repoPathFor(ctx, ws)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("detach holder: repo path: %w", err)
	}
	home, err := u.crowbarHome()
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("detach holder: crowbar home: %w", err)
	}
	outcome, err := holder.Resolve(ctx, u.git, repoPath, ws.Branch, home)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("detach holder: resolve holder: %w", err)
	}
	if outcome.Kind == holder.HeldByHome || outcome.Kind == holder.HeldByExternal {
		if dErr := u.git.DetachWorktree(ctx, outcome.HeldByPath); dErr != nil {
			return domain.Workspace{}, fmt.Errorf("detach holder: detach %s: %w", outcome.HeldByPath, dErr)
		}
		if outcome.Kind == holder.HeldByHome {
			homeID, ok, hErr := u.repoHomeWorkspaceID(ctx, ws.RepoID)
			if hErr != nil {
				return domain.Workspace{}, fmt.Errorf("detach holder: find home: %w", hErr)
			}
			if ok {
				if _, cErr := u.workspaces.ClearBranch(ctx, homeID); cErr != nil {
					return domain.Workspace{}, fmt.Errorf("detach holder: clear home branch: %w", cErr)
				}
			}
		}
	}
	return u.RetryProvision(ctx, wsID)
}

// repoHomeWorkspaceID returns the id of the repo's default (home) workspace.
func (u *worktreeUsecase) repoHomeWorkspaceID(
	ctx context.Context,
	repoID string,
) (string, bool, error) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return "", false, err
	}
	for _, w := range all {
		if w.RepoID == repoID && w.IsDefault && w.Status != domain.WorkspaceStatusDeleted {
			return w.ID, true, nil
		}
	}
	return "", false, nil
}
```

- [ ] **Step 1: Write the failing tests**

Append to `api/internal/app/usecases/worktree/worktree_test.go`:

```go
// TestDetachHolder_Home_ClearsBranchThenProvisions proves detaching a home
// holder detaches the repo folder, clears the home row's branch via ClearBranch,
// then provisions the placeholder in place (spec §3.5/B6). The second
// holder.Resolve (inside RetryProvision) sees the freed branch.
func TestDetachHolder_Home_ClearsBranchThenProvisions(t *testing.T) {
	// First Resolve: home holds develop. After DetachWorktree the fake frees it,
	// so RetryProvision's Resolve sees Free.
	g := &fakeGit{
		revParseSha:            "forksha",
		worktrees:              []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}},
		addConflictUntilDetach: nil,
	}
	clearedID := ""
	provisioned := false
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{ID: id, RepoID: "r1", ProjectID: "p1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo"}, nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "home", RepoID: "r1", Branch: "develop", WorktreePath: "/repo", IsDefault: true},
			}, nil
		},
		ClearBranchFn: func(id string) (domain.Workspace, error) {
			clearedID = id
			return domain.Workspace{ID: id}, nil
		},
		ProvisionInPlaceFn: func(id, path, sha string) (domain.Workspace, error) {
			provisioned = true
			return domain.Workspace{ID: id, WorktreePath: path, ForkPointSha: sha, Status: domain.WorkspaceStatusLocked}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.DetachHolder(context.Background(), "ph")
	require.NoError(t, err)
	assert.Contains(t, g.ops(), "DetachWorktree")
	assert.Equal(t, "home", clearedID, "the home row's branch is cleared")
	assert.True(t, provisioned, "the placeholder is provisioned after the detach")
}

// TestDetachHolder_DetachFails_NoPartialState proves a detach failure
// (mid-merge/rebase) surfaces cleanly: no ClearBranch, no provision.
func TestDetachHolder_DetachFails_NoPartialState(t *testing.T) {
	g := &fakeGit{
		worktrees: []enginegit.WorktreeEntry{{Path: "/repo", Branch: "develop"}},
		detachErr: errors.New("fatal: cannot detach while merging"),
	}
	cleared := false
	provisioned := false
	ws := &fakeWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			return domain.Workspace{ID: id, RepoID: "r1", Branch: "develop",
				Status: domain.WorkspaceStatusLocked, HeldByPath: "/repo"}, nil
		},
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{{ID: "home", RepoID: "r1", IsDefault: true}}, nil
		},
		ClearBranchFn:      func(_ string) (domain.Workspace, error) { cleared = true; return domain.Workspace{}, nil },
		ProvisionInPlaceFn: func(_, _, _ string) (domain.Workspace, error) { provisioned = true; return domain.Workspace{}, nil },
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.DetachHolder(context.Background(), "ph")
	require.Error(t, err)
	assert.False(t, cleared, "no ClearBranch after a failed detach")
	assert.False(t, provisioned, "no provision after a failed detach")
}
```

> The `fakeGit.DetachWorktree` in this package already sets `detachCalled=true`; to make the first-Resolve-held / second-Resolve-free flow work, add a small behavior to the fake: when `detachCalled` is true, `WorktreeList` returns no holder. Add to `fakeGit.WorktreeList` (Step 2).

- [ ] **Step 2: Make the fake free the branch after detach**

In `api/internal/app/usecases/worktree/worktree_test.go`, replace `fakeGit.WorktreeList` (lines 202-208) with:

```go
func (f *fakeGit) WorktreeList(
	_ context.Context,
	repoPath string,
) ([]enginegit.WorktreeEntry, error) {
	f.record("WorktreeList", repoPath)
	if f.detachCalled {
		// After a detach the holder is freed: the branch is no longer checked out
		// anywhere, so the next resolution classifies it Free.
		return nil, f.worktreeListErr
	}
	return f.worktrees, f.worktreeListErr
}
```

> This keeps existing CreateChild detach tests green: those assert only that a detach happened and count `WorktreeAdd`; after their single detach the retry's `addWorktree` no longer conflicts (`addConflictUntilDetach` clears on `detachCalled`), and they do not re-list.

- [ ] **Step 3: Run tests to verify they pass**

Run: `cd api && go test ./internal/app/usecases/worktree/ -run 'TestDetachHolder'`
Expected: PASS.

- [ ] **Step 4: Verify suite still green**

Run: `cd api && go test ./...`
Expected: PASS.

---

## Task 12: REST endpoints — Retry + Detach holder

**Files:**
- Modify: `api/internal/api/v0/endpoints/workspaces/handlers/handlers.go:42-65` (`Hierarchy` interface)
- Modify: `api/internal/api/v0/endpoints/workspaces/handlers/hierarchy.go` (add two handlers)
- Modify: `api/internal/api/v0/endpoints/workspaces/routes.go:30-37` (mount routes)
- Modify: `api/internal/api/v0/endpoints/workspaces/handlers/handlers_test.go` (extend `fakeHierarchy`)
- Test: append to `api/internal/api/v0/endpoints/workspaces/handlers/hierarchy_test.go`

**Interfaces:**
- Consumes: `worktree.Usecase.RetryProvision/DetachHolder` (via the `Hierarchy` interface), `libs.WriteAccepted/WriteErr/StatusAndMessage`, `runAsync`, `h.broadcastLastError`.
- Produces: `POST .../workspaces/:wsId/retry-provision` and `POST .../workspaces/:wsId/detach-holder` — both 202 + async, delivering the outcome on the workspace WS stream and a failure as `LastError`.

- [ ] **Step 1: Write the failing test**

Append to `api/internal/api/v0/endpoints/workspaces/handlers/hierarchy_test.go` (uses the existing `newRouter`/`do` helpers + `fakeReader{get, getErr}`; the two new routes are added to `newRouter` in Step 6):

```go
func TestRetryProvision_Returns202(t *testing.T) {
	reader := &fakeReader{get: domain.Workspace{ID: "child"}}
	rec := do(
		newRouter(reader, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/retry-provision",
		"",
	)
	assert.Equal(t, http.StatusAccepted, rec.Code)
}

func TestDetachHolder_Returns202(t *testing.T) {
	reader := &fakeReader{get: domain.Workspace{ID: "child"}}
	rec := do(
		newRouter(reader, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/child/detach-holder",
		"",
	)
	assert.Equal(t, http.StatusAccepted, rec.Code)
}

func TestRetryProvisionMissingWorkspace_4xx(t *testing.T) {
	reader := &fakeReader{getErr: apperr.ErrNotFound}
	rec := do(
		newRouter(reader, &fakeHierarchy{}, &fakeRepos{}),
		http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces/nope/retry-provision",
		"",
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
```

> `apperr` is already imported in the test package for the existing 4xx cases (`TestMergeIntoParentMissingWorkspace_4xx`). `do(router, method, path, body string)` runs the request through the router; an empty body string is fine for these no-body POSTs.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/api/v0/endpoints/workspaces/handlers/ -run 'RetryProvision|DetachHolder'`
Expected: FAIL to compile — `h.RetryProvision`/`h.DetachHolder` undefined, `fakeHierarchy` does not implement the widened `Hierarchy` interface, and `newRouter` does not mount the new routes.

- [ ] **Step 3: Extend the `Hierarchy` interface**

In `handlers.go`, add to the `Hierarchy` interface after `DeleteCascade` (before line 65's `}`):

```go
	DeleteCascade(
		ctx context.Context,
		rootID string,
	) error
	// RetryProvision re-provisions a placeholder workspace in place (spec §3.3).
	RetryProvision(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
	// DetachHolder frees the placeholder's branch from its holder (with consent),
	// then re-provisions in place (spec §3.5/§3.7).
	DetachHolder(
		ctx context.Context,
		wsID string,
	) (domain.Workspace, error)
```

- [ ] **Step 4: Add the two handlers**

Append to `api/internal/api/v0/endpoints/workspaces/handlers/hierarchy.go`:

```go
// RetryProvision handles
// POST /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/retry-provision.
// It validates the workspace exists synchronously (4xx if not), then returns 202
// and re-provisions the placeholder in place in the background. The provisioned
// workspace is delivered on the workspace WebSocket stream; a failure (e.g. the
// branch is still held) surfaces as LastError on the entity.
func (h *Handlers) RetryProvision(
	c *gin.Context,
) {
	id := c.Param("wsId")
	if _, err := h.reader.Get(c.Request.Context(), id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteAccepted(c)
	runAsync(
		c.Request.Context(),
		h.broadcastLastError,
		id,
		func(ctx context.Context) error {
			_, retryErr := h.hierarchy.RetryProvision(ctx, id)
			return retryErr
		},
	)
}

// DetachHolder handles
// POST /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/detach-holder.
// It validates the workspace exists synchronously (4xx if not), then returns 202
// and, in the background, detaches the branch's holder (with the user's consent,
// captured by the modal that fires this call), clears the home row's branch when
// the holder is the repo home, and re-provisions the placeholder in place. A
// failure (e.g. detach blocked mid-merge) surfaces as LastError on the entity.
func (h *Handlers) DetachHolder(
	c *gin.Context,
) {
	id := c.Param("wsId")
	if _, err := h.reader.Get(c.Request.Context(), id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteAccepted(c)
	runAsync(
		c.Request.Context(),
		h.broadcastLastError,
		id,
		func(ctx context.Context) error {
			_, detachErr := h.hierarchy.DetachHolder(ctx, id)
			return detachErr
		},
	)
}
```

- [ ] **Step 5: Mount the routes**

In `routes.go`, add after the `rebase-onto-parent` route (line 37):

```go
	rg.POST("/workspaces/:wsId/rebase-onto-parent", h.RebaseOntoParent)
	rg.POST("/workspaces/:wsId/retry-provision", h.RetryProvision)
	rg.POST("/workspaces/:wsId/detach-holder", h.DetachHolder)
```

- [ ] **Step 6: Extend the test fake + router helper**

In `handlers_test.go`, add stubs to `fakeHierarchy` (after `DeleteCascade`):

```go
func (f *fakeHierarchy) RetryProvision(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (f *fakeHierarchy) DetachHolder(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}
```

In `handlers_test.go`, mount the two new routes in `newRouter`, after the `reparent` route:

```go
	rg.POST("/workspaces/:wsId/reparent", h.Reparent)
	rg.POST("/workspaces/:wsId/retry-provision", h.RetryProvision)
	rg.POST("/workspaces/:wsId/detach-holder", h.DetachHolder)
	return r
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd api && go test ./internal/api/v0/endpoints/workspaces/...`
Expected: PASS.

- [ ] **Step 8: Verify suite still green**

Run: `cd api && go test ./...`
Expected: PASS (the concrete `worktree.Usecase` already satisfies the widened `Hierarchy` interface via Tasks 10-11; the router wiring at `router.go:105-113` compiles unchanged).

---

## Task 13: Frontend — `heldByPath` on the DTO + sidebar + unconditional mapping

**Files:**
- Modify: `web/src/lib/types.ts:71` (add field to `WorkspaceDTO`)
- Modify: `web/src/lib/store/sidebar.ts:52` (add field to `Workspace`)
- Modify: `web/src/lib/store/build-repo-tree.ts:45-65` (`toSidebarWorkspace`)
- Test: extend `web/src/__tests__/lib/store/build-repo-tree.test.ts`

**Interfaces:**
- Consumes: `WorkspaceDTO`, `Workspace`, `toSidebarWorkspace(ws: WorkspaceDTO): Workspace`.
- Produces: `WorkspaceDTO.heldByPath?: string`; `Workspace.heldByPath?: string`; `toSidebarWorkspace` maps `heldByPath: ws.heldByPath ?? ''` unconditionally.

- [ ] **Step 1: Write the failing test**

Add to `web/src/__tests__/lib/store/build-repo-tree.test.ts` (extend the `ws()` helper's `Partial<WorkspaceDTO>` already allows `heldByPath`), inside a suitable `describe`:

```ts
import { toSidebarWorkspace } from '@/lib/store/build-repo-tree'

describe('toSidebarWorkspace heldByPath', () => {
  it('maps heldByPath from the DTO', () => {
    const w = toSidebarWorkspace(ws('w1', 'r1', { heldByPath: '/repo' }))
    expect(w.heldByPath).toBe('/repo')
  })

  it('maps heldByPath to "" when absent (overwrites a stale value on Retry)', () => {
    const w = toSidebarWorkspace(ws('w1', 'r1'))
    expect(w.heldByPath).toBe('')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/lib/store/build-repo-tree.test.ts`
Expected: FAIL — `heldByPath` is not a key on the sidebar `Workspace` type / value is `undefined`.

- [ ] **Step 3: Add the DTO + sidebar fields**

In `web/src/lib/types.ts`, add to `WorkspaceDTO` after `localPath` (line 71):

```ts
  /** On-disk worktree directory for this workspace (e.g. /home/user/project). */
  localPath?: string
  /** Worktree dir holding this branch when the workspace is a placeholder
   *  (locked + no localPath). Absent on healthy workspaces. */
  heldByPath?: string
```

In `web/src/lib/store/sidebar.ts`, add to `Workspace` after `localPath` (line 52):

```ts
  /** On-disk worktree directory, from the backend WorkspaceDTO. */
  localPath?: string
  /** Holder path for a placeholder workspace (locked + no localPath); drives the
   *  reconstructed reason and whether the Detach… action is offered. */
  heldByPath?: string
```

- [ ] **Step 4: Map it unconditionally**

In `web/src/lib/store/build-repo-tree.ts`, add to `toSidebarWorkspace`'s returned object after the `lastError` mapping (line 61):

```ts
    lastError: ws.lastError ?? '',
    // Always present (not conditionally spread): applyWorkspaceDTO merges frames
    // with {...w, ...ws}, so a HeldByPath cleared by a successful Retry (absent
    // under omitempty) must overwrite the stale value rather than linger.
    heldByPath: ws.heldByPath ?? '',
    age: '',
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/lib/store/build-repo-tree.test.ts`
Expected: PASS.

- [ ] **Step 6: Verify suite still green**

Run: `cd web && npx vitest run && npx vite build`
Expected: PASS (all vitest specs) and a successful bundle.

---

## Task 14: Frontend — placeholder helper

**Files:**
- Create: `web/src/lib/workspace/placeholder.ts`
- Create test: `web/src/__tests__/lib/workspace/placeholder.test.ts`

**Interfaces:**
- Consumes: `Workspace` (from `@/lib/store/sidebar`).
- Produces:
  - `isPlaceholderWorkspace(ws: Workspace): boolean` → `ws.status === 'locked' && !ws.localPath`.
  - `placeholderReason(ws: Workspace): string` → reconstructed from `branch` + `heldByPath`.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/lib/workspace/placeholder.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { isPlaceholderWorkspace, placeholderReason } from '@/lib/workspace/placeholder'
import type { Workspace } from '@/lib/store/sidebar'

const ws = (over: Partial<Workspace> = {}): Workspace => ({
  id: 'w1',
  branch: 'develop',
  age: '',
  ...over,
})

describe('isPlaceholderWorkspace', () => {
  it('is true for a locked workspace with no localPath', () => {
    expect(isPlaceholderWorkspace(ws({ status: 'locked', heldByPath: '/repo' }))).toBe(true)
  })
  it('is false for a locked workspace that has a localPath (healthy managed)', () => {
    expect(isPlaceholderWorkspace(ws({ status: 'locked', localPath: '/managed' }))).toBe(false)
  })
  it('is false for a non-locked workspace', () => {
    expect(isPlaceholderWorkspace(ws({ status: 'new' }))).toBe(false)
  })
})

describe('placeholderReason', () => {
  it('names the branch and the holder path when known', () => {
    const reason = placeholderReason(ws({ status: 'locked', heldByPath: '/Users/me/repo' }))
    expect(reason).toContain('develop')
    expect(reason).toContain('/Users/me/repo')
  })
  it('falls back to a generic reason without a holder path', () => {
    const reason = placeholderReason(ws({ status: 'locked' }))
    expect(reason).toContain('develop')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/lib/workspace/placeholder.test.ts`
Expected: FAIL — cannot resolve `@/lib/workspace/placeholder`.

- [ ] **Step 3: Write the helper**

Create `web/src/lib/workspace/placeholder.ts`:

```ts
import type { Workspace } from '@/lib/store/sidebar'

/**
 * A placeholder workspace is a protected branch that could not get a managed
 * worktree because a live worktree holds its branch. It is a `locked` row with
 * no on-disk worktree (spec §3.3). The status stays `locked` so every protection
 * guard applies; the placeholder is identified by the missing localPath.
 */
export function isPlaceholderWorkspace(ws: Workspace): boolean {
  return ws.status === 'locked' && !ws.localPath
}

/**
 * Reconstruct the human-readable reason a placeholder exists, from the branch
 * name + heldByPath (there is no persisted lastError — spec §3.3/§4/B7).
 */
export function placeholderReason(ws: Workspace): string {
  if (ws.heldByPath) {
    return `\`${ws.branch}\` is checked out at ${ws.heldByPath} — detach it to let Crowbar manage this branch.`
  }
  return `Crowbar couldn't set up \`${ws.branch}\`. Retry to provision it.`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/lib/workspace/placeholder.test.ts`
Expected: PASS.

- [ ] **Step 5: Verify suite still green**

Run: `cd web && npx vitest run && npx vite build`
Expected: PASS.

---

## Task 15: Frontend — placeholder warning glyph

**Files:**
- Modify: `web/src/components/layout/workspace-branch-icon.tsx:13-58`
- Create test: `web/src/__tests__/components/layout/workspace-branch-icon.test.tsx`

**Interfaces:**
- Consumes: `WorkspaceStatus`, `@phosphor-icons/react` `Warning`, `Lock`.
- Produces: `WorkspaceBranchIcon({ status, working, isPlaceholder }: { status: WorkspaceStatus; working?: boolean; isPlaceholder?: boolean })` — renders a labeled `Warning` glyph when `isPlaceholder` (ahead of the `locked → Lock` case); the `status` switch + `_exhaustive` check are unchanged.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/layout/workspace-branch-icon.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { WorkspaceBranchIcon } from '@/components/layout/workspace-branch-icon'

describe('WorkspaceBranchIcon placeholder', () => {
  it('renders the warning glyph (not the lock glyph) for a placeholder', () => {
    render(<WorkspaceBranchIcon status="locked" isPlaceholder />)
    expect(screen.getByRole('img', { name: /needs provisioning/i })).toBeInTheDocument()
  })

  it('renders the lock glyph for a healthy locked workspace', () => {
    render(<WorkspaceBranchIcon status="locked" />)
    expect(screen.queryByRole('img', { name: /needs provisioning/i })).toBeNull()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/components/layout/workspace-branch-icon.test.tsx`
Expected: FAIL — `isPlaceholder` is not a prop; no labeled warning glyph rendered.

- [ ] **Step 3: Add the prop + glyph**

In `web/src/components/layout/workspace-branch-icon.tsx`, replace the props interface (lines 13-17) and the top of the component (lines 19-23):

```tsx
interface WorkspaceBranchIconProps {
  status: WorkspaceStatus
  /** True while an agent/long-running op is in flight — renders the spinner. */
  working?: boolean
  /** True for a placeholder (locked + no localPath) — renders the warning glyph
   *  ahead of the locked→Lock case (spec §3.3). */
  isPlaceholder?: boolean
}

export function WorkspaceBranchIcon({ status, working, isPlaceholder }: WorkspaceBranchIconProps) {
  // `working` is the §5 in-flight flag that replaced the old 'agent-running'
  // status overlay; it shows the spinner regardless of the underlying status.
  if (working) return <WorkspaceAgentSpinner />

  // A placeholder is a locked row, but it needs the user's attention rather than
  // the "protected, immutable" lock: render the warning glyph ahead of the switch.
  if (isPlaceholder) {
    return (
      <Warning
        role="img"
        aria-label="Branch needs provisioning"
        className="size-4 shrink-0 text-amber-500"
        weight="fill"
      />
    )
  }

  switch (status) {
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/components/layout/workspace-branch-icon.test.tsx`
Expected: PASS.

- [ ] **Step 5: Verify suite still green**

Run: `cd web && npx vitest run && npx vite build`
Expected: PASS.

---

## Task 16: Frontend — Retry / Detach API calls

**Files:**
- Modify: `web/src/lib/api/workspace.ts`
- Create test: `web/src/__tests__/lib/api/workspace.test.ts`

**Interfaces:**
- Consumes: `apiFetch` (`@/lib/api`), `workspaceBase` (`@/lib/workspace-scope-url`).
- Produces: `retryProvision(wsId: string): Promise<void>`, `detachHolder(wsId: string): Promise<void>` — POST to `${workspaceBase(wsId)}/retry-provision` and `/detach-holder`.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/lib/api/workspace.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/lib/api', () => ({ apiFetch: vi.fn().mockResolvedValue(undefined), API_BASE: '' }))
vi.mock('@/lib/workspace-scope-url', () => ({
  workspaceBase: (id: string) => `/v0/projects/p/repos/r/workspaces/${id}`,
}))

import { apiFetch } from '@/lib/api'
import { retryProvision, detachHolder } from '@/lib/api/workspace'

beforeEach(() => vi.clearAllMocks())

describe('retryProvision', () => {
  it('POSTs to the retry-provision route', async () => {
    await retryProvision('w1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/projects/p/repos/r/workspaces/w1/retry-provision', {
      method: 'POST',
    })
  })
})

describe('detachHolder', () => {
  it('POSTs to the detach-holder route', async () => {
    await detachHolder('w1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/projects/p/repos/r/workspaces/w1/detach-holder', {
      method: 'POST',
    })
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/lib/api/workspace.test.ts`
Expected: FAIL — `retryProvision`/`detachHolder` are not exported.

- [ ] **Step 3: Add the calls**

Append to `web/src/lib/api/workspace.ts`:

```ts
/**
 * Retry provisioning a placeholder workspace in place (§3.3, 202 Accepted). On
 * success the backend attaches the worktree and the WS broadcast reflects the
 * now-managed row; a failure (branch still held) surfaces as LastError.
 */
export async function retryProvision(wsId: string): Promise<void> {
  await apiFetch(`${workspaceBase(wsId)}/retry-provision`, { method: 'POST' })
}

/**
 * Detach the holder off a placeholder's branch with the user's consent, then
 * re-provision in place (§3.5/§3.7, 202 Accepted). The outcome rides the WS
 * broadcast; a detach blocked mid-operation surfaces as LastError.
 */
export async function detachHolder(wsId: string): Promise<void> {
  await apiFetch(`${workspaceBase(wsId)}/detach-holder`, { method: 'POST' })
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/lib/api/workspace.test.ts`
Expected: PASS.

- [ ] **Step 5: Verify suite still green**

Run: `cd web && npx vitest run && npx vite build`
Expected: PASS.

---

## Task 17: Frontend — detach-modal store

**Files:**
- Create: `web/src/features/window/stores/detach-modal-store.ts`
- Create test: `web/src/__tests__/features/window/stores/detach-modal-store.test.ts`

**Interfaces:**
- Consumes: `zustand`.
- Produces: `useDetachModalStore` with `target: DetachTarget | null`, `open(target: DetachTarget): void`, `close(): void`; `DetachTarget = { wsId: string; branch: string; heldByPath: string }`.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/window/stores/detach-modal-store.test.ts`:

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'

beforeEach(() => useDetachModalStore.setState({ target: null }))

describe('detach-modal-store', () => {
  it('opens with a target and closes back to null', () => {
    useDetachModalStore.getState().open({ wsId: 'w1', branch: 'develop', heldByPath: '/repo' })
    expect(useDetachModalStore.getState().target).toEqual({
      wsId: 'w1',
      branch: 'develop',
      heldByPath: '/repo',
    })
    useDetachModalStore.getState().close()
    expect(useDetachModalStore.getState().target).toBeNull()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/features/window/stores/detach-modal-store.test.ts`
Expected: FAIL — cannot resolve `@/features/window/stores/detach-modal-store`.

- [ ] **Step 3: Write the store**

Create `web/src/features/window/stores/detach-modal-store.ts`:

```ts
import { create } from 'zustand'

/** The placeholder whose holder the user is about to detach. */
export interface DetachTarget {
  wsId: string
  branch: string
  heldByPath: string
}

interface DetachModalState {
  target: DetachTarget | null
  open: (target: DetachTarget) => void
  close: () => void
}

// Global UI store (features/window/stores): both the placeholder row's Detach…
// button and the placeholder toast's Fix… action open the single detach modal,
// which is rendered once at the shell level and reads this target.
export const useDetachModalStore = create<DetachModalState>()((set) => ({
  target: null,
  open: (target) => set({ target }),
  close: () => set({ target: null }),
}))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/features/window/stores/detach-modal-store.test.ts`
Expected: PASS.

- [ ] **Step 5: Verify suite still green**

Run: `cd web && npx vitest run && npx vite build`
Expected: PASS.

---

## Task 18: Frontend — detach-holder modal

**Files:**
- Create: `web/src/components/layout/detach-holder-modal.tsx`
- Create test: `web/src/__tests__/components/layout/detach-holder-modal.test.tsx`

**Interfaces:**
- Consumes: `useDetachModalStore` (Task 17), `detachHolder` (Task 16), `@/components/ui/dialog` (`Dialog`, `DialogPortal`, `DialogBackdrop`, `DialogPopup`, `DialogTitle`, `DialogDescription`, `DialogFooter`), `@/components/ui/button` `Button`.
- Produces: `DetachHolderModal()` — renders nothing when `target` is null; otherwise a consent dialog naming `heldByPath`, stating files are safe (disruptive-not-destructive), with Cancel + Detach; Detach calls `detachHolder(target.wsId)` then closes.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/layout/detach-holder-modal.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

vi.mock('@/lib/api/workspace', () => ({ detachHolder: vi.fn().mockResolvedValue(undefined) }))

import { detachHolder } from '@/lib/api/workspace'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'
import { DetachHolderModal } from '@/components/layout/detach-holder-modal'

beforeEach(() => {
  vi.clearAllMocks()
  useDetachModalStore.setState({ target: null })
})

describe('DetachHolderModal', () => {
  it('renders nothing when there is no target', () => {
    const { container } = render(<DetachHolderModal />)
    expect(container).toBeEmptyDOMElement()
  })

  it('names the holder path and states files are safe', () => {
    useDetachModalStore.setState({ target: { wsId: 'w1', branch: 'develop', heldByPath: '/Users/me/repo' } })
    render(<DetachHolderModal />)
    expect(screen.getByText(/\/Users\/me\/repo/)).toBeInTheDocument()
    expect(screen.getByText(/files are safe/i)).toBeInTheDocument()
  })

  it('detaches and closes on confirm', async () => {
    useDetachModalStore.setState({ target: { wsId: 'w1', branch: 'develop', heldByPath: '/repo' } })
    render(<DetachHolderModal />)
    await userEvent.click(screen.getByRole('button', { name: /^detach$/i }))
    expect(detachHolder).toHaveBeenCalledWith('w1')
    await waitFor(() => expect(useDetachModalStore.getState().target).toBeNull())
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/components/layout/detach-holder-modal.test.tsx`
Expected: FAIL — cannot resolve `@/components/layout/detach-holder-modal`.

- [ ] **Step 3: Write the modal**

Create `web/src/components/layout/detach-holder-modal.tsx`:

```tsx
import { Dialog, DialogBackdrop, DialogPopup, DialogPortal, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'
import { detachHolder } from '@/lib/api/workspace'

// The consent modal for freeing a protected branch from a live holder. Framed as
// disruptive-not-destructive: per the git engine's contract, detach never touches
// the working tree — only moves HEAD — so uncommitted changes and commits are
// preserved (spec §3.7). A detach blocked mid-merge/rebase surfaces cleanly as a
// LastError from the backend (no partial state), which the toast/entity surface.
export function DetachHolderModal() {
  const target = useDetachModalStore((s) => s.target)
  const close = useDetachModalStore((s) => s.close)

  if (!target) return null

  const onConfirm = async () => {
    await detachHolder(target.wsId)
    close()
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open) close() }}>
      <DialogPortal>
        <DialogBackdrop />
        <DialogPopup>
          <DialogTitle>Detach to manage {target.branch}</DialogTitle>
          <DialogDescription>
            The checkout at <span className="font-mono">{target.heldByPath}</span> will move to a
            detached HEAD, releasing <span className="font-mono">{target.branch}</span> so Crowbar
            can manage it in its own worktree. Your files are safe — only the working directory's
            current branch changes; uncommitted changes and commits are preserved.
          </DialogDescription>
          <DialogFooter>
            <Button variant="ghost" onClick={close}>
              Cancel
            </Button>
            <Button onClick={onConfirm}>Detach</Button>
          </DialogFooter>
        </DialogPopup>
      </DialogPortal>
    </Dialog>
  )
}
```

> If `@/components/ui/button` exports `Button` with different variant names, use the project's default variant (drop the `variant` prop on Cancel if `ghost` is unavailable). The test only asserts on the button label `Detach` and the copy.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/components/layout/detach-holder-modal.test.tsx`
Expected: PASS.

> If base-ui's `Dialog` requires the portal to attach to `document.body` and jsdom text queries via `screen` don't find it, wrap assertions in `within(document.body)` — `screen` already scopes to `document.body`, so portalled content is found.

- [ ] **Step 5: Mount the modal once at the shell**

In `web/src/components/layout/ide-shell.tsx`, add `import { DetachHolderModal } from './detach-holder-modal'` and render `<DetachHolderModal />` once inside the main render tree (the `return (...)` around line 185), alongside `<FpsOverlay />` (~line 249). No test needed for the mount (covered by the component test in Step 4).

- [ ] **Step 6: Verify suite still green**

Run: `cd web && npx vitest run && npx vite build`
Expected: PASS.

---

## Task 19: Frontend — placeholder row actions + tree-item wiring

**Files:**
- Create: `web/src/components/layout/placeholder-row-actions.tsx`
- Create test: `web/src/__tests__/components/layout/placeholder-row-actions.test.tsx`
- Modify: `web/src/components/layout/workspace-tree-item.tsx:29-31, 96`

**Interfaces:**
- Consumes: `Workspace`, `isPlaceholderWorkspace`/`placeholderReason` (Task 14), `retryProvision` (Task 16), `useDetachModalStore` (Task 17).
- Produces: `PlaceholderRowActions({ workspace }: { workspace: Workspace })` — renders the reconstructed reason + a Retry button (`retryProvision(id)`) + a Detach button (opens the modal store) shown only when `heldByPath` is set; and `WorkspaceTreeItem` computes `isPlaceholder` and passes it to `WorkspaceBranchIcon` + renders `PlaceholderRowActions`.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/layout/placeholder-row-actions.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

vi.mock('@/lib/api/workspace', () => ({ retryProvision: vi.fn().mockResolvedValue(undefined) }))

import { retryProvision } from '@/lib/api/workspace'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'
import { PlaceholderRowActions } from '@/components/layout/placeholder-row-actions'
import type { Workspace } from '@/lib/store/sidebar'

const ws = (over: Partial<Workspace> = {}): Workspace => ({
  id: 'w1',
  branch: 'develop',
  status: 'locked',
  age: '',
  heldByPath: '/repo',
  ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  useDetachModalStore.setState({ target: null })
})

describe('PlaceholderRowActions', () => {
  it('shows the reconstructed reason naming the holder', () => {
    render(<PlaceholderRowActions workspace={ws()} />)
    expect(screen.getByText(/checked out at \/repo/i)).toBeInTheDocument()
  })

  it('retries provisioning on Retry', async () => {
    render(<PlaceholderRowActions workspace={ws()} />)
    await userEvent.click(screen.getByRole('button', { name: /retry/i }))
    expect(retryProvision).toHaveBeenCalledWith('w1')
  })

  it('opens the detach modal on Detach when a holder is known', async () => {
    render(<PlaceholderRowActions workspace={ws()} />)
    await userEvent.click(screen.getByRole('button', { name: /detach/i }))
    expect(useDetachModalStore.getState().target).toEqual({
      wsId: 'w1',
      branch: 'develop',
      heldByPath: '/repo',
    })
  })

  it('hides Detach when there is no holder path', () => {
    render(<PlaceholderRowActions workspace={ws({ heldByPath: '' })} />)
    expect(screen.queryByRole('button', { name: /detach/i })).toBeNull()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/components/layout/placeholder-row-actions.test.tsx`
Expected: FAIL — cannot resolve `@/components/layout/placeholder-row-actions`.

- [ ] **Step 3: Write the component**

Create `web/src/components/layout/placeholder-row-actions.tsx`:

```tsx
import type { Workspace } from '@/lib/store/sidebar'
import { placeholderReason } from '@/lib/workspace/placeholder'
import { retryProvision } from '@/lib/api/workspace'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'

// The inline surface for a placeholder row (spec §3.3): a reconstructed reason
// plus Retry and Detach… actions. Retry re-provisions in place; Detach… opens the
// consent modal (only when a holder path is known — an unknown holder can only be
// retried). Reason + gating are derived from heldByPath; there is no persisted
// lastError (spec §4/B7).
export function PlaceholderRowActions({ workspace }: { workspace: Workspace }) {
  const openDetach = useDetachModalStore((s) => s.open)

  const onRetry = (e: React.MouseEvent) => {
    e.stopPropagation()
    void retryProvision(workspace.id)
  }

  const onDetach = (e: React.MouseEvent) => {
    e.stopPropagation()
    openDetach({
      wsId: workspace.id,
      branch: workspace.branch,
      heldByPath: workspace.heldByPath ?? '',
    })
  }

  return (
    <div className="flex flex-col gap-1 pl-6 pr-2 pb-1">
      <p className="text-xs text-muted-foreground">{placeholderReason(workspace)}</p>
      <div className="flex gap-2">
        <button
          type="button"
          className="rounded-md px-2 py-0.5 text-xs text-foreground/70 hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
          onClick={onRetry}
          onPointerDown={(e) => e.stopPropagation()}
        >
          Retry
        </button>
        {workspace.heldByPath ? (
          <button
            type="button"
            className="rounded-md px-2 py-0.5 text-xs text-foreground/70 hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            onClick={onDetach}
            onPointerDown={(e) => e.stopPropagation()}
          >
            Detach…
          </button>
        ) : null}
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/components/layout/placeholder-row-actions.test.tsx`
Expected: PASS.

- [ ] **Step 5: Wire it into `WorkspaceTreeItem`**

In `web/src/components/layout/workspace-tree-item.tsx`, add the imports:

```tsx
import { WorkspaceBranchIcon } from './workspace-branch-icon'
import { PlaceholderRowActions } from './placeholder-row-actions'
import { isPlaceholderWorkspace } from '@/lib/workspace/placeholder'
```

Compute the flag near `isLocked` (line 31):

```tsx
  const isLocked = workspace.status === 'locked'
  const isPlaceholder = isPlaceholderWorkspace(workspace)
```

Pass it to the icon (line 96):

```tsx
          <WorkspaceBranchIcon
            status={workspace.status ?? 'new'}
            working={workspace.working || isMoving}
            isPlaceholder={isPlaceholder}
          />
```

Render the actions under the row. After the closing `</div>` of the row block (line 179, the `</div>` that closes the `style={{ paddingLeft: ... }}` wrapper), add:

```tsx
      </div>

      {isPlaceholder && <PlaceholderRowActions workspace={workspace} />}

      {showChildrenSection && (
```

> Child-create stays enabled (locked parents already allow it) and the merge action stays hidden (locked) — no change needed for those; the placeholder simply renders the extra actions row.

- [ ] **Step 6: Verify suite still green**

Run: `cd web && npx vitest run && npx vite build`
Expected: PASS.

---

## Task 20: Frontend — placeholder toast watcher

**Files:**
- Create: `web/src/components/layout/placeholder-toast-watcher.tsx`
- Create test: `web/src/__tests__/components/layout/placeholder-toast-watcher.test.tsx`
- Modify: `web/src/components/layout/ide-shell.tsx` (mount `<PlaceholderToastWatcher />`)

**Interfaces:**
- Consumes: `useSidebarStore` (repos), `isPlaceholderWorkspace`/`placeholderReason` (Task 14), `toast.show` (`@/features/window/stores/toast-store`), `useDetachModalStore.open` (Task 17).
- Produces: `PlaceholderToastWatcher()` — renders null; on a newly-observed placeholder fires `toast.show({ message, description, type: 'error', key: ws.id, action: { label: 'Fix…', onClick } })` exactly once per id; the action opens the detach modal.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/layout/placeholder-toast-watcher.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'

vi.mock('@/features/window/stores/toast-store', () => ({ toast: { show: vi.fn() } }))

import { toast } from '@/features/window/stores/toast-store'
import { useSidebarStore } from '@/lib/store/sidebar'
import { PlaceholderToastWatcher } from '@/components/layout/placeholder-toast-watcher'
import type { Repo } from '@/lib/store/sidebar'

const repoWith = (over = {}): Repo => ({
  id: 'r1',
  name: 'repo',
  avatarLabel: 'R',
  avatarColor: 'bg-sky-700',
  workspaces: [
    { id: 'ph', branch: 'develop', status: 'locked', heldByPath: '/repo', age: '' },
  ],
  ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState({ repos: [] })
})

describe('PlaceholderToastWatcher', () => {
  it('fires an error toast with a Fix action once per new placeholder', () => {
    useSidebarStore.setState({ repos: [repoWith()] })
    render(<PlaceholderToastWatcher />)
    expect(toast.show).toHaveBeenCalledTimes(1)
    const arg = (toast.show as unknown as { mock: { calls: unknown[][] } }).mock.calls[0][0] as {
      type: string
      key: string
      action: { label: string }
    }
    expect(arg.type).toBe('error')
    expect(arg.key).toBe('ph')
    expect(arg.action.label).toMatch(/fix/i)
  })

  it('does not re-fire for an already-seen placeholder on re-render', () => {
    useSidebarStore.setState({ repos: [repoWith()] })
    const { rerender } = render(<PlaceholderToastWatcher />)
    rerender(<PlaceholderToastWatcher />)
    expect(toast.show).toHaveBeenCalledTimes(1)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/components/layout/placeholder-toast-watcher.test.tsx`
Expected: FAIL — cannot resolve `@/components/layout/placeholder-toast-watcher`.

- [ ] **Step 3: Write the watcher**

Create `web/src/components/layout/placeholder-toast-watcher.tsx`:

```tsx
import { useEffect, useRef } from 'react'
import { useSidebarStore } from '@/lib/store/sidebar'
import { toast } from '@/features/window/stores/toast-store'
import { isPlaceholderWorkspace, placeholderReason } from '@/lib/workspace/placeholder'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'

// Watches sidebar state and fires ONE error toast per newly-observed placeholder
// protected workspace (spec §3.6). Per CLAUDE.md the toast is fired from a
// component watching store state, never a store/backend. Uses toast.show (the
// only variant carrying an action + a dedup key); the Fix… action opens the
// detach modal.
export function PlaceholderToastWatcher() {
  const repos = useSidebarStore((s) => s.repos)
  const openDetach = useDetachModalStore((s) => s.open)
  const seen = useRef(new Set<string>())

  useEffect(() => {
    for (const repo of repos) {
      for (const ws of repo.workspaces) {
        if (!isPlaceholderWorkspace(ws)) continue
        if (seen.current.has(ws.id)) continue
        seen.current.add(ws.id)
        toast.show({
          message: `Couldn't set up ${ws.branch}`,
          description: placeholderReason(ws),
          type: 'error',
          key: ws.id,
          action: {
            label: 'Fix…',
            onClick: () =>
              openDetach({ wsId: ws.id, branch: ws.branch, heldByPath: ws.heldByPath ?? '' }),
          },
        })
      }
    }
  }, [repos, openDetach])

  return null
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/components/layout/placeholder-toast-watcher.test.tsx`
Expected: PASS.

- [ ] **Step 5: Mount the watcher**

In `web/src/components/layout/ide-shell.tsx`, add `import { PlaceholderToastWatcher } from './placeholder-toast-watcher'` and render `<PlaceholderToastWatcher />` once in the main render tree (the `return (...)` around line 185), alongside `<DetachHolderModal />` from Task 18 and `<FpsOverlay />`. No separate test for the mount.

- [ ] **Step 6: Verify suite still green**

Run: `cd web && npx vitest run && npx vite build`
Expected: PASS.

---

## Final verification

- [ ] **Backend full suite**

Run: `cd api && go test ./...`
Expected: PASS (all packages).

- [ ] **Frontend full suite + build**

Run: `cd web && npx vitest run && npx vite build`
Expected: PASS (all specs) + successful bundle.

---

## Self-Review (completed by plan author)

**1. Spec coverage** — every section maps to a task:
- §3.1 holder resolution (prune-first, samePath, four kinds) → Task 5.
- §3.2 import best-effort + placeholder (single owner, best-effort FF) → Task 6.
- §3.3 placeholder (`locked` + empty `WorktreePath` + `HeldByPath`), Retry-in-place → Tasks 1, 2, 3, 6, 10; FE predicate → Tasks 14, 15, 19.
- §3.4 children under placeholder parent + empty-`WorktreePath` parent guards → Task 7 (child-create needs no change; noted in Task 19 Step 5).
- §3.5 home uses same mechanism (no auto-detach, single owner) → Task 6; `ClearBranch` on consent → Tasks 4, 11.
- §3.6 surface-don't-swallow (placeholder row persisted; toast) → Tasks 6 (persisted row is the surface), 20.
- §3.7 detach modal (consent, disruptive-not-destructive, no partial state) → Tasks 11 (no partial state), 18.
- §4 data model (`HeldByPath` on domain/DTO/CreateInput/command; `ProvisionInPlace`; `ClearBranch`; no `LastError`) → Tasks 1–4; FE fields → Task 13.
- §5 change surface: holder pkg (5), `ImportGitEngine.WorktreePrune` (6), import edits (6), worktree Retry/Detach + guards + removeOne + CreateChild refit (7–11), DTO mapping (1), repo commands (2–4), endpoints (12), FE fields/mapping (13), branch-icon (15), tree-item (19), detach modal (18), toast watcher (20), workspace API (16). `build-repo-tree.ts` unconditional mapping (13).
- §6 resolved decisions (locked reuse, consent-for-all, prune-always) → honored throughout (Global Constraints + Tasks 5, 6, 7).
- §7 testing strategy cases → dead-reg prune (6), home-held single placeholder/B5 (6), external holder (6), parent-FF-fails-local-tip (6), Retry-in-place clears `HeldByPath` only (10), child rejected with `ErrParentUnprovisioned` until provisioned (7), delete protection / removeOne skip (8), detach clears home branch (11), `ClearBranch` blanks only branch (4); FE placeholder glyph/reason (15, 19), unconditional mapping overwrite-on-clear (13), toast once (20), modal copy (18).
- §9/§10/§11 adversarial resolutions: B1 (locked reuse — Global Constraints + 8), B2 (empty-path parent guards — 7), B3 (domain field + DTO mapping — 1), B4 (`WorktreePrune` on `ImportGitEngine` + narrow `holder.Engine` — 5, 6), B5 (single owner — 6), B6 (`ClearBranch` — 4, 11), B7 (`WorkspaceCreator` NOT widened; reason derived from `HeldByPath` — Global Constraints + 6, 14); minors (toast.show — 20; stale home branch — 11) and nits (`samePath` in `holder.Resolve` — 5; unconditional FE mapping — 13).

**2. Placeholder scan** — no `TBD`/`TODO`/"similar to Task N"/"add error handling" without code. Every code step shows real code; classification helpers (e.g. `w.Protected()`) that do not exist are not used — the placeholder is keyed on `Status == locked` + `WorktreePath`.

**3. Type consistency** — verified across tasks:
- `HeldByPath` spelled identically on `domain.Workspace`, `WorkspaceDTO` (Go), `CreateInput`, `CreateWorkspace`, `ProvisionInPlace` (clears it), and FE `heldByPath`.
- `holder.Resolve(ctx, git, repoPath, branch, crowbarHome) (Outcome, error)` + `holder.Kind` constants used identically in Tasks 5, 6, 9, 10, 11.
- Repository methods `ProvisionInPlace(ctx, id, worktreePath, forkPointSha)` and `ClearBranch(ctx, id)` — same signatures in interface (Tasks 3, 4), impl, the three fakes, and the usecase call sites (Tasks 10, 11).
- `Usecase.RetryProvision`/`DetachHolder` and `Hierarchy.RetryProvision`/`DetachHolder` — same signatures (Tasks 10, 11, 12).
- FE `retryProvision(wsId)`/`detachHolder(wsId)`, `isPlaceholderWorkspace`/`placeholderReason`, `useDetachModalStore` `open/close/target`, `DetachTarget{wsId,branch,heldByPath}` — consistent across Tasks 14, 16, 17, 18, 19, 20.
- `ErrParentUnprovisioned` (409-mapped) and `ErrBranchStillHeld` — used in Tasks 7, 10.

---

## Adversarial review (2026-07-01) — BLOCKING

**B-impl-1 (blocker): Task 6 removes the force-detach from `adoptRepoHome` but leaves multiple EXISTING tests that still assert the old force-detach / empty-home-branch behavior, so `go test ./...` at Task 6 Step 8 (and every later checkpoint) FAILS.** Task 6 Step 6 only rewrites `TestImport_CreatesProjectReposAndAdoptsWorktrees` and `TestImport_SkipsNonProtectedLocalWorktrees`. The following UNIT tests in `api/internal/app/usecases/project/project_import_test.go` (package `project`, run by plain `go test ./...`) still encode the removed behavior and are neither rewritten nor deleted:
  - `TestImport_ProvisionsManagedWorktreesForProtectedBranches` (home on protected `main`) — line 554 `Contains(managed["main"].WorktreePath, "/worktree")` (now a placeholder with empty path) and line 556 `Contains(git.Detached, "/repoA")` (home no longer detached) both FAIL.
  - `TestImport_DefaultProtectedBranch_HomeDetachedAndManaged` (home on protected `develop`) — line 575 `Empty(home.Branch)` FAILS (home keeps `develop`); the test's whole premise is gone.
  - `TestImport_HomeRowFailureAfterDetach_ReattachesRepo` — lines 718 `Contains(git.Detached, "/repoA")` and 719 `Len(git.CheckedOut, 1)` FAIL (no detach, no re-attach path anymore).
  - `TestImportRepo_AdoptsDefaultBranchWorkspace` (home on protected `develop`) — line 830 `Empty(home.Branch)` FAILS.
  RESOLVED: Task 6 Step 6 now carries concrete new-model rewrites (actual Go test code, not a directive) for all four tests alongside the first two — home adopted in place, the home-held protected branch becomes a placeholder (locked, empty `WorktreePath`, `HeldByPath == repo path`), and the home-row-failure case asserts NO detach and NO reattach. With those in place `go test ./...` stays green at Step 8 and every later checkpoint.

**B-impl-2 (blocker): the integration suite (`make test-integration`, `-tags integration`) is never updated and breaks; the spec §7 regression update is missing.** `api/tests/repo_import_test.go::TestRepoImport_ProtectedBranchesGetManagedWorktrees` (real git) asserts `Empty(home.Branch)` (line 124) and that the repo folder is on a detached HEAD (lines 148-149); `api/tests/regressions_test.go` asserts the home is "detached off the protected branch (branch=\"\")" (line 116) and "the repo home is detached (HEAD) to free the default branch" (lines 731-733). Under the new model none of these hold. Both files are `//go:build integration`, so the plan's plain `cd api && go test ./...` does NOT compile them — masking the breakage — but `make test-integration` fails, and spec §7's required regression ("Import → `rm -rf ~/.crowbar` → re-import yields both protected branches") is not added by any task. FIX: add a task to update these integration tests to the new model and add the §7 re-import regression.

**M-impl-1 (minor): Task 19 Step 5 instructs adding `import { WorkspaceBranchIcon } from './workspace-branch-icon'`, but `workspace-tree-item.tsx` already imports it at line 3** — a literal follow produces a duplicate-identifier error. Only the two NEW imports (`PlaceholderRowActions`, `isPlaceholderWorkspace`) should be added.

**N-impl-1 (nit): Task 6 Step 6's "Replace the assertion block (lines 152-169)" for `TestImport_SkipsNonProtectedLocalWorktrees` is off by one** — the block's leading comment starts at line 151.

Everything else verified against the codebase is consistent (line/anchor references, the three full-interface `workspace.Workspace` fakes, `holder.Engine` satisfied by both engines, the worktree-test `fakeGit` already defining `WorktreePrune`/`ForceDeleteBranch`, existing CreateChild detach tests using `Contains`, toast/dialog/button APIs, and the Task-10-implements-both interface/impl ordering).
