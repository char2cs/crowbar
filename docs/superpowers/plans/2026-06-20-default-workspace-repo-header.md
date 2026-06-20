# Default Workspace & Repo Header Implementation Plan (revised)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the original-repo-folder workspace from the workspace tree and make it reachable by clicking the repo header, by flagging it `IsDefault`, exposing that on the wire, and resolving it to its real workspace id on the frontend.

**Architecture:** The git main worktree (`worktreePath == repo.Path`) is flagged `IsDefault` at import, persisted via the workspace command path, and exposed on `WorkspaceDTO`. The frontend pulls the `isDefault` workspace out of the sidebar tree (capturing its id + branch on the `Repo`) and the repo header navigates to that **real** workspace id — so every existing scoped route and WS stream works unchanged. There is **no** `"default"` URL alias and **no** backend list/snapshot exclusion.

**Tech Stack:** Go (backend), TypeScript/React/Zustand (frontend), TanStack Router, Gin, Vitest.

## Global Constraints

- Go style: errors wrapped `fmt.Errorf("...: %w", err)`, no naked returns; gofmt with tabs.
- Backend tests live beside source as `_test.go`. Use the REAL existing helpers — they are named in each task. Do NOT invent helpers.
- Frontend test files live in `web/src/__tests__/` mirroring `web/src/`; use `@/` imports. The two test files touched here ALREADY EXIST — edit them, never recreate.
- No hardcoded colors — use CSS-variable tokens and `@/components/ui/*`.
- Narrow Zustand selectors: `useXxxStore((s) => s.field)`.
- Verify backend with `cd api && go build ./... && go test ./...`; frontend with `cd web && pnpm tsc --noEmit && pnpm test --run`.
- Commit 8a08def already added `domain.Workspace.IsDefault`, `domain.Repository.MainBranch`, and `workspace.CreateInput.IsDefault`. Keep the two `IsDefault` additions; the `MainBranch` addition is reverted in Task 1.

---

### Task 1: Backend — flag the default at import, persist it, revert MainBranch

**Files:**
- Modify: `api/internal/domain/repository.go` (revert MainBranch)
- Modify: `api/internal/app/usecases/project/project_import.go:397-425` (`adoptOneWorktree`)
- Modify: `api/internal/app/repositories/workspace/internal/commands/create.go`
- Modify: `api/internal/app/repositories/workspace/workspace.go:247-258` (`Create`)
- Modify: `api/internal/app/usecases/mocks/mocks.go` (`WorkspaceRepo.Create`)
- Test: `api/internal/app/usecases/project/project_import_test.go`
- Test: `api/internal/app/repositories/workspace/workspace_test.go`

**Interfaces:**
- Consumes: `workspace.CreateInput.IsDefault` (already exists), `domain.Workspace.IsDefault` (already exists).
- Produces: a created workspace round-trips `IsDefault` through persistence; import flags the main-worktree workspace `IsDefault==true`.

- [ ] **Step 1: Revert MainBranch from the domain**

In `api/internal/domain/repository.go` delete the line:

```go
	MainBranch    string `json:"mainBranch,omitempty"`
```

(Leave `DefaultBranch` and everything else.)

- [ ] **Step 2: Write the failing import test**

In `api/internal/app/usecases/project/project_import_test.go`, add (the `newImport`/`mocks`/`gitengine`/`require`/`assert` imports are already present):

```go
func TestImportRepo_FlagsMainWorktreeAsDefault(t *testing.T) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	prov := mocks.NewProviderEngine()

	require.NoError(t, projects.Save(context.Background(),
		domain.Project{ID: "proj-1", Name: "P", Path: "/root"}))

	// The main worktree is the entry whose Path == repo.Path (/root/repoA).
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/root/repoA", Branch: "develop", Head: "h1"},
		{Path: "/root/wt-x", Branch: "feature/x", Head: "h2"},
	}
	prov.Protected = []string{"staging"} // a protected branch with no local worktree

	uc := project.NewImport(project.ImportDeps{
		Projects: projects, Repos: repos, Workspaces: ws, Git: git, Provider: prov,
		Discover:  func(string, int) ([]string, error) { return nil, nil },
		RefRunner: func(string) defaultbranch.RefRunner { return func(...string) (string, bool) { return "", false } },
		Now:       func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:      statExists,
	})

	_, err := uc.ImportRepo(context.Background(), "proj-1", "/root/repoA")
	require.NoError(t, err)

	byBranch := map[string]bool{}
	for _, c := range ws.Created {
		byBranch[c.Branch] = c.IsDefault
	}
	assert.True(t, byBranch["develop"], "main-worktree workspace must be IsDefault")
	assert.False(t, byBranch["feature/x"], "non-main worktree must not be IsDefault")
	assert.False(t, byBranch["staging"], "protected stub must not be IsDefault")
}
```

