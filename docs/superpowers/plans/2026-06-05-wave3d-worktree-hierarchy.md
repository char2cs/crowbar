# Wave 3D — Worktree Hierarchy Implementation Plan ★ signature feature

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax. **Invoke the `go-style` skill before writing any Go.**
>
> **DEPENDS ON Plan 3A** (`2026-06-05-wave3a-app-core.md`) — the Workspace aggregate command set (`Reparent`, `UpdateForkPoint`, `SetPendingMerge`, `ClearPendingMerge`, `Delete`, `Create`, `TouchActivity`) and read model (`Workspace.List`/`Get`) must already exist. Do not start until 3A is merged.

**Goal:** Build the worktree-hierarchy **usecases** in `internal/app/usecases/` — worktree-backed workspace create, local child→parent merge (all three strategies), re-parenting, cascade delete (skip locked), and the `pendingMerge`/conflict-resume marker — orchestrating the Wave-1 git primitives with 3A's Workspace Asynx commands. **This is the one to get exactly right.**

**Architecture:** Each hierarchy operation is a usecase method composing git-engine primitives (`WorktreeAdd`, `WorktreeRemove`, `Merge`, `MergeFFOnly`, `Rebase`, `RebaseOnto`, `ForceDeleteBranch`) inside the per-repo mutation lock (held by the engine) with 3A's serialized Workspace commands. The git primitives and 3A commands are **consumed, never forked**. Leaf-guards and locked-guards live in the usecase; the aggregate commands only mutate fields.

**Tech Stack:** Go 1.26.2, `github.com/char2cs/asynx`, the git engine (`internal/engine/git`), the provider engine (locked resolution), testify. Module `github.com/char2cs/crowbar/api`.

---

## ⛔ Rabbyte standards gate (a reviewer checks EACH item)

Same seven rules as 3A. Critical for this plan: **max 2 indentation levels** (these orchestration methods are conflict-prone — decompose aggressively); **one parameter per line**; **early returns**; **no `time.Sleep` in tests** (use real temp git repos + condition checks); **benchmarks** for the merge/reparent hot paths; coverage **≥95%**.

**Verification command after every task:** `cd api && gofumpt -l -w . && goimports -w . && go build ./... && go vet ./... && go test ./internal/app/usecases/...`

---

## Reference — git primitives available (Wave-1, do NOT reimplement)

From `internal/engine/git` (`engine.go`, `worktree.go`, verified present):
- `WorktreeAdd(ctx, repoPath, worktreePath, branch)` — note: current impl is `git worktree add <path> <branch>` (no `-b`). **This task needs branch creation.** Verify/extend (Task 1).
- `WorktreeRemove(ctx, repoPath, worktreePath)` — runs `git worktree remove --force <path>`.
- `WorktreeList(ctx, repoPath) ([]WorktreeEntry, error)`.
- `Merge(ctx, repoPath, branch)` — `git merge <branch>` (classifies `ErrConflict`).
- `MergeFFOnly(ctx, repoPath, branch)` — `git merge --ff-only <branch>`.
- `Rebase(ctx, repoPath, onto)` — `git rebase <onto>` (classifies `ErrConflict`).
- `RebaseOnto(ctx, repoPath, newTip, forkPoint, branch)` — `git rebase --onto`.
- `ForceDeleteBranch(ctx, repoPath, name)` — `git branch -D`.
- `OperationContinue` / `OperationAbort` — finalize/rollback in-progress merge/rebase.
- `ErrConflict` sentinel in `internal/engine/git/errors.go`.

Missing primitives this plan must add to the git engine (each its own Wave-1-style file + test + interface method):
- A branch-creating worktree add: `git worktree add <path> -b <branch> <startPoint>` returning the recorded start SHA.
- `RevParse(ctx, repoPath, rev) (string, error)` — resolve a ref/branch to a SHA (for `forkPointSha` capture and parent-tip reads). **Grep first** — may already exist.
- `Squash` merge: `git merge --squash <branch>` then `git commit` (parent worktree).
- A `MergeBase` primitive may already have been added in 3A Task 24 — reuse it.

---

## File Structure

All under `api/`:

**Git-engine primitives (Wave-1 layer — only if missing after grep):**
- `internal/engine/git/worktree_add_branch.go` (+ test) — branch-creating worktree add returning start SHA.
- `internal/engine/git/rev_parse.go` (+ test) — `RevParse`.
- `internal/engine/git/merge_squash.go` (+ test) — squash merge.
- Extend `internal/engine/git/git.go` interface with the new methods.

