# One Workspace Per Branch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce at most one non-deleted workspace per `(repoId, branch)`, with a clean 409 on both backend create paths and a live-validation create UX that offers to open the existing workspace.

**Architecture:** Identity stays a random v4 UUID; uniqueness is a separate create-time invariant. The backend rejects a duplicate-branch create in the `CreateChild` usecase (authoritative) and synchronously in the create handler (for an immediate 409). The frontend validates the create input as the user types against the repo's branches (including the default) and, on a collision, disables confirm and shows a clickable "open it" hint.

**Tech Stack:** Go (usecases + gin handlers + testify, build tag `integration` for `api/tests`), React + zustand + Vitest/RTL (`web/`), TanStack Router.

## Global Constraints

- Invariant: at most one workspace with `Status != deleted` per `(repoId, branch)`. (verbatim from spec)
- Workspace ids remain random v4 (`uuid.NewString()`); do NOT derive ids from the branch. (spec non-goal)
- No backend rename exists; enforce the invariant at create time only. (spec non-goal)
- Branch match is exact, case-sensitive, scoped to the repo. (spec)
- Deleted prior workspace on the same branch is ALLOWED (re-create). (spec)
- Hint copy exactly: `'<branch>' already has a workspace — open it`. (spec)
- Backend test files use `//go:build integration` only for `api/tests`; unit tests are untagged. Frontend tests live under `web/src/__tests__/` mirroring `web/src/`, using `@/` imports (project CLAUDE.md).

---

### Task 1: Generalize the backend create guard to all branches

Replaces the already-committed default-only guard (`mainWorktreeAdopted`, inside `adoptMainWorktree`) with a uniform `(repoId, branch)` check at the top of `CreateChild`, covering both the adopt and child paths, and renames the sentinel.

**Files:**
- Modify: `api/internal/app/usecases/worktree/worktree_errors.go` (rename sentinel)
- Modify: `api/internal/app/usecases/worktree/worktree.go` (move + generalize guard)
- Modify: `api/internal/api/libs/status.go` (409 mapping symbol)
- Test: `api/internal/app/usecases/worktree/worktree_test.go`
- Test: `api/internal/api/libs/status_test.go`

**Interfaces:**
- Consumes: `u.workspaces.List(ctx) ([]domain.Workspace, error)`; `domain.Workspace{RepoID, Branch string; Status WorkspaceStatus}`; `domain.WorkspaceStatusDeleted`.
- Produces: `worktree.ErrBranchWorkspaceExists` (replaces `worktree.ErrDefaultWorkspaceExists`); `(*worktreeUsecase).branchWorkspaceExists(ctx, repoID, branch string) (bool, error)`.

- [ ] **Step 1: Update the failing/again tests first**

In `api/internal/app/usecases/worktree/worktree_test.go`, rename the existing reject test and update its data to match on branch, fix its sentinel, and add a child-path reject test. Replace the body of `TestCreateChild_AdoptMainWorktree_RejectsSecondAdoption` and add the two new tests:

```go
// TestCreateChild_RejectsDuplicateBranch_AdoptPath proves a second workspace on
// the repo's default branch (empty parent + branch == default) is rejected with
// ErrBranchWorkspaceExists before any git work — no phantom duplicate row.
func TestCreateChild_RejectsDuplicateBranch_AdoptPath(t *testing.T) {
	g := &fakeGit{revParseSha: "headsha"}
	createCalls := 0
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{{ID: "ws-default", RepoID: "r1", Branch: "develop"}}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			createCalls++
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		Branch: "develop", ParentID: "", ParentBranch: "develop",
	})

	require.ErrorIs(t, err, worktree.ErrBranchWorkspaceExists)
	assert.Equal(t, 0, createCalls, "no duplicate workspace row is persisted")
	assert.Empty(t, g.ops(), "guard rejects before any git work")
}

// TestCreateChild_RejectsDuplicateBranch_ChildPath proves a NON-default branch
// that already has a workspace is rejected cleanly (not a raw git "branch
// exists" error) before git worktree add runs.
func TestCreateChild_RejectsDuplicateBranch_ChildPath(t *testing.T) {
	g := &fakeGit{addStartSha: "sha"}
	createCalls := 0
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{{ID: "ws-x", RepoID: "r1", Branch: "feature/x"}}, nil
		},
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			createCalls++
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", RemoteURL: "https://github.com/test/repo.git",
		Branch: "feature/x", ParentID: "w-parent", ParentBranch: "develop",
	})

	require.ErrorIs(t, err, worktree.ErrBranchWorkspaceExists)
	assert.Equal(t, 0, createCalls)
	assert.NotContains(t, g.ops(), "WorktreeAddBranch", "guard rejects before git worktree add")
}
```