- [ ] **Step 3: Run it — expect failure**

```bash
cd api && go test ./internal/app/usecases/project/... -run TestImportRepo_FlagsMainWorktreeAsDefault -v
```

Expected: FAIL (all `IsDefault` false — flag not set, and the mock doesn't copy it).

- [ ] **Step 4: Set IsDefault in adoptOneWorktree**

In `api/internal/app/usecases/project/project_import.go`, in `adoptOneWorktree`, add `IsDefault` to the `CreateInput` literal (the main worktree is the one whose path equals the repo root):

```go
	in := workspace.CreateInput{
		ID:           uuid.NewString(),
		RepoID:       repo.ID,
		ProjectID:    repo.ProjectID,
		Branch:       wt.Branch,
		WorktreePath: wt.Path,
		ForkPointSha: u.forkPoint(ctx, repo, wt.Branch),
		Protected:    locked[wt.Branch],
		IsDefault:    wt.Path == repo.Path,
	}
```

Do NOT change `importProtectedBranchStubs` (its stubs stay non-default). Do NOT pre-fetch worktrees or use `worktrees[0]`.

- [ ] **Step 5: Copy IsDefault in the WorkspaceRepo mock**

In `api/internal/app/usecases/mocks/mocks.go`, in `WorkspaceRepo.Create`, add `IsDefault: in.IsDefault,` to the `domain.Workspace{...}` literal it appends to `r.Created`.

- [ ] **Step 6: Run the import test — expect pass**

```bash
cd api && go test ./internal/app/usecases/project/... -run TestImportRepo_FlagsMainWorktreeAsDefault -v
```

Expected: PASS.

- [ ] **Step 7: Write the failing persistence test**

In `api/internal/app/repositories/workspace/workspace_test.go`, add (uses the existing `newRepo` helper):

```go
func TestCreate_PersistsIsDefault(t *testing.T) {
	ctx, repo := newRepo(t)

	created, err := repo.Create(ctx, workspace.CreateInput{
		ID:        "w-default",
		RepoID:    "r1",
		ProjectID: "p1",
		Branch:    "develop",
		IsDefault: true,
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	assert.True(t, created.IsDefault, "Create must return IsDefault")

	got, err := repo.Get(ctx, "w-default")
	require.NoError(t, err)
	assert.True(t, got.IsDefault, "Get must round-trip IsDefault")
}
```

- [ ] **Step 8: Run it — expect failure**

```bash
cd api && go test ./internal/app/repositories/workspace/... -run TestCreate_PersistsIsDefault -v
```

Expected: FAIL (command path drops `IsDefault`).

- [ ] **Step 9: Thread IsDefault through the command**

In `api/internal/app/repositories/workspace/internal/commands/create.go`:

Add the field to the struct (after `Protected bool`):

```go
	IsDefault     bool
```

And set it on the returned aggregate in `EmitEvent` (add inside the `return domain.Workspace{...}` literal, e.g. after `ParentID: c.ParentID,`):

```go
		IsDefault:     c.IsDefault,
```

- [ ] **Step 10: Pass IsDefault from Create**

In `api/internal/app/repositories/workspace/workspace.go`, in `Create`'s `SendWait(ctx, commands.CreateWorkspace{...})`, add (after `Protected: in.Protected,`):

```go
			IsDefault:     in.IsDefault,
```

- [ ] **Step 11: Run both backend tests + the package suites**

```bash
cd api && go test ./internal/app/usecases/project/... ./internal/app/repositories/workspace/... -v
```

Expected: both new tests PASS, no regressions (incl. `TestImport_WorktreeListError_IsTolerated`, `TestImportRepo_AdoptsDefaultBranchWorkspace`).

- [ ] **Step 12: Build + commit**

```bash
cd api && go build ./... && cd ..
git add api/internal/domain/repository.go api/internal/app/usecases/project/ api/internal/app/repositories/workspace/ api/internal/app/usecases/mocks/mocks.go
git commit -m "feat(workspace): flag main-worktree workspace IsDefault and persist it; revert MainBranch"
```

---

### Task 2: Backend — expose IsDefault on WorkspaceDTO

**Files:**
- Modify: `api/internal/api/v0/dto/workspace.go`
- Test: `api/internal/api/v0/dto/workspace_test.go`
- Modify: `web/src/lib/types.ts`

**Interfaces:**
- Consumes: `domain.Workspace.IsDefault` (Task 1).
- Produces: `WorkspaceDTO.isDefault` on the wire (Go + TS) for the frontend (Tasks 3–5).

- [ ] **Step 1: Write the failing DTO test**

In `api/internal/api/v0/dto/workspace_test.go`, add (match the file's existing import style; `workspace.MergeEligibility{}` is the zero eligibility):

```go
func TestWorkspaceDTOFrom_MapsIsDefault(t *testing.T) {
	got := dto.WorkspaceDTOFrom(
		domain.Workspace{ID: "w1", RepoID: "r1", ProjectID: "p1", IsDefault: true},
		workspace.MergeEligibility{},
	)
	assert.True(t, got.IsDefault)

	got2 := dto.WorkspaceDTOFrom(
		domain.Workspace{ID: "w2", RepoID: "r1", ProjectID: "p1"},
		workspace.MergeEligibility{},
	)
	assert.False(t, got2.IsDefault)
}
```

- [ ] **Step 2: Run it — expect failure**

```bash
cd api && go test ./internal/api/v0/dto/... -run TestWorkspaceDTOFrom_MapsIsDefault -v
```

Expected: FAIL (`got.IsDefault` undefined field / always false).

- [ ] **Step 3: Add the field + mapping**

In `api/internal/api/v0/dto/workspace.go`, add to the `WorkspaceDTO` struct (after `LastError`):

```go
	IsDefault       bool                    `json:"isDefault,omitempty"`
```

And in `WorkspaceDTOFrom`'s returned literal (after `LastError: w.LastError,`):

```go
		IsDefault:       w.IsDefault,
```

- [ ] **Step 4: Run it — expect pass**

```bash
cd api && go test ./internal/api/v0/dto/... -run TestWorkspaceDTOFrom_MapsIsDefault -v
```

Expected: PASS.

- [ ] **Step 5: Add isDefault to the frontend WorkspaceDTO**

In `web/src/lib/types.ts`, in `interface WorkspaceDTO`, add (optional — Go omits it when false):

```ts
  isDefault?: boolean
```

- [ ] **Step 6: Type-check + commit**

```bash
cd api && go build ./... && cd ../web && pnpm tsc --noEmit && cd ..
git add api/internal/api/v0/dto/workspace.go api/internal/api/v0/dto/workspace_test.go web/src/lib/types.ts
git commit -m "feat(api): expose isDefault on WorkspaceDTO (wire + frontend type)"
```

---

### Task 3: Frontend — pull the default workspace out of the tree

**Files:**
- Modify: `web/src/lib/store/sidebar.ts` (`Repo`)
- Modify: `web/src/lib/store/build-repo-tree.ts` (`toSidebarRepo`)
- Test: `web/src/__tests__/lib/store/build-repo-tree.test.ts` (EXISTS — edit)

**Interfaces:**
- Consumes: `WorkspaceDTO.isDefault` (Task 2).
- Produces: `Repo.defaultWorkspaceId`, `Repo.defaultWorkspaceBranch`; `Repo.workspaces` excludes the default. Consumed by Tasks 4–5.

- [ ] **Step 1: Add the Repo fields**

In `web/src/lib/store/sidebar.ts`, in `interface Repo`, add:

```ts
  /** Real id of the IsDefault workspace (the imported repo folder); used by the repo header. */
  defaultWorkspaceId?: string
  /** Branch checked out in the default workspace; shown as the repo-header subtitle. */
  defaultWorkspaceBranch?: string
```

- [ ] **Step 2: Write the failing test**

In `web/src/__tests__/lib/store/build-repo-tree.test.ts`, add inside the existing `describe('buildRepoTree', …)` block (the `ws()` factory already accepts a `Partial<WorkspaceDTO>` override, so `{ isDefault: true }` works):

```ts
  it('pulls the isDefault workspace out of the tree and onto the repo', () => {
    const tree = buildRepoTree(
      [repo('r1', 'crowbar')],
      [
        ws('w-default', 'r1', { isDefault: true, branch: 'develop' }),
        ws('w-child', 'r1', { branch: 'feature/x' }),
      ],
    )
    expect(tree[0].workspaces.map((w) => w.id)).toEqual(['w-child'])
    expect(tree[0].defaultWorkspaceId).toBe('w-default')
    expect(tree[0].defaultWorkspaceBranch).toBe('develop')
  })

  it('leaves the default fields undefined when no workspace is the default', () => {
    const tree = buildRepoTree([repo('r1', 'crowbar')], [ws('w1', 'r1')])
    expect(tree[0].defaultWorkspaceId).toBeUndefined()
    expect(tree[0].defaultWorkspaceBranch).toBeUndefined()
  })
```

- [ ] **Step 3: Run it — expect failure**

```bash
cd web && pnpm test --run src/__tests__/lib/store/build-repo-tree.test.ts
```

Expected: FAIL (default still in `workspaces`; fields undefined).

- [ ] **Step 4: Update toSidebarRepo**

In `web/src/lib/store/build-repo-tree.ts`, replace `toSidebarRepo` with:

```ts
export function toSidebarRepo(repo: RepoDTO, workspaces: WorkspaceDTO[]): Repo {
  const repoWs = workspaces.filter((ws) => ws.repoId === repo.id)
  const defaultWs = repoWs.find((ws) => ws.isDefault)
  return {
    id: repo.id,
    projectId: repo.projectId,
    name: repo.name,
    avatarLabel: repo.avatarLabel || repoAvatarLabel(repo.name),
    avatarColor: repo.avatarColor || repoAvatarColor(repo.name),
    avatarURL: repoAvatarURL(repo),
    workspaces: repoWs.filter((ws) => !ws.isDefault).map(toSidebarWorkspace),
    ...(defaultWs ? { defaultWorkspaceId: defaultWs.id, defaultWorkspaceBranch: defaultWs.branch } : {}),
  }
}
```

- [ ] **Step 5: Run it — expect pass**

```bash
cd web && pnpm test --run src/__tests__/lib/store/build-repo-tree.test.ts
```

Expected: PASS (all existing cases still green).

- [ ] **Step 6: Type-check + commit**

```bash
cd web && pnpm tsc --noEmit && cd ..
git add web/src/lib/store/sidebar.ts web/src/lib/store/build-repo-tree.ts web/src/__tests__/lib/store/build-repo-tree.test.ts
git commit -m "feat(sidebar): exclude default workspace from tree, surface its id+branch on Repo"
```

---

### Task 4: Frontend — route guard accepts the default workspace id

**Files:**
- Modify: `web/src/lib/store/workspace-route-guard.ts`
- Test: `web/src/__tests__/lib/store/workspace-route-guard.test.ts` (EXISTS — edit)

**Interfaces:**
- Consumes: `Repo.defaultWorkspaceId` (Task 3).
- Produces: navigation to the default's real id is not redirected.

- [ ] **Step 1: Add the failing test case**

In `web/src/__tests__/lib/store/workspace-route-guard.test.ts`, add `defaultWorkspaceId: 'ws-default'` to the existing `REPOS[0]` object literal, then add:

```ts
test('does not redirect for the repo default workspace id (excluded from the tree)', () => {
  expect(shouldRedirectUnknownWorkspace('success', REPOS, 'ws-default')).toBe(false)
})
```

- [ ] **Step 2: Run it — expect failure**

```bash
cd web && pnpm test --run src/__tests__/lib/store/workspace-route-guard.test.ts
```

Expected: the new test FAILS (returns true); the others pass.

- [ ] **Step 3: Update the guard**

In `web/src/lib/store/workspace-route-guard.ts`, change the final return to also accept any repo's default workspace id:

```ts
  return !repos.some(
    (repo) =>
      repo.defaultWorkspaceId === wsId ||
      repo.workspaces.some((ws) => ws.id === wsId),
  )
```

- [ ] **Step 4: Run it — expect pass**

```bash
cd web && pnpm test --run src/__tests__/lib/store/workspace-route-guard.test.ts
```

Expected: PASS.

- [ ] **Step 5: Type-check + commit**

```bash
cd web && pnpm tsc --noEmit && cd ..
git add web/src/lib/store/workspace-route-guard.ts web/src/__tests__/lib/store/workspace-route-guard.test.ts
git commit -m "feat(route-guard): accept the repo default workspace id"
```

---

### Task 5: Frontend — repo header opens the default workspace

**Files:**
- Modify: `web/src/components/layout/workspace-tree.tsx`

**Interfaces:**
- Consumes: `Repo.defaultWorkspaceId`, `Repo.defaultWorkspaceBranch` (Task 3); the tree context's `creatingChildOf`/`startCreating`/`confirmCreate`/`cancelCreate` (existing).
- Produces: clicking the repo header opens the default workspace; hover shows the branch; `+` forks from it; chevron still collapses.

This task has no unit test (component wiring). Verify with `pnpm tsc --noEmit` and a live check in the running Tauri app (see Step 5).

- [ ] **Step 1: Widen the context + icon imports**

In `web/src/components/layout/workspace-tree.tsx`:
- Add `Plus` to the lucide-react import: `import { Plus, Settings } from 'lucide-react'`.
- Add the inline-input import: `import { WorkspaceInlineInput } from './workspace-inline-input'`.
- Widen the context destructure in `WorkspaceTreeInner`:

```tsx
  const { hoverTargetId, creatingChildOf, startCreating, confirmCreate, cancelCreate } =
    useWorkspaceTreeContext()
```

- [ ] **Step 2: Rebuild the repo header row**

Replace the repo header `div[role="button"]` block (the one with `onClick={() => useSidebarStore.getState().toggleRepo(repo.id)}` and its avatar/name/Settings/chevron children) with the structure below. Keep the existing avatar rendering exactly (the `repo.avatarURL?.startsWith('emoji:') ? … : repo.avatarURL ? <img/> : <span/>` block) — only its wrapping changes.

```tsx
<div
  className={cn(
    ROW_BASE,
    'group border-transparent text-foreground hover:bg-accent',
    activeWorkspaceId !== '' && activeWorkspaceId === repo.defaultWorkspaceId && 'bg-accent',
    isRepoDragOver && 'ring-1 ring-ring',
  )}
  data-repo-drop={repo.id}
>
  <button
    type="button"
    className="flex min-w-0 flex-1 items-center gap-2 text-left"
    onClick={() => {
      if (repo.projectId && repo.defaultWorkspaceId) {
        void navigate({
          to: '/ide/$projectId/$repoId/$wsId',
          params: { projectId: repo.projectId, repoId: repo.id, wsId: repo.defaultWorkspaceId },
        })
      }
    }}
    aria-label={`Open ${repo.name}`}
  >
    {/* ---- existing avatar rendering block goes here, unchanged ---- */}
    <span className="flex min-w-0 flex-col">
      <span className="min-w-0 truncate font-mono text-foreground">{repo.name}</span>
      {repo.defaultWorkspaceBranch && (
        <span className="min-w-0 truncate font-mono text-[11px] text-foreground/40 opacity-0 transition-opacity group-hover:opacity-100">
          {repo.defaultWorkspaceBranch}
        </span>
      )}
    </span>
  </button>

  {repo.defaultWorkspaceId && (
    <button
      type="button"
      aria-label="New workspace from default"
      className="hidden shrink-0 rounded-md p-1 text-foreground/50 hover:text-foreground group-hover:inline-flex"
      onClick={(e) => {
        e.stopPropagation()
        if (collapsedRepos.has(repo.id)) useSidebarStore.getState().toggleRepo(repo.id)
        startCreating(repo.id, repo.defaultWorkspaceId!)
      }}
    >
      <Plus className="size-3" />
    </button>
  )}

  <button
    type="button"
    aria-label="Repo settings"
    className="hidden shrink-0 rounded-md p-1 text-foreground/50 hover:text-foreground group-hover:inline-flex"
    onClick={(e) => {
      e.stopPropagation()
      useSidebarNavStore.getState().push({
        id: `repo-settings:${repo.id}`,
        title: repo.name,
        component: (
          <RepoSettingsPanel
            projectId={repo.projectId ?? ''}
            repoId={repo.id}
            repoName={repo.name}
          />
        ),
      })
    }}
  >
    <Settings className="size-3" />
  </button>

  <button
    type="button"
    aria-label={isCollapsed ? 'Expand repo' : 'Collapse repo'}
    className="inline-flex shrink-0 rounded-md p-1 text-foreground/30 hover:text-foreground"
    onClick={(e) => {
      e.stopPropagation()
      useSidebarStore.getState().toggleRepo(repo.id)
    }}
  >
    <svg
      aria-hidden="true"
      className={cn('size-3 transition-transform', !isCollapsed && 'rotate-90')}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
    >
      <path d="M6 3l5 5-5 5" />
    </svg>
  </button>
</div>
```

- [ ] **Step 3: Render the inline create input at repo level**

The default workspace has no tree row, so its inline create input must render under the header. In the `{!isCollapsed && ( … )}` block, before `{roots.map(…)}`, add:

```tsx
{creatingChildOf?.repoId === repo.id &&
  creatingChildOf?.parentId === repo.defaultWorkspaceId && (
    <div style={{ paddingLeft: 14 }}>
      <div className={cn(ROW_BASE, 'border-transparent text-foreground')}>
        <Plus className="size-4 shrink-0 text-foreground/30" />
        <WorkspaceInlineInput onConfirm={confirmCreate} onCancel={cancelCreate} />
      </div>
    </div>
  )}
```

- [ ] **Step 4: Type-check**

```bash
cd web && pnpm tsc --noEmit
```

Expected: no errors. Fix any unused-import or type issues.

- [ ] **Step 5: Live verification in Tauri (required before claiming done)**

Run the app and confirm in the running Tauri window: (a) the repo header no longer has a `develop`/default row beneath it duplicated as a workspace; (b) clicking the repo header opens the IDE with the file tree, git, and a terminal all working (not 404); (c) hovering the header reveals the branch subtitle; (d) the `+` shows an inline branch input under the header and creating it adds a top-level workspace; (e) the chevron still collapses/expands. Capture a screenshot.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/layout/workspace-tree.tsx
git commit -m "feat(workspace-tree): repo header opens default workspace, branch subtitle, fork button"
```

---

## Self-Review

**Spec coverage:**
- ✅ Revert MainBranch — Task 1 Step 1
- ✅ Flag main worktree IsDefault at import (path-based, stubs excluded) — Task 1 Steps 4
- ✅ Persist IsDefault via command path (commands + EmitEvent + Create + mock) — Task 1 Steps 5,9,10
- ✅ Expose isDefault on WorkspaceDTO (Go + TS) — Task 2
- ✅ Exclude default from tree, surface id+branch on Repo — Task 3
- ✅ Route guard accepts default id — Task 4
- ✅ Repo header click/subtitle/+/chevron/active + repo-level create input — Task 5
- ✅ No backend list/snapshot exclusion, no "default" alias — by omission (none added)

**Type consistency:** `IsDefault` (Go) / `isDefault` (TS) used consistently; `Repo.defaultWorkspaceId` + `Repo.defaultWorkspaceBranch` defined in Task 3, consumed in Tasks 4–5; real helpers verified against source (`newRepo`, `newImport`, `mocks.NewWorkspaceRepo`, `WorkspaceDTOFrom`, `ws()`/`repo()` factories, `startCreating`/`confirmCreate`).

**No invented helpers:** every test uses an existing helper confirmed present in the named file.