**Hierarchy usecase:**
- `internal/app/usecases/worktree.go` — `WorktreeUsecase` interface + impl: `CreateChild`, `MergeIntoParent`, `Reparent`, `DeleteCascade`.
- `internal/app/usecases/internal/worktreepath/worktreepath.go` (+ test) — deterministic worktree dir naming.
- `internal/app/usecases/internal/cascade/cascade.go` (+ test) — pure cascade-tree computation over `parentId`, skip-locked rule.
- `internal/app/usecases/worktree_test.go` — usecase unit tests (mocked git + repo).
- `internal/app/usecases/worktree_integration_test.go` — real-temp-repo integration (build tag none; uses real `git`).
- `internal/app/usecases/worktree_bench_test.go` — merge/reparent benchmarks.

**Wiring:**
- Modify `internal/app/usecases/container.go` — add `Worktree` field.

---

## Phase 0 — Git-engine primitives (only what's missing)

### Task 1: Branch-creating worktree add + RevParse + squash merge

**Files:**
- Create: `internal/engine/git/worktree_add_branch.go` (+ `worktree_add_branch_test.go`)
- Create: `internal/engine/git/rev_parse.go` (+ test) — **skip if grep finds an existing RevParse/equivalent**
- Create: `internal/engine/git/merge_squash.go` (+ test)
- Modify: `internal/engine/git/git.go` (interface)

- [ ] **Step 1: Grep for existing primitives**

Run: `cd api && grep -rn "rev-parse\|RevParse\|worktree add\|--squash\|MergeBase" internal/engine/git/`
Note which already exist. Implement only the genuinely missing ones; reuse the rest. (3A Task 24 may have added `MergeBase`.)

- [ ] **Step 2: Write the failing test for `WorktreeAddBranch`**

Create `internal/engine/git/worktree_add_branch_test.go` (uses a real temp repo helper — check `internal_test.go`/`ops_test.go` for an existing `initRepo(t)` helper and reuse it):

```go
package git

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreeAddBranch_CreatesBranchAndReturnsStartSha(t *testing.T) {
	repo := initRepoWithCommit(t) // helper: git init + one commit on the default branch
	e := New()
	ctx := context.Background()

	child := filepath.Join(t.TempDir(), "child")
	startSha, err := e.WorktreeAddBranch(ctx, repo, child, "feature/x", "HEAD")
	require.NoError(t, err)
	assert.NotEmpty(t, startSha)

	// the worktree exists on branch feature/x
	entries, err := e.WorktreeList(ctx, repo)
	require.NoError(t, err)
	found := false
	for _, en := range entries {
		if en.Branch == "feature/x" {
			found = true
		}
	}
	assert.True(t, found)
}
```

> If no `initRepoWithCommit`/`initRepo` helper exists, add one to `export_test.go` or a shared `_test.go` using `exec.Command("git", ...)` against `t.TempDir()` with `git -c user.email=t@t -c user.name=t commit --allow-empty -m init`.

- [ ] **Step 3: Run red.**

Run: `cd api && go test ./internal/engine/git/ -run TestWorktreeAddBranch`
Expected: FAIL — undefined.

- [ ] **Step 4: Implement `worktree_add_branch.go`**

```go
package git

import (
	"context"
	"fmt"
	"strings"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func (e *engine) WorktreeAddBranch(
	ctx context.Context,
	repoPath string,
	worktreePath string,
	branch string,
	startPoint string,
) (string, error) {
	startSha, err := e.revParse(ctx, repoPath, startPoint)
	if err != nil {
		return "", fmt.Errorf("worktree add branch: resolve start: %w", err)
	}
	unlock := e.lockRepo(repoPath)
	defer unlock()
	r := e.exec(ctx, repoPath, "worktree", "add", worktreePath, "-b", branch, startSha)
	if err := gitexec.RequireSuccess("worktree add branch", r); err != nil {
		return "", err
	}
	return startSha, nil
}

func (e *engine) revParse(
	ctx context.Context,
	repoPath string,
	rev string,
) (string, error) {
	r := e.exec(ctx, repoPath, "rev-parse", rev)
	if err := gitexec.RequireSuccess("rev-parse", r); err != nil {
		return "", err
	}
	return strings.TrimSpace(r.Stdout), nil
}
```

> `revParse` is unexported helper; the exported `RevParse` interface method (if needed elsewhere) wraps it. Keep `lockRepo` usage but note: `revParse` runs **before** taking the lock to avoid double-locking (the `git worktree add` is the locked section). If `lockRepo` is non-reentrant (it is — `sync.Mutex`), this ordering matters.

- [ ] **Step 5: Implement `RevParse` exported method + `MergeSquash`**

`rev_parse.go` (only if no public equivalent):
```go
package git

import "context"

func (e *engine) RevParse(
	ctx context.Context,
	repoPath string,
	rev string,
) (string, error) {
	return e.revParse(ctx, repoPath, rev)
}
```