Also update `TestCreateChild_AdoptMainWorktree_IgnoresDeletedAdoption` (already present) so its `ListFn` row carries a matching branch:

```go
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "old", RepoID: "r1", Branch: "develop", Status: domain.WorkspaceStatusDeleted},
			}, nil
		},
```

Delete the old `TestCreateChild_AdoptMainWorktree_RejectsSecondAdoption` (replaced by `..._AdoptPath` above). The two non-duplicate adopt tests (`TestCreateChild_AdoptMainWorktreeUnchanged`, `..._RevParseError`) already return `nil` from `ListFn` — leave them.

In `api/internal/api/libs/status_test.go`, find the `worktree.ErrParentLocked` 409 case row and add a sibling row:

```go
		{
			name:   "duplicate branch workspace is conflict",
			err:    worktree.ErrBranchWorkspaceExists,
			status: http.StatusConflict,
		},
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd api && go test ./internal/app/usecases/worktree/ ./internal/api/libs/ 2>&1 | tail`
Expected: FAIL — `undefined: worktree.ErrBranchWorkspaceExists` (compile error), since the sentinel/helper don't exist yet.

- [ ] **Step 3: Rename the sentinel**

In `api/internal/app/usecases/worktree/worktree_errors.go`, replace the `ErrDefaultWorkspaceExists` block with:

```go
// ErrBranchWorkspaceExists is returned when a create would produce a second
// workspace for a branch that a non-deleted workspace already holds in the repo.
// A branch can be checked out in at most one worktree, so the repo keeps at most
// one workspace per branch (the default workspace reserves the default branch).
// The guard runs before any git work, so a duplicate is rejected cleanly instead
// of failing midway with a raw git error. Handlers map it to HTTP 409.
var ErrBranchWorkspaceExists = errors.New("usecases: a workspace already exists for this branch")
```

- [ ] **Step 4: Generalize the guard in `worktree.go`**

In `api/internal/app/usecases/worktree/worktree.go`, (a) remove the guard block currently at the top of `adoptMainWorktree` (the `mainWorktreeAdopted` call and its rejection), restoring `adoptMainWorktree` to start directly at `startSha, err := u.git.RevParse(...)`. (b) Replace the `mainWorktreeAdopted` helper with `branchWorkspaceExists`:

```go
// branchWorkspaceExists reports whether a non-deleted workspace already holds
// this branch in the repo. A branch can be checked out in at most one worktree,
// so the repo keeps at most one workspace per branch.
func (u *worktreeUsecase) branchWorkspaceExists(
	ctx context.Context,
	repoID string,
	branch string,
) (bool, error) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return false, fmt.Errorf("create child: list workspaces: %w", err)
	}
	for _, w := range all {
		if w.RepoID == repoID &&
			w.Branch == branch &&
			w.Status != domain.WorkspaceStatusDeleted {
			return true, nil
		}
	}
	return false, nil
}
```

(c) Add the check at the top of `CreateChild`, immediately AFTER the `if in.RepoPath == "" { ... }` virtual-repo short-circuit and BEFORE the `if in.ParentID == "" && in.Branch == in.ParentBranch {` adopt branch:

```go
	// At most one non-deleted workspace per (repo, branch). Reject a duplicate on
	// both the adopt and child paths before any git work, so it surfaces as a
	// clean 409 rather than a raw git "branch already exists" error.
	exists, err := u.branchWorkspaceExists(ctx, in.RepoID, in.Branch)
	if err != nil {
		return domain.Workspace{}, err
	}
	if exists {
		return domain.Workspace{}, fmt.Errorf("%w (repo %s, branch %q)", ErrBranchWorkspaceExists, in.RepoID, in.Branch)
	}
```

Note: the first statement in `CreateChild` after this is `wsID := uuid.NewString()` (the existing child path). Insert the block above the adopt-branch `if`, which precedes that.

- [ ] **Step 5: Update the 409 mapping**

In `api/internal/api/libs/status.go`, in the conflict block that lists `worktree.ErrRebaseNonLeaf` / `worktree.ErrChildHasChildren`, replace `worktree.ErrDefaultWorkspaceExists` with `worktree.ErrBranchWorkspaceExists`.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd api && go test ./internal/app/usecases/worktree/ ./internal/api/libs/`
Expected: `ok` for both packages.

- [ ] **Step 7: Commit**

```bash
git add api/internal/app/usecases/worktree/worktree.go api/internal/app/usecases/worktree/worktree_errors.go api/internal/app/usecases/worktree/worktree_test.go api/internal/api/libs/status.go api/internal/api/libs/status_test.go
git commit -m "fix(workspace): reject duplicate-branch create on all paths (ErrBranchWorkspaceExists)"
```

---

### Task 2: Synchronous handler pre-check → 409

Today a create error is async (the 202 is already sent). Add the same uniqueness check synchronously in the create handler so a duplicate returns 409 before the 202.

**Files:**
- Modify: `api/internal/api/v0/endpoints/workspaces/handlers/crud.go`
- Test: `api/internal/api/v0/endpoints/workspaces/handlers/crud_test.go`

**Interfaces:**
- Consumes: `h.reader.List(ctx) ([]domain.Workspace, error)`; `h.repos.FindByKey`; `worktree.ErrBranchWorkspaceExists`; `libs.StatusAndMessage`.
- Produces: behavior only — `Create` returns 409 synchronously on a duplicate branch.

- [ ] **Step 1: Write the failing test**

In `api/internal/api/v0/endpoints/workspaces/handlers/crud_test.go`, add (match the existing test file's harness for building `Handlers` and issuing requests; reuse its fakes):

```go
func TestCreate_DuplicateBranch_Returns409(t *testing.T) {
	h, deps := newTestHandlers(t) // existing helper in this package's tests
	deps.reader.ListFn = func(_ context.Context) ([]domain.Workspace, error) {
		return []domain.Workspace{{ID: "w1", RepoID: "r1", Branch: "develop"}}, nil
	}
	deps.repos.repo = &domain.Repository{ID: "r1", ProjectID: "p1", Path: "/repo", DefaultBranch: "develop"}

	rec := deps.do(t, h, http.MethodPost,
		"/v0/projects/p1/repos/r1/workspaces",
		`{"branch":"develop"}`)

	require.Equal(t, http.StatusConflict, rec.Code)
}
```

If the package's test harness differs (helper names), mirror the existing `crud_test.go` setup exactly — the assertion that matters is: POST a branch already present in `reader.List` → `http.StatusConflict`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd api && go test ./internal/api/v0/endpoints/workspaces/handlers/ -run TestCreate_DuplicateBranch -v`
Expected: FAIL — currently returns 202 (the duplicate check isn't in the sync path).

- [ ] **Step 3: Add the sync check in `Create`**

In `api/internal/api/v0/endpoints/workspaces/handlers/crud.go`, in `Create`, AFTER `in, err := h.buildCreateInput(...)` (and its error handling) and BEFORE `libs.WriteAccepted(c)`, insert:

```go
	dup, err := h.branchTaken(c.Request.Context(), in.RepoID, in.Branch)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	if dup {
		libs.WriteErr(c, http.StatusConflict, "a workspace already exists for this branch")
		return
	}
```

Add the helper to the same file:

```go
// branchTaken reports whether a non-deleted workspace already holds the branch
// in the repo, so the create handler can return 409 synchronously instead of
// letting the async CreateChild reject after the 202 (where the error is only
// best-effort logged). The usecase guard remains the authoritative backstop.
func (h *Handlers) branchTaken(ctx context.Context, repoID, branch string) (bool, error) {
	all, err := h.reader.List(ctx)
	if err != nil {
		return false, err
	}
	for _, w := range all {
		if w.RepoID == repoID && w.Branch == branch && w.Status != domain.WorkspaceStatusDeleted {
			return true, nil
		}
	}
	return false, nil
}
```

Add the `domain` import if not present: `"github.com/char2cs/crowbar/api/internal/domain"`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd api && go test ./internal/api/v0/endpoints/workspaces/handlers/ -run TestCreate_DuplicateBranch -v`
Expected: PASS. Then run the whole package: `go test ./internal/api/v0/endpoints/workspaces/handlers/` → `ok`.

- [ ] **Step 5: Commit**

```bash
git add api/internal/api/v0/endpoints/workspaces/handlers/crud.go api/internal/api/v0/endpoints/workspaces/handlers/crud_test.go
git commit -m "feat(workspace): 409 synchronously on duplicate-branch create"
```

---

### Task 3: Integration regression for a non-default duplicate branch

Extends the real-daemon regression suite to cover a non-default branch duplicate (the default case is already covered by `TestRegression_DuplicateDefaultBranchWorkspace`).

**Files:**
- Modify: `api/tests/regressions_test.go`

**Interfaces:**
- Consumes: `newHarness`, `importProject`, `listWorkspaces`, `h.raw`, `workspaceDTO{Branch string}` (existing test helpers). Default branch of the fixture repo is `"main"`.

- [ ] **Step 1: Write the failing test**

Append to `api/tests/regressions_test.go`:

```go
// TestRegression_DuplicateNonDefaultBranchWorkspace proves the one-per-branch
// invariant also covers child branches: once a workspace exists for a branch,
// creating a second workspace on that same branch is rejected (409 sync) and
// never persists a duplicate.
func TestRegression_DuplicateNonDefaultBranchWorkspace(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	// Create a child workspace on a fresh branch (async 202).
	_ = h.raw(http.MethodPost, base+"/workspaces",
		map[string]string{"branch": "feature/dup"}, http.StatusAccepted).Body.Close()

	countBranch := func(name string) int {
		n := 0
		for _, w := range listWorkspaces(t, h, imported.projectID, imported.repoID) {
			if w.Branch == name {
				n++
			}
		}
		return n
	}
	require.Eventually(t, func() bool { return countBranch("feature/dup") == 1 },
		3*time.Second, 100*time.Millisecond, "first feature/dup workspace should land")

	// A second create on the same branch is rejected synchronously with 409.
	resp := h.raw(http.MethodPost, base+"/workspaces",
		map[string]string{"branch": "feature/dup"}, http.StatusConflict)
	_ = resp.Body.Close()

	// And never produces a duplicate.
	require.Never(t, func() bool { return countBranch("feature/dup") > 1 },
		2*time.Second, 100*time.Millisecond, "no duplicate feature/dup workspace")
}
```

- [ ] **Step 2: Run the test to verify it passes (and is meaningful)**

Run: `cd api && go test -tags integration ./tests/ -run 'TestRegression_DuplicateNonDefaultBranchWorkspace' -count=1`
Expected: `ok`. (Depends on Tasks 1–2; if run before them, the second POST returns 202 not 409 → FAIL, which confirms the test exercises the new behavior.)

- [ ] **Step 3: Commit**

```bash
git add api/tests/regressions_test.go
git commit -m "test(workspace): integration regression for duplicate non-default branch"
```

---

### Task 4: Expose `defaultBranch` on the sidebar `Repo` type

The default workspace is filtered out of `repo.workspaces`, so the frontend can't see the default branch name. Add it so live validation can catch a collision with the default branch.

**Files:**
- Modify: `web/src/lib/store/sidebar.ts` (add field to `Repo`)
- Modify: `web/src/lib/store/build-repo-tree.ts` (populate in `toSidebarRepo`)
- Test: `web/src/__tests__/lib/store/build-repo-tree.test.ts`

**Interfaces:**
- Produces: `Repo.defaultBranch?: string` (the branch of the `isDefault` workspace, when present).

- [ ] **Step 1: Write the failing test**

In `web/src/__tests__/lib/store/build-repo-tree.test.ts` (create if absent, mirroring existing test style), add:

```ts
import { describe, it, expect } from 'vitest'
import { toSidebarRepo } from '@/lib/store/build-repo-tree'
import type { RepoDTO, WorkspaceDTO } from '@/lib/types'

function ws(over: Partial<WorkspaceDTO> & { id: string }): WorkspaceDTO {
  return {
    id: over.id, repoId: 'r1', projectId: 'p1', branch: 'main', parentId: '',
    forkPointSha: '', status: 'new', working: false, lastError: '', added: 0,
    deleted: 0, mergeStrategy: 'merge', canMergeLocally: false, mergeConflicts: false,
    parentBranch: '', prUrl: '', prTitle: '', prTargetBranch: '', ...over,
  } as WorkspaceDTO
}
const repo: RepoDTO = { id: 'r1', projectId: 'p1', name: 'crowbar' } as RepoDTO

describe('toSidebarRepo defaultBranch', () => {
  it('sets defaultBranch from the isDefault workspace', () => {
    const out = toSidebarRepo(repo, [
      ws({ id: 'd', branch: 'develop', isDefault: true }),
      ws({ id: 'c', branch: 'feature/x' }),
    ])
    expect(out.defaultBranch).toBe('develop')
    expect(out.defaultWorkspaceId).toBe('d')
  })

  it('leaves defaultBranch undefined when there is no default workspace', () => {
    const out = toSidebarRepo(repo, [ws({ id: 'c', branch: 'feature/x' })])
    expect(out.defaultBranch).toBeUndefined()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/lib/store/build-repo-tree.test.ts`
Expected: FAIL — `out.defaultBranch` is `undefined` in the first case.

- [ ] **Step 3: Add the field + populate it**

In `web/src/lib/store/sidebar.ts`, add to the `Repo` interface (near `defaultWorkspaceId?: string`):

```ts
  /** Branch name of the default (main-worktree) workspace, surfaced on the repo
   *  header. Used by create-input validation to reserve the default branch. */
  defaultBranch?: string
```

In `web/src/lib/store/build-repo-tree.ts`, in `toSidebarRepo`, where `defaultWs` is already computed (`const defaultWs = repoWs.find((ws) => ws.isDefault)`), add the field to the returned object alongside `...(defaultWs ? { defaultWorkspaceId: defaultWs.id } : {})`:

```ts
    ...(defaultWs ? { defaultWorkspaceId: defaultWs.id, defaultBranch: defaultWs.branch } : {}),
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/lib/store/build-repo-tree.test.ts`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/store/sidebar.ts web/src/lib/store/build-repo-tree.ts web/src/__tests__/lib/store/build-repo-tree.test.ts
git commit -m "feat(sidebar): expose defaultBranch on Repo for create validation"
```

---

### Task 5: `findWorkspaceForBranch` predicate

A pure function returning the id of the workspace already holding a branch (including the default), or null. The create input uses it.

**Files:**
- Create: `web/src/lib/workspace/branch-workspace.ts`
- Test: `web/src/__tests__/lib/workspace/branch-workspace.test.ts`

**Interfaces:**
- Consumes: `Repo` (with `defaultBranch`, `defaultWorkspaceId`, `workspaces[]`).
- Produces: `findWorkspaceForBranch(repo: Repo, branch: string): string | null`.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/lib/workspace/branch-workspace.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { findWorkspaceForBranch } from '@/lib/workspace/branch-workspace'
import type { Repo, Workspace } from '@/lib/store/sidebar'

function w(id: string, branch: string): Workspace {
  return { id, branch, status: 'new', added: 0, deleted: 0, working: false,
    canMergeLocally: false, mergeConflicts: false, lastError: '', age: '' } as Workspace
}
const repo: Repo = {
  id: 'r1', projectId: 'p1', name: 'crowbar',
  avatarLabel: 'C', avatarColor: 'bg-sky-700',
  defaultWorkspaceId: 'd', defaultBranch: 'develop',
  workspaces: [w('c1', 'feature/x'), w('c2', 'spike/y')],
} as Repo

describe('findWorkspaceForBranch', () => {
  it('matches an existing child branch', () => {
    expect(findWorkspaceForBranch(repo, 'feature/x')).toBe('c1')
  })
  it('matches the default branch via defaultWorkspaceId', () => {
    expect(findWorkspaceForBranch(repo, 'develop')).toBe('d')
  })
  it('returns null for a free branch', () => {
    expect(findWorkspaceForBranch(repo, 'feature/new')).toBeNull()
  })
  it('is case-sensitive', () => {
    expect(findWorkspaceForBranch(repo, 'Feature/X')).toBeNull()
  })
  it('trims surrounding whitespace before matching', () => {
    expect(findWorkspaceForBranch(repo, '  develop  ')).toBe('d')
  })
  it('returns null for an empty/whitespace branch', () => {
    expect(findWorkspaceForBranch(repo, '   ')).toBeNull()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/lib/workspace/branch-workspace.test.ts`
Expected: FAIL — module `@/lib/workspace/branch-workspace` not found.

- [ ] **Step 3: Implement the predicate**

Create `web/src/lib/workspace/branch-workspace.ts`:

```ts
import type { Repo } from '@/lib/store/sidebar'

/**
 * Returns the id of the workspace that already holds `branch` in this repo —
 * including the default (main-worktree) workspace — or null if the branch is
 * free. Exact, case-sensitive match (git branch semantics), scoped to the repo.
 * Used to enforce one-workspace-per-branch in the create input.
 */
export function findWorkspaceForBranch(repo: Repo, branch: string): string | null {
  const name = branch.trim()
  if (!name) return null
  if (repo.defaultBranch === name && repo.defaultWorkspaceId) {
    return repo.defaultWorkspaceId
  }
  const match = repo.workspaces.find((w) => w.branch === name)
  return match ? match.id : null
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/lib/workspace/branch-workspace.test.ts`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/workspace/branch-workspace.ts web/src/__tests__/lib/workspace/branch-workspace.test.ts
git commit -m "feat(workspace): findWorkspaceForBranch predicate for create validation"
```

---

### Task 6: Live validation + "open it" hint in `WorkspaceInlineInput`

The input gains an optional resolver and an open callback. While the typed branch collides, confirm is disabled and a clickable hint is shown.

**Files:**
- Modify: `web/src/components/layout/workspace-inline-input.tsx`
- Test: `web/src/__tests__/components/layout/workspace-inline-input.test.tsx`

**Interfaces:**
- Consumes: `findWorkspaceForBranch` is NOT imported here; the resolver is injected so the component stays pure. New props:
  - `resolveExisting?: (branch: string) => string | null`
  - `onOpenExisting?: (wsId: string) => void`
- Produces: same `onConfirm`/`onCancel` contract; confirm is suppressed while `resolveExisting(value)` is non-null.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/layout/workspace-inline-input.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { WorkspaceInlineInput } from '@/components/layout/workspace-inline-input'

describe('WorkspaceInlineInput collision handling', () => {
  function setup() {
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    const onOpenExisting = vi.fn()
    const resolveExisting = (b: string) => (b.trim() === 'develop' ? 'ws-default' : null)
    render(
      <WorkspaceInlineInput
        onConfirm={onConfirm}
        onCancel={onCancel}
        resolveExisting={resolveExisting}
        onOpenExisting={onOpenExisting}
      />,
    )
    return { onConfirm, onCancel, onOpenExisting }
  }

  it('shows the hint and suppresses confirm for an existing branch', () => {
    const { onConfirm } = setup()
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: 'develop' } })
    expect(screen.getByText(/already has a workspace/i)).toBeInTheDocument()
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('clicking the hint opens the existing workspace', () => {
    const { onOpenExisting } = setup()
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: 'develop' } })
    fireEvent.click(screen.getByText(/already has a workspace/i))
    expect(onOpenExisting).toHaveBeenCalledWith('ws-default')
  })

  it('confirms normally for a free branch', () => {
    const { onConfirm } = setup()
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: 'feature/new' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onConfirm).toHaveBeenCalledWith('feature/new')
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/components/layout/workspace-inline-input.test.tsx`
Expected: FAIL — no hint element; `onConfirm` is called for `develop` (collision not handled).

- [ ] **Step 3: Implement the collision handling**

Replace `web/src/components/layout/workspace-inline-input.tsx` with:

```tsx
import { useEffect, useRef, useState } from 'react'

interface WorkspaceInlineInputProps {
  defaultValue?: string
  placeholder?: string
  onConfirm: (value: string) => void
  onCancel: () => void
  /** Resolve a branch to the id of the workspace already holding it, or null. */
  resolveExisting?: (branch: string) => string | null
  /** Navigate to the existing workspace when the user clicks the hint. */
  onOpenExisting?: (wsId: string) => void
}

export function WorkspaceInlineInput({
  defaultValue = '',
  placeholder = 'branch-name',
  onConfirm,
  onCancel,
  resolveExisting,
  onOpenExisting,
}: WorkspaceInlineInputProps) {
  const [value, setValue] = useState(defaultValue)
  const ref = useRef<HTMLInputElement>(null)
  // Prevents blur from double-firing after Enter/Escape already handled
  const handledRef = useRef(false)

  useEffect(() => {
    ref.current?.focus()
    ref.current?.select()
  }, [])

  const existingWsId = resolveExisting?.(value) ?? null

  function tryConfirm() {
    // A collision suppresses create — the user opens the existing one or renames.
    if (existingWsId) return
    if (value.trim()) onConfirm(value.trim())
    else onCancel()
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') {
      e.preventDefault()
      handledRef.current = true
      tryConfirm()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      handledRef.current = true
      onCancel()
    }
  }

  function handleBlur() {
    if (handledRef.current) return
    tryConfirm()
  }

  return (
    <div className="flex min-w-0 flex-1 flex-col">
      <input
        ref={ref}
        type="text"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        onBlur={handleBlur}
        placeholder={placeholder}
        className="min-w-0 flex-1 bg-transparent font-mono text-[13px] outline-none placeholder:text-muted-foreground/40"
      />
      {existingWsId && (
        <button
          type="button"
          // Use mousedown so it fires before the input's blur cancels the create.
          onMouseDown={(e) => {
            e.preventDefault()
            handledRef.current = true
            onOpenExisting?.(existingWsId)
          }}
          className="mt-0.5 text-left font-mono text-[11px] text-muted-foreground/70 hover:text-foreground"
        >
          {`'${value.trim()}' already has a workspace — open it`}
        </button>
      )}
    </div>
  )
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/components/layout/workspace-inline-input.test.tsx`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/workspace-inline-input.tsx web/src/__tests__/components/layout/workspace-inline-input.test.tsx
git commit -m "feat(workspace): live duplicate-branch validation + open-existing hint in create input"
```

---

### Task 7: Wire the validation into the workspace tree

Pass the per-repo resolver and an open-existing navigator into the two `WorkspaceInlineInput` render sites (repo-level create in `workspace-tree.tsx`, per-workspace create in `workspace-tree-item.tsx`).

**Files:**
- Modify: `web/src/components/layout/workspace-tree.tsx`
- Modify: `web/src/components/layout/workspace-tree-item.tsx`

**Interfaces:**
- Consumes: `findWorkspaceForBranch(repo, branch)` (Task 5); `WorkspaceInlineInput` props `resolveExisting`, `onOpenExisting` (Task 6); existing `handleWorkspaceClick(wsId, projectId, repoId)` in `workspace-tree.tsx`; the `repo` object (has `id`, `projectId`, `workspaces`, `defaultBranch`, `defaultWorkspaceId`).

- [ ] **Step 1: Wire `workspace-tree.tsx` (repo-level create input)**

In `web/src/components/layout/workspace-tree.tsx`, add the import:

```ts
import { findWorkspaceForBranch } from '@/lib/workspace/branch-workspace'
```

Find the `<WorkspaceInlineInput onConfirm={confirmCreate} onCancel={cancelCreate} />` used for the repo-level create (inside the `creatingChildOf?.repoId === repo.id && creatingChildOf?.parentId === repo.defaultWorkspaceId` block) and replace it with:

```tsx
                            <WorkspaceInlineInput
                              onConfirm={confirmCreate}
                              onCancel={cancelCreate}
                              resolveExisting={(b) => findWorkspaceForBranch(repo, b)}
                              onOpenExisting={(wsId) => {
                                cancelCreate()
                                if (repo.projectId) handleWorkspaceClick(wsId, repo.projectId, repo.id)
                              }}
                            />
```

- [ ] **Step 2: Wire `workspace-tree-item.tsx` (per-workspace create input)**

In `web/src/components/layout/workspace-tree-item.tsx`, this component renders a `WorkspaceInlineInput` for the per-workspace "Add child" create. It already has access to `repoId`, `projectId`, and `onWorkspaceClick` (the same handler threaded down from `workspace-tree.tsx`), plus the `repo` is reachable via the sidebar store. Add the import:

```ts
import { findWorkspaceForBranch } from '@/lib/workspace/branch-workspace'
import { useSidebarStore } from '@/lib/store/sidebar'
```

At the top of the component body, resolve the repo for validation:

```ts
  const repo = useSidebarStore((s) => s.repos.find((r) => r.id === repoId))
```

Then on the `WorkspaceInlineInput` it renders, add:

```tsx
              resolveExisting={(b) => (repo ? findWorkspaceForBranch(repo, b) : null)}
              onOpenExisting={(wsId) => onWorkspaceClick(wsId, projectId, repoId)}
```

(Keep the existing `onConfirm`/`onCancel` props as they are.)

- [ ] **Step 3: Type-check and run the frontend gate**

Run: `cd web && npx tsc --noEmit && npx vitest run src/__tests__/components/layout/ src/__tests__/lib/workspace/ src/__tests__/lib/store/build-repo-tree.test.ts`
Expected: tsc clean; all listed suites PASS.

- [ ] **Step 4: Manual Tauri check (per project convention)**

Reload the running app; in a repo with a `develop` default and a `feature/*` child, click "Add child workspace" and type an existing branch name (e.g. `develop`, then the child's branch). Verify: the hint `'<branch>' already has a workspace — open it` appears, Enter does nothing, and clicking the hint navigates to that workspace. Type a fresh name and verify create still works.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/workspace-tree.tsx web/src/components/layout/workspace-tree-item.tsx
git commit -m "feat(workspace): wire duplicate-branch validation into the workspace tree create inputs"
```

---

## Final verification (after all tasks)

- [ ] Backend: `cd api && gofmt -l internal tests && go vet ./internal/... && go test ./internal/... && go test -tags integration ./tests/ -run 'Workspace|Duplicate'`
- [ ] Frontend: `cd web && npx tsc --noEmit && npx eslint src/lib/workspace src/components/layout/workspace-inline-input.tsx src/lib/store/build-repo-tree.ts && npx vitest run`
- [ ] Manual Tauri: collision hint + open-existing + free-branch create all behave per Task 7 Step 4.