`merge_squash.go`:
```go
package git

import (
	"context"

	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func (e *engine) MergeSquash(
	ctx context.Context,
	repoPath string,
	branch string,
	subject string,
) error {
	unlock := e.lockRepo(repoPath)
	defer unlock()
	sq := e.exec(ctx, repoPath, "merge", "--squash", branch)
	if err := classifyGitError("merge --squash", sq); err != nil {
		return err
	}
	c := e.exec(ctx, repoPath, "commit", "-m", subject)
	return classifyGitError("merge squash commit", c)
}
```

- [ ] **Step 6: Add the interface methods to `git.go`** (one param per line each): `WorktreeAddBranch`, `RevParse`, `MergeSquash` with doc comments referencing `07 §1/§3.1`.

- [ ] **Step 7: Write tests for `RevParse` + `MergeSquash`** (real temp repo: `MergeSquash` of a child branch with one commit yields a single squashed commit on parent). Run red → green.

- [ ] **Step 8: Run + commit**

Run: `cd api && go test ./internal/engine/git/`
```bash
git add api/internal/engine/git/
git commit -m "feat(git): worktree-add-branch (start SHA) + RevParse + MergeSquash (07)"
```

---

## Phase 1 — Pure helpers

### Task 2: Worktree path naming

**Files:**
- Create: `internal/app/usecases/internal/worktreepath/worktreepath.go` (+ test)

- [ ] **Step 1: Write failing tests** — `For(repoPath, branch)` returns a deterministic sibling dir (e.g. `<repoParent>/.crowbar-worktrees/<repoBase>/<sanitized-branch>`), sanitizing `/` in branch names. Same inputs → same path; different branches → different paths.

```go
func TestFor_DeterministicAndSanitized(t *testing.T) {
	a := For("/home/u/proj/repo", "feature/x")
	b := For("/home/u/proj/repo", "feature/x")
	assert.Equal(t, a, b)
	assert.NotContains(t, filepath.Base(a), "/")
	assert.NotEqual(t, a, For("/home/u/proj/repo", "feature/y"))
}
```

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement `worktreepath.go`** — `For(repoPath, branch string) string`; replace `/` and unsafe chars with `-`; root under a stable `.crowbar-worktrees` sibling of the repo. Pure function.

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/usecases/internal/worktreepath/
git commit -m "feat(usecases): deterministic worktree path naming (07 §1)"
```

### Task 3: Cascade-tree computation (skip locked)

**Files:**
- Create: `internal/app/usecases/internal/cascade/cascade.go` (+ test)

> Pure tree logic over `parentId`. `Plan(root, all)` returns the deletion order (deepest-first) of the subtree rooted at `root`, **skipping locked nodes and their would-be removal** but still descending past a locked node? Per 07 §5: "a locked child blocks its own deletion and is left in place; unlocked descendants are removed." So a locked node is **not** deleted, but its unlocked descendants **are** still removed. Compute: full subtree; exclude locked nodes from the delete list; keep unlocked descendants even under a locked ancestor.

- [ ] **Step 1: Write failing tests**

```go
func TestPlan_DeepestFirstSkippingLocked(t *testing.T) {
	all := []Node{
		{ID: "root", Parent: ""},
		{ID: "a", Parent: "root"},
		{ID: "b", Parent: "a", Locked: true},
		{ID: "c", Parent: "b"}, // unlocked descendant under a locked node
	}
	order := Plan("root", all)
	// c and a and root deletable; b skipped; deepest-first
	assert.Equal(t, []string{"c", "a", "root"}, order)
}

func TestPlan_LockedRootYieldsOnlyUnlockedDescendants(t *testing.T) {
	all := []Node{
		{ID: "root", Parent: "", Locked: true},
		{ID: "a", Parent: "root"},
	}
	assert.Equal(t, []string{"a"}, Plan("root", all))
}

func TestPlan_UnknownRootEmpty(t *testing.T) {
	assert.Empty(t, Plan("nope", nil))
}
```

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement `cascade.go`** — `Node{ID, Parent string; Locked bool}`; `Plan(rootID string, all []Node) []string`. Build a children index, DFS post-order (deepest-first), append only unlocked nodes. Decompose DFS into a `planner` struct method to stay ≤2 indent levels.

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/usecases/internal/cascade/
git commit -m "feat(usecases): cascade delete-order computation, skip locked (07 §5)"
```

---

## Phase 2 — The hierarchy usecase

> One usecase type, four operations, each decomposed. Tests come in two layers: **unit** (mocked git engine + mocked workspace repo, asserting the exact primitive/command sequence) and **integration** (real `git` against `t.TempDir()`, asserting parent SHAs advance correctly — the spec's "Verify" section). Build the interface first.

### Task 4: `WorktreeUsecase` skeleton + `CreateChild`

**Files:**
- Create: `internal/app/usecases/worktree.go`
- Create: `internal/app/usecases/worktree_test.go`

- [ ] **Step 1: Write the failing unit test for `CreateChild`**

Mocked deps: a fake git engine recording `WorktreeAddBranch(repoPath, path, branch, startPoint)` and returning a start SHA; a `workspace.MockWorkspace` (from 3A) capturing `Create`; a fake provider engine returning protected `[main]`. Assert: child created with `forkPointSha` = the returned start SHA, `locked=false` (branch `feature/x` not protected), `parentId` set, worktree path from `worktreepath.For`.

```go
func TestCreateChild_RecordsForkPointAndLocked(t *testing.T) {
	// parent workspace w-parent on branch "develop" at /repo
	// expect: git.WorktreeAddBranch(/repo, <path>, "feature/x", "develop") -> "sha123"
	//         workspace.Create with ForkPointSha "sha123", ParentID "w-parent", Locked false
	out, err := uc.CreateChild(ctx, CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		Branch: "feature/x", ParentID: "w-parent", ParentBranch: "develop",
	})
	require.NoError(t, err)
	assert.Equal(t, "sha123", capturedCreate.ForkPointSha)
	assert.False(t, capturedCreate.Locked)
	assert.NotEmpty(t, out.ID)
}

func TestCreateChild_LocksProtectedBranch(t *testing.T) {
	// provider returns ["feature/x"] protected -> Locked true
}
```

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement `worktree.go` skeleton + `CreateChild`**

```go
package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
)

// CreateChildInput carries the fields needed to create a worktree-backed child.
type CreateChildInput struct {
	RepoID       string
	ProjectID    string
	RepoPath     string
	Branch       string
	ParentID     string
	ParentBranch string
}

// WorktreeUsecase orchestrates the worktree hierarchy (07): create, local merge,
// re-parent, cascade delete. It composes the git primitives with 3A's Workspace
// Asynx commands; it forks neither.
type WorktreeUsecase interface {
	CreateChild(
		ctx context.Context,
		in CreateChildInput,
	) (domain.Workspace, error)
	MergeIntoParent(
		ctx context.Context,
		childID string,
		strategy gitdomain.MergeStrategy,
	) (MergeResult, error)
	Reparent(
		ctx context.Context,
		childID string,
		newParentID string,
	) (domain.Workspace, error)
	DeleteCascade(
		ctx context.Context,
		rootID string,
	) error
}

type worktreeUsecase struct {
	workspaces workspace.Workspace
	git        enginegit.Engine
	provider   engineprovider.Engine
	now        func() time.Time
}

// NewWorktreeUsecase builds the hierarchy usecase.
func NewWorktreeUsecase(
	workspaces workspace.Workspace,
	git enginegit.Engine,
	provider engineprovider.Engine,
	now func() time.Time,
) WorktreeUsecase {
	return &worktreeUsecase{
		workspaces: workspaces,
		git:        git,
		provider:   provider,
		now:        now,
	}
}

func (u *worktreeUsecase) CreateChild(
	ctx context.Context,
	in CreateChildInput,
) (domain.Workspace, error) {
	path := worktreepath.For(in.RepoPath, in.Branch)
	startSha, err := u.git.WorktreeAddBranch(ctx, in.RepoPath, path, in.Branch, in.ParentBranch)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: worktree add: %w", err)
	}
	locked, err := u.resolveLocked(ctx, in.RepoPath, in.Branch)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create child: locked: %w", err)
	}
	return u.workspaces.Create(ctx, workspace.CreateInput{
		ID:           uuid.NewString(),
		RepoID:       in.RepoID,
		ProjectID:    in.ProjectID,
		Branch:       in.Branch,
		WorktreePath: path,
		ForkPointSha: startSha,
		ParentID:     in.ParentID,
		Locked:       locked,
	}, u.now())
}

func (u *worktreeUsecase) resolveLocked(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	protected, err := u.provider.ProtectedBranches(ctx, repoPath)
	if err != nil {
		return false, err
	}
	for _, b := range protected {
		if b == branch {
			return true, nil
		}
	}
	return false, nil
}
```

> Confirm the UUID package used elsewhere (`grep -rn "uuid" internal/`); match it. If the project uses a different id minter, use that.

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/usecases/worktree.go api/internal/app/usecases/worktree_test.go
git commit -m "feat(usecases): WorktreeUsecase.CreateChild — worktree-backed create (07 §1)"
```

### Task 5: `MergeIntoParent` — all three strategies + conflict marker

**Files:**
- Modify: `internal/app/usecases/worktree.go`
- Create: `internal/app/usecases/merge_result.go`
- Test: append to `internal/app/usecases/worktree_test.go`

> The most important method. Per 07 §3.1:
> - Guard: parent unlocked; `rebase` strategy forbidden for a non-leaf child.
> - `merge`: in **parent** worktree, `git merge <childBranch>`.
> - `squash`: in **parent** worktree, `git merge --squash` + commit.
> - `rebase`: in **child** worktree, `git rebase <parentBranch>`, **then** in parent `git merge --ff-only <childBranch>` — one locked critical section (the engine's per-repo lock covers each call; the spec requires no parent advance between — since all worktrees of a repo share the engine's per-repo mutex by `repoPath`, sequential calls are atomic enough; **document that both child and parent ops use the same `repoPath` lock key**).
> - On conflict (`ErrConflict`): set `pendingMerge{strategy, targetParentId}` and return a "conflicts pending" result; the op completes/aborts later via the git operation continue/abort routes (3B/04 surface) which then clear the marker + finalize fork points.
> - On success: for a **kept** child, `UpdateForkPoint` to the parent's post-merge tip (all strategies). (Default action is delete; keep is the alternative — the usecase returns the result and lets the caller decide keep/delete. For this usecase, `MergeIntoParent` does the merge + always updates the kept child's fork point to the new parent tip; the caller deletes separately if the user chose delete.)

- [ ] **Step 1: Write `merge_result.go`**

```go
package usecases

// MergeResult reports the outcome of a local merge-into-parent (07 §3.1).
type MergeResult struct {
	ConflictsPending bool
	ParentTipSha     string
}
```

- [ ] **Step 2: Write failing unit tests** for each strategy and the guards:

```go
func TestMergeIntoParent_RejectsLockedParent(t *testing.T) { /* parent.Locked -> error, no git calls */ }
func TestMergeIntoParent_RejectsRebaseForNonLeafChild(t *testing.T) { /* child has children -> error */ }

func TestMergeIntoParent_MergeStrategy_RunsInParentThenUpdatesForkPoint(t *testing.T) {
	// expect git.Merge(parentRepoPath==parentWorktree, childBranch)
	// then RevParse(parent worktree, "HEAD") -> "ptip"
	// then workspaces.UpdateForkPoint(childID, "ptip")
}

func TestMergeIntoParent_RebaseStrategy_RebasesChildThenFFMerges(t *testing.T) {
	// expect git.Rebase(childWorktree, parentBranch) then git.MergeFFOnly(parentWorktree, childBranch)
}

func TestMergeIntoParent_Conflict_SetsPendingMerge(t *testing.T) {
	// git.Merge returns ErrConflict -> workspaces.SetPendingMerge(childID, strategy, parentID), result.ConflictsPending true
}
```

> The unit test needs the child + parent workspace rows from `workspaces.Get`; provide them via the mock. "child has children" is detected via `workspaces.List` filtered by `ParentID == childID`.

- [ ] **Step 3: Run red.**

- [ ] **Step 4: Implement `MergeIntoParent`** — decomposed so no method exceeds 2 indent levels / 100 LOC:

```go
func (u *worktreeUsecase) MergeIntoParent(
	ctx context.Context,
	childID string,
	strategy gitdomain.MergeStrategy,
) (MergeResult, error) {
	child, parent, err := u.loadMergePair(ctx, childID)
	if err != nil {
		return MergeResult{}, err
	}
	if err := u.guardMerge(ctx, child, parent, strategy); err != nil {
		return MergeResult{}, err
	}
	if conflictErr := u.runMerge(ctx, child, parent, strategy); conflictErr != nil {
		return u.handleMergeError(ctx, child, parent, strategy, conflictErr)
	}
	return u.finalizeMerge(ctx, child, parent)
}
```

with helpers:
- `loadMergePair` — `Get(childID)`, then `Get(child.ParentID)`; error if no parent.
- `guardMerge` — parent unlocked; if `strategy == rebase` and child has children (`childHasChildren`), reject; return typed errors (`ErrParentLocked`, `ErrRebaseNonLeaf` — define in `worktree_errors.go`).
- `runMerge` — switch on strategy, calling the right primitive in the right worktree (`parent.WorktreePath` vs `child.WorktreePath`); for `rebase`, both calls. Returns the git error (possibly `ErrConflict`).
- `handleMergeError` — if `errors.Is(err, enginegit.ErrConflict)`: `SetPendingMerge` + return `{ConflictsPending:true}`, nil; else return the wrapped error.
- `finalizeMerge` — `RevParse(parent.WorktreePath, "HEAD")` → parent tip; `UpdateForkPoint(child.ID, tip)`; return `{ParentTipSha: tip}`.
- `childHasChildren(ctx, childID)` — `workspaces.List`, any with `ParentID == childID`.

Define `worktree_errors.go` with `ErrParentLocked`, `ErrRebaseNonLeaf`, `ErrChildHasChildren`, `ErrNewParentLocked` sentinels (each `errors.New`, doc comment).

- [ ] **Step 5: Run green.** Run: `cd api && go test ./internal/app/usecases/ -run MergeIntoParent`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/app/usecases/
git commit -m "feat(usecases): MergeIntoParent — 3 strategies + conflict marker (07 §3.1)"
```

### Task 6: `Reparent` — leaf-guard + `rebase --onto` + fork-point update

**Files:**
- Modify: `internal/app/usecases/worktree.go`
- Test: append to `worktree_test.go`

> Per 07 §4: `git rebase --onto <newParentTip> <forkPointSha> <childBranch>` (child worktree), using the **recorded** `forkPointSha` (never `merge-base`). Guards: child must be a leaf (`409` if it has children); new parent must be unlocked. On success: `Reparent` command sets `parentId=newParentId` + `forkPointSha=newParentTip`.

- [ ] **Step 1: Write failing tests**

```go
func TestReparent_RejectsNonLeafChild(t *testing.T) { /* child has children -> ErrChildHasChildren */ }
func TestReparent_RejectsLockedNewParent(t *testing.T) { /* newParent.Locked -> ErrNewParentLocked */ }
func TestReparent_RebasesOntoNewTipAndUpdatesAggregate(t *testing.T) {
	// newParentTip = RevParse(newParent.WorktreePath, "HEAD") -> "ntip"
	// expect git.RebaseOnto(child.RepoPath?, "ntip", child.ForkPointSha, child.Branch)
	// then workspaces.Reparent(childID, newParentID, "ntip", now)
}
```

> `RebaseOnto` runs in the child's worktree (the branch checked out there). Pass the child's worktree path as `repoPath` to `RebaseOnto`.

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement `Reparent`** — decomposed:

```go
func (u *worktreeUsecase) Reparent(
	ctx context.Context,
	childID string,
	newParentID string,
) (domain.Workspace, error) {
	child, err := u.workspaces.Get(ctx, childID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: get child: %w", err)
	}
	newParent, err := u.workspaces.Get(ctx, newParentID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: get new parent: %w", err)
	}
	if err := u.guardReparent(ctx, child, newParent); err != nil {
		return domain.Workspace{}, err
	}
	tip, err := u.git.RevParse(ctx, newParent.WorktreePath, "HEAD")
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: new parent tip: %w", err)
	}
	if err := u.git.RebaseOnto(ctx, child.WorktreePath, tip, child.ForkPointSha, child.Branch); err != nil {
		return domain.Workspace{}, fmt.Errorf("reparent: rebase onto: %w", err)
	}
	return u.workspaces.Reparent(ctx, childID, newParentID, tip, u.now())
}

func (u *worktreeUsecase) guardReparent(
	ctx context.Context,
	child domain.Workspace,
	newParent domain.Workspace,
) error {
	if newParent.Locked {
		return ErrNewParentLocked
	}
	hasKids, err := u.childHasChildren(ctx, child.ID)
	if err != nil {
		return fmt.Errorf("reparent: leaf check: %w", err)
	}
	if hasKids {
		return ErrChildHasChildren
	}
	return nil
}
```

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/usecases/worktree.go api/internal/app/usecases/worktree_test.go
git commit -m "feat(usecases): Reparent — leaf-guard + rebase --onto + fork-point (07 §4)"
```

### Task 7: `DeleteCascade` — skip locked, worktree-remove-then-branch-delete order

**Files:**
- Modify: `internal/app/usecases/worktree.go`
- Test: append to `worktree_test.go`

> Per 07 §5: compute cascade order (deepest-first, skip locked) via `cascade.Plan`; per removable node, `git worktree remove --force <path>` **then** `git branch -D <branch>` (order matters; force both), then the aggregate `Delete` (Asynx `Forget`). A locked node is skipped (left in place). Removal of one node's worktree must precede its branch delete.

- [ ] **Step 1: Write failing tests**

```go
func TestDeleteCascade_DeepestFirstSkippingLocked(t *testing.T) {
	// tree root->a->b(locked)->c ; List returns all four
	// expect remove order c, a, root (b skipped); each: WorktreeRemove then ForceDeleteBranch then workspaces.Delete
}
func TestDeleteCascade_WorktreeRemovedBeforeBranchDeleted(t *testing.T) {
	// assert call order per node via a recording fake
}
```

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Implement `DeleteCascade`**

```go
func (u *worktreeUsecase) DeleteCascade(
	ctx context.Context,
	rootID string,
) error {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return fmt.Errorf("delete cascade: list: %w", err)
	}
	order := cascade.Plan(rootID, toNodes(all))
	index := indexByID(all)
	for _, id := range order {
		if err := u.removeOne(ctx, index[id]); err != nil {
			return fmt.Errorf("delete cascade: remove %s: %w", id, err)
		}
	}
	return nil
}

func (u *worktreeUsecase) removeOne(
	ctx context.Context,
	ws domain.Workspace,
) error {
	if err := u.git.WorktreeRemove(ctx, ws.WorktreePath, ws.WorktreePath); err != nil {
		return fmt.Errorf("worktree remove: %w", err)
	}
	if err := u.git.ForceDeleteBranch(ctx, ws.WorktreePath, ws.Branch); err != nil {
		return fmt.Errorf("branch delete: %w", err)
	}
	return u.workspaces.Delete(ctx, ws.ID)
}
```

> `WorktreeRemove(ctx, repoPath, worktreePath)` — pass the **repo** path as the first arg. The Workspace row needs the repo path; if the read-model row only has `WorktreePath`, derive repo path from the `RepoID` → Repository store, or store `RepoPath` on the workspace. **Resolve this:** the `WorktreeRemove`/`ForceDeleteBranch` must run against the repo's main worktree, not the child's own (a worktree can't remove itself). Add a `repoPathFor(ws)` lookup via the Repository GORM store (inject it into the usecase) and use it. Update the usecase constructor + `removeOne` accordingly, and adjust `CreateChild`/merge/reparent to use `repoPathFor` consistently. Add a test that `WorktreeRemove` is called with the repo path, not the child path.

`toNodes`, `indexByID` are small pure helpers (own file `cascade_adapt.go` or inline in `worktree.go` if short).

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/usecases/
git commit -m "feat(usecases): DeleteCascade — skip locked, remove-then-delete order (07 §5)"
```

---

## Phase 3 — Integration verification (real git)

### Task 8: Real-repo integration test — the spec's "Verify" matrix

**Files:**
- Create: `internal/app/usecases/worktree_integration_test.go`

> This is the spec's explicit acceptance: "child create → commit → local merge (each strategy) → parent advances correctly; re-parent a leaf; re-parent with children is rejected; delete cascade skips a locked child." Build a **real** workspace usecase over a real git repo (in-memory event store + in-memory GORM DB + real `enginegit.New()` + a stub provider that marks nothing protected unless told). No mocks for git.

- [ ] **Step 1: Write the integration helper + first scenario (merge strategy)**

```go
package usecases_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	// build real workspace repo (asynx + gorm), enginegit.New(), a stub provider
)

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

func TestIntegration_MergeStrategyAdvancesParent(t *testing.T) {
	// init repo, one commit on "develop"
	// uc.CreateChild(branch feature/x off develop)
	// write a file + commit in the child worktree (gitRun)
	// uc.MergeIntoParent(childID, merge)
	// assert: develop tip now contains the child's commit (git log on parent)
	// assert: child.forkPointSha == new develop tip (kept-child invariant)
}
```

- [ ] **Step 2: Run it (red until the wiring helper is built).**

- [ ] **Step 3: Build the integration wiring helper** `newRealUsecase(t)` returning a `WorktreeUsecase` + repo handles over real git/asynx/gorm. Run green.

- [ ] **Step 4: Add the remaining scenarios** (one test func each, each its own step):
  - `TestIntegration_SquashStrategyAdvancesParent` — parent gets a single squashed commit.
  - `TestIntegration_RebaseStrategyReplaysChildThenFFMerges` — parent fast-forwards; child SHAs rewritten; kept child fork point updated.
  - `TestIntegration_ReparentLeafReplaysOnlyChildCommits` — re-parent a leaf onto a new parent; child commits replayed, old-parent history dropped.
  - `TestIntegration_ReparentWithChildrenRejected` — create grandchild, expect `ErrChildHasChildren`.
  - `TestIntegration_DeleteCascadeSkipsLockedChild` — mark one child locked (provider stub), delete root, assert the locked child's worktree + branch survive while unlocked descendants are gone.
  - `TestIntegration_MergeConflictSetsPendingMerge` — create conflicting edits in parent + child, merge, assert `pendingMerge` set on the child and `ConflictsPending` true; then `OperationAbort` + `ClearPendingMerge` rolls back.

> **No `time.Sleep`** — use `SendWait` everywhere (already synchronous) and read the read model directly after; the projection runs synchronously under `SendWait`. If a projection read races, use `require.Eventually` (polling, not sleeping).

- [ ] **Step 5: Run the full integration suite**

Run: `cd api && go test ./internal/app/usecases/ -run Integration -v`
Expected: all scenarios PASS. (Requires `git` on PATH — it is, per the engine design.)

- [ ] **Step 6: Commit**

```bash
git add api/internal/app/usecases/worktree_integration_test.go
git commit -m "test(usecases): worktree hierarchy real-git integration matrix (07 Verify)"
```

### Task 9: Merge/reparent benchmarks

**Files:**
- Create: `internal/app/usecases/worktree_bench_test.go`

- [ ] **Step 1: Implement benchmarks** — `BenchmarkMergeIntoParent` and `BenchmarkReparent` over a real small repo (reuse the integration helper, reset state per iteration). Crowbar is an IDE — these are hot paths (07 §6 broadcasts on every mutation).

- [ ] **Step 2: Run**

Run: `cd api && go test ./internal/app/usecases/ -bench 'MergeIntoParent|Reparent' -benchtime 5x -run '^$'`
Expected: benchmarks execute without error.

- [ ] **Step 3: Commit**

```bash
git add api/internal/app/usecases/worktree_bench_test.go
git commit -m "test(usecases): worktree merge/reparent benchmarks"
```

---

## Phase 4 — Wire into the usecases container

### Task 10: Add `Worktree` to the usecases container

**Files:**
- Modify: `internal/app/usecases/container.go`
- Test: modify `internal/app/usecases/container_test.go`

- [ ] **Step 1: Write the failing test** — `container.Worktree` is non-nil after `New`.

- [ ] **Step 2: Run red.**

- [ ] **Step 3: Add the field + construction** — `Worktree WorktreeUsecase`; build via `NewWorktreeUsecase(repos.Workspace, engines.Git, engines.Provider, time.Now, repoPathLookup)`. The `repoPathLookup` reads the Repository GORM store. One param per line.

- [ ] **Step 4: Run green. Commit.**

```bash
git add api/internal/app/usecases/container.go api/internal/app/usecases/container_test.go
git commit -m "feat(usecases): mount WorktreeUsecase in container"
```

---

## Phase 5 — Full verification

### Task 11: Build, vet, coverage, lint, race

- [ ] **Step 1:** `cd api && gofumpt -l -w . && goimports -w . && go build ./... && go vet ./...` → clean.
- [ ] **Step 2:** `cd api && go test -coverpkg=./internal/app/usecases/...,./internal/engine/git/... -coverprofile=cover.out ./internal/app/usecases/... ./internal/engine/git/... && go tool cover -func=cover.out | tail -1` → **≥95%**. Add tests for any uncovered guard/error branch.
- [ ] **Step 3:** `cd api && golangci-lint run ./internal/app/usecases/... ./internal/engine/git/...` → no findings (watch nestif on the merge switch — decompose if it trips).
- [ ] **Step 4:** `cd api && go test -race ./internal/app/usecases/...` → PASS.
- [ ] **Step 5: Final commit**

```bash
git add -A api/
git commit -m "test(wave3d): worktree hierarchy build/vet/coverage/lint/race green"
```

---

## Self-Review checklist

- **Spec coverage (07):** worktree-backed create + `forkPointSha` capture ✓ Task 4; local merge all three strategies, correct worktree per command, rebase = rebase-child-then-ff-merge ✓ Task 5; kept-child fork-point update every strategy ✓ Task 5 `finalizeMerge`; conflict → `pendingMerge` + resume/abort ✓ Tasks 5, 8; re-parent leaf-guard + `rebase --onto` recorded fork point + fork-point update ✓ Task 6; re-parent-with-children rejected (409) ✓ Tasks 6, 8; cascade delete skip-locked + remove-then-`-D` order ✓ Task 7; real-repo Verify matrix ✓ Task 8.
- **Consumes, never forks:** git primitives (engine) + 3A Workspace commands — confirmed; only genuinely-missing git primitives added (Task 1).
- **Guard placement:** all guards in the usecase (`guardMerge`/`guardReparent`); commands only mutate.
- **repoPath correctness:** worktree remove/branch-delete run against the repo path (via `repoPathFor`), never the child's own worktree — Task 7.
- **No placeholders:** every method body specified; integration matrix enumerated.

---

## Execution Handoff

Depends on **3A**. Recommended: **Subagent-Driven** execution. Sibling plans: `2026-06-05-wave3b-branch-review.md` (can run after 3A independently of 3D), `2026-06-05-wave3c-lsp.md` (independent).
