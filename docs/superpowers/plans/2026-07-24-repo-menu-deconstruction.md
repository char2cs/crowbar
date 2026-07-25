# Repo Menu Deconstruction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the repo settings panel into an icon-editing Popover (on the avatar) and a branch-Import Dialog, and make import PR-aware so branches are parented under their PR base, creating missing ancestors up to a protected/default root.

**Architecture:** Backend gains a provider `OpenPullRequests` graph call, a read-only `GET …/pull-requests` endpoint for the client hint, and a batch `POST …/workspaces/import` backed by a new `worktree.CreateFromImport` resolver that walks each branch's PR-base chain and creates the tree parents-first. Frontend replaces the nav-stack `RepoSettingsPanel` with a `RepoIconPopover` (avatar trigger + pencil hover) and a virtualized `RepoImportDialog` whose "creates M parents" hint is pure client-side math over the PR graph.

**Tech Stack:** Go (gin, provider engine via `gh`/`glab`), React 19 + TypeScript, base-ui (`Popover`, `Dialog`), `@tanstack/react-virtual`, Vitest/bun test, Tauri MCP for live verification.

## Global Constraints

- Test files live in `web/src/__tests__/` mirroring `web/src/`; use `@/` imports. (CLAUDE.md)
- Component files kebab-case; exported component PascalCase. (CLAUDE.md)
- Stores: narrow selector `useXxxStore((s) => s.field)`; `.getState()` only in handlers/effects. (CLAUDE.md)
- Always use `@/components/ui/*` + CSS-variable tokens; never hardcode colors. (memory: component-tokens)
- Every backend bug fix gets a `TestRegression_*` in `api/tests` (integration tag). (memory: blackbox-regression-tests)
- No timing in tests — block on real signals (`WaitAsync`, WS state), never sleeps. (memory: no-timing-in-tests)
- Never spawn a new dev instance; reuse the running dev app (desktop PID `crowbar-desktop`, daemon on socket `crowbar-9e4cf99cff8053b.sock`). Verify live in Tauri before claiming any UI works. (memory: one-dev-instance, verify-in-tauri)
- Do not push or open PRs; commit locally only. (memory: no-unrequested-prs)
- bun on PATH via `export PATH="$HOME/.bun/bin:$PATH"`. (memory: web-test-env-gotchas)

---

## Task 1: Provider `OpenPullRequests` graph

**Files:**
- Modify: `api/internal/engine/provider/types/types.go`
- Modify: `api/internal/engine/provider/provider.go` (both interfaces)
- Modify: `api/internal/engine/provider/engine.go` (passthrough)
- Modify: `api/internal/engine/provider/providers/github/github.go`
- Modify: `api/internal/engine/provider/providers/gitlab/gitlab.go`
- Test: `api/internal/engine/provider/providers/github/github_test.go`

**Interfaces:**
- Produces: `type PRLink struct { Head, Base string; Number int; Status, URL, Title string }`; `GitProvider.OpenPullRequests(ctx, repoPath) ([]PRLink, error)`; `Engine.OpenPullRequests(ctx, repoPath) ([]PRLink, error)`.

- [ ] **Step 1: Write the failing test** (github open-PR list parse)

In `github_test.go`, add a test that stubs the exec to return a `gh pr list` JSON array with two open PRs (`feat/9324`→`feat/base`, `feat/base`→`dev`) and asserts `OpenPullRequests` returns two `PRLink`s with those Head/Base pairs. Mirror the existing `PullRequestForBranch` test's exec-stub style (search the file for `NewWithExec`).

```go
func TestOpenPullRequests_ParsesHeadBaseGraph(t *testing.T) {
	const out = `[
	  {"number":9324,"state":"OPEN","url":"u1","title":"t1","headRefName":"feat/9324","baseRefName":"feat/base"},
	  {"number":10,"state":"OPEN","url":"u2","title":"t2","headRefName":"feat/base","baseRefName":"dev"}
	]`
	prov := NewWithExec(stubExec(t, out)) // reuse this file's existing exec stub helper
	links, err := prov.OpenPullRequests(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got := map[string]string{}
	for _, l := range links {
		got[l.Head] = l.Base
	}
	if got["feat/9324"] != "feat/base" || got["feat/base"] != "dev" {
		t.Fatalf("graph wrong: %#v", got)
	}
}
```

If the file's exec stub helper has a different name/signature, adapt this call to it (do not invent a new stub).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/engine/provider/providers/github/ -run TestOpenPullRequests_ParsesHeadBaseGraph`
Expected: FAIL — `prov.OpenPullRequests undefined`.

- [ ] **Step 3: Add the type**

In `types/types.go`, after `PRInfo`:

```go
// PRLink is one edge of the repo's open-PR graph: an open PR's head branch and
// the base it targets. Used to parent imported branches under their PR base.
type PRLink struct {
	Head   string `json:"head"`
	Base   string `json:"base"`
	Number int    `json:"number"`
	Status string `json:"status"`
	URL    string `json:"url"`
	Title  string `json:"title"`
}
```

In `provider.go`, add the alias next to `PRInfo`: `type PRLink = providertypes.PRLink`, and add to both `GitProvider` and `Engine` interfaces:

```go
// OpenPullRequests returns the head→base graph of all OPEN PRs for the repo in
// a single provider call. Empty when the CLI is unavailable or none are open.
OpenPullRequests(ctx context.Context, repoPath string) ([]PRLink, error)
```

- [ ] **Step 4: Implement GitHub**

In `github.go`, add (reuse `runGH`, `parsePRList`, and the `prJSON` shape — `prJSON` already has `headRefName`? it has `HeadRefOid`; add a `HeadRefName string json:"headRefName"` field to `prJSON`):

```go
// OpenPullRequests lists all open PRs as head→base links in one gh call.
// Open PRs own their head ref by name, so no per-branch ownership check is
// needed (see ownsPR).
func (g *ghProvider) OpenPullRequests(ctx context.Context, repoPath string) ([]providertypes.PRLink, error) {
	out, err := g.runGH(ctx, repoPath,
		"pr", "list", "--state", "open", "--limit", "500",
		"--json", "number,state,url,title,headRefName,baseRefName",
	)
	if err != nil {
		return nil, fmt.Errorf("github: open-prs: %w", err)
	}
	prs, err := parsePRList([]byte(out))
	if err != nil {
		return nil, fmt.Errorf("github: open-prs: parse: %w", err)
	}
	links := make([]providertypes.PRLink, 0, len(prs))
	for _, p := range prs {
		links = append(links, providertypes.PRLink{
			Head: p.HeadRefName, Base: p.BaseRefName, Number: p.Number,
			Status: mapState(p.State), URL: p.URL, Title: p.Title,
		})
	}
	return links, nil
}
```

- [ ] **Step 5: Implement GitLab**

In `gitlab.go`, mirror the above with `runGlab(ctx, repoPath, "mr", "list", "--state", "opened", "--output", "json")` and map each MR's `SourceBranch`→Head, `TargetBranch`→Base (reuse `parseMRList`, the `mrJSON`/whatever struct, and `mapState`; add source/target fields if absent). Match the existing MR struct's JSON tags.

- [ ] **Step 6: Implement engine passthrough**

In `engine.go`, add (mirroring `OwnerAvatarURL` — soft-fallback to empty on any failure):

```go
// OpenPullRequests returns the repo's open-PR head→base graph, or nil when the
// provider is unavailable. Best-effort: never fails the caller.
func (e *providerEngine) OpenPullRequests(ctx context.Context, repoPath string) ([]providertypes.PRLink, error) {
	res, err := e.detectFn(ctx, repoPath)
	if err != nil || !res.Enabled {
		return nil, nil
	}
	prov := e.providerFor(res.Kind)
	if prov == nil {
		return nil, nil
	}
	links, err := prov.OpenPullRequests(ctx, repoPath)
	if err != nil {
		return nil, nil
	}
	return links, nil
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd api && go test ./internal/engine/provider/...`
Expected: PASS. If GitLab has a mock provider implementing `GitProvider` in a `mocks` package, add `OpenPullRequests` there too (compile break will point at it).

- [ ] **Step 8: Commit**

```bash
git add api/internal/engine/provider
git commit -m "feat(provider): add OpenPullRequests head→base graph (gh/glab)"
```

---

## Task 2: `GET …/pull-requests` endpoint (client hint data)

**Files:**
- Modify: `api/internal/api/v0/endpoints/repos/handlers/repos.go` (add to `BranchProviderEngine`, add `PullRequests` handler + `PRLinkDTO`)
- Modify: `api/internal/api/v0/endpoints/repos/routes.go` (mount route)
- Test: `api/internal/api/v0/endpoints/repos/handlers/*_test.go` (mirror an existing `Branches` handler test)

**Interfaces:**
- Consumes: `Engine.OpenPullRequests` (Task 1).
- Produces: `GET /v0/projects/:projectId/repos/:repoId/pull-requests` → `[{ head, base, number, status, url, title }]`.

- [ ] **Step 1: Write the failing test**

Add a handler test that constructs `Handlers` with a stub `BranchProviderEngine` whose `OpenPullRequests` returns two links, hits `PullRequests`, and asserts a 200 JSON array of length 2 with the right head/base. Mirror the existing `Branches` handler test setup in this package (find it: `grep -rn "func Test.*Branches" api/internal/api/v0/endpoints/repos`).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/api/v0/endpoints/repos/... -run PullRequests`
Expected: FAIL — `h.PullRequests undefined`.

- [ ] **Step 3: Widen the provider interface**

In `repos.go`, add to `BranchProviderEngine`:

```go
OpenPullRequests(ctx context.Context, repoPath string) ([]providertypes.PRLink, error)
```

(import `providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"`).

- [ ] **Step 4: Implement the handler**

In `repos.go`:

```go
// PRLinkDTO is one edge of the open-PR graph for the import hint.
type PRLinkDTO struct {
	Head   string `json:"head"`
	Base   string `json:"base"`
	Number int    `json:"number"`
	Status string `json:"status"`
	URL    string `json:"url"`
	Title  string `json:"title"`
}

// PullRequests handles GET …/repos/:repoId/pull-requests. Returns the open-PR
// head→base graph for the import dialog's parent hint. Advisory only — the
// import endpoint re-resolves authoritatively. Soft-fails to [] when the
// provider CLI is unavailable.
func (h *Handlers) PullRequests(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	links := []PRLinkDTO{}
	if h.provider != nil {
		got, _ := h.provider.OpenPullRequests(c.Request.Context(), repo.Path)
		for _, l := range got {
			links = append(links, PRLinkDTO{
				Head: l.Head, Base: l.Base, Number: l.Number,
				Status: l.Status, URL: l.URL, Title: l.Title,
			})
		}
	}
	libs.WriteQueryOK(c, links)
}
```

- [ ] **Step 5: Mount the route**

In `routes.go`, after the branches route:

```go
rg.GET("/repos/:repoId/pull-requests", h.PullRequests)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd api && go test ./internal/api/v0/endpoints/repos/...`
Expected: PASS. Fix any stub `BranchProviderEngine` in other tests that now needs the new method.

- [ ] **Step 7: Commit**

```bash
git add api/internal/api/v0/endpoints/repos
git commit -m "feat(api): GET repos/:id/pull-requests open-PR graph for import hint"
```

---

## Task 3: `worktree.CreateFromImport` resolver

**Files:**
- Modify: `api/internal/app/usecases/worktree/worktree.go` (add `ImportInput`, `CreateFromImport` to `Usecase` interface)
- Create: `api/internal/app/usecases/worktree/import.go` (resolver + `chainFor` helper)
- Test: `api/internal/app/usecases/worktree/import_test.go`

**Interfaces:**
- Consumes: `u.provider.OpenPullRequests` (Task 1), `u.workspaces.List`, `u.CreateChild`.
- Produces: `ImportInput{ RepoID, ProjectID, RepoPath, RemoteURL, DefaultBranch string; Branches []string }`; `Usecase.CreateFromImport(ctx, ImportInput) error`.

- [ ] **Step 1: Write the failing test** (chain build parents-first)

`import_test.go` — unit-test the pure helper `chainFor` (no mocks needed):

```go
func TestChainFor_BuildsAncestorsFirstStoppingAtTerminals(t *testing.T) {
	base := map[string]string{"feat/9324": "feat/base", "feat/base": "dev"}
	existing := map[string]string{} // nothing imported yet
	got := chainFor("feat/9324", "dev", base, existing)
	want := []string{"feat/base", "feat/9324"} // dev is default → terminal, not created
	if !slicesEqual(got, want) {
		t.Fatalf("chain = %v, want %v", got, want)
	}

	// Existing ancestor is a terminal: feat/base already a workspace.
	existing2 := map[string]string{"feat/base": "ws-base"}
	got2 := chainFor("feat/9324", "dev", base, existing2)
	if !slicesEqual(got2, []string{"feat/9324"}) {
		t.Fatalf("chain2 = %v, want [feat/9324]", got2)
	}

	// Cycle guard: base points back up.
	baseCyc := map[string]string{"a": "b", "b": "a"}
	got3 := chainFor("a", "main", baseCyc, map[string]string{})
	if len(got3) == 0 || got3[len(got3)-1] != "a" {
		t.Fatalf("cycle chain must still include the leaf: %v", got3)
	}
}
```

Add a tiny `slicesEqual` helper in the test file.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test ./internal/app/usecases/worktree/ -run TestChainFor`
Expected: FAIL — `chainFor` undefined.

- [ ] **Step 3: Implement the resolver**

Create `import.go`:

```go
package worktree

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ImportInput carries a batch branch-import request: branches to import as
// managed workspaces, PR-parented up to a protected/default root.
type ImportInput struct {
	RepoID        string
	ProjectID     string
	RepoPath      string
	RemoteURL     string
	DefaultBranch string
	Branches      []string
}

// chainFor returns branch's ancestors-first chain of branches that must be
// CREATED: [rootmost-missing-ancestor, …, branch]. The walk climbs the PR-base
// graph and stops (excluding the terminal) at an existing workspace or the
// default branch; a branch with no PR base terminates at the default branch. A
// per-walk visited set breaks PR-base cycles.
func chainFor(branch, defaultBranch string, base, existing map[string]string) []string {
	var leafFirst []string
	visited := map[string]bool{}
	cur := branch
	for cur != "" && cur != defaultBranch {
		if _, ok := existing[cur]; ok {
			break // existing workspace is the parent terminal, not created here
		}
		if visited[cur] {
			break // cycle
		}
		visited[cur] = true
		leafFirst = append(leafFirst, cur)
		cur = base[cur] // "" when no open PR → loop ends, terminal is the default branch
	}
	// reverse to ancestors-first
	for i, j := 0, len(leafFirst)-1; i < j; i, j = i+1, j-1 {
		leafFirst[i], leafFirst[j] = leafFirst[j], leafFirst[i]
	}
	return leafFirst
}

// CreateFromImport imports each requested branch as a managed workspace,
// parenting it under the workspace for its open PR's base branch. Missing
// ancestors are created first and the whole chain is parented up to an existing
// workspace, a protected branch (already a locked workspace), or the repo
// default branch (parented under the repo home via an empty ParentID). It is
// best-effort per branch: one branch's failure is logged and does not abort the
// batch.
func (u *worktreeUsecase) CreateFromImport(ctx context.Context, in ImportInput) error {
	// 1. Open-PR graph head→base (best-effort).
	base := map[string]string{}
	links, err := u.provider.OpenPullRequests(ctx, in.RepoPath)
	if err != nil {
		slog.WarnContext(ctx, "import: open-PR graph unavailable; importing without PR parenting", "err", err)
	}
	for _, l := range links {
		if l.Head != "" && l.Base != "" {
			base[l.Head] = l.Base
		}
	}

	// 2. Existing non-default workspaces: branch → id (matches hasWorkspace).
	existing := map[string]string{}
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return fmt.Errorf("import: list workspaces: %w", err)
	}
	for _, w := range all {
		if w.RepoID == in.RepoID && !w.IsDefault && w.Status != domain.WorkspaceStatusDeleted {
			existing[w.Branch] = w.ID
		}
	}

	// 3. Global creation order, parents-before-children, deduped.
	order := []string{}
	queued := map[string]bool{}
	for _, b := range in.Branches {
		for _, node := range chainFor(b, in.DefaultBranch, base, existing) {
			if !queued[node] {
				queued[node] = true
				order = append(order, node)
			}
		}
	}

	// 4. Create each node, resolving its parent from existing/just-created.
	created := map[string]string{}
	for _, branch := range order {
		parentBranch := base[branch]
		parentID := ""
		switch {
		case parentBranch == "" || parentBranch == in.DefaultBranch:
			parentBranch = in.DefaultBranch
		default:
			if id, ok := existing[parentBranch]; ok {
				parentID = id
			} else if id, ok := created[parentBranch]; ok {
				parentID = id
			} else {
				parentBranch = in.DefaultBranch // parent unresolved (cycle) → default
			}
		}
		ws, createErr := u.CreateChild(ctx, CreateChildInput{
			RepoID:       in.RepoID,
			ProjectID:    in.ProjectID,
			RepoPath:     in.RepoPath,
			RemoteURL:    in.RemoteURL,
			Branch:       branch,
			ParentID:     parentID,
			ParentBranch: parentBranch,
		})
		if createErr != nil {
			slog.WarnContext(ctx, "import: create workspace failed", "branch", branch, "err", createErr)
			continue
		}
		created[branch] = ws.ID
	}
	return nil
}
```

Add `CreateFromImport(ctx context.Context, in ImportInput) (error)` to the `Usecase` interface in `worktree.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd api && go test ./internal/app/usecases/worktree/ -run TestChainFor`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/worktree
git commit -m "feat(worktree): CreateFromImport PR-base parent-chain resolver"
```

---

## Task 4: `POST …/workspaces/import` endpoint + regression test

**Files:**
- Modify: `api/internal/api/v0/endpoints/workspaces/handlers/handlers.go` (add `CreateFromImport` to `Hierarchy`)
- Create: `api/internal/api/v0/endpoints/workspaces/handlers/import.go` (`Import` handler)
- Modify: `api/internal/api/v0/endpoints/workspaces/routes.go` (mount route)
- Test: `api/tests/regressions_test.go` (add `TestRegression_ImportParentsPRChain`)

**Interfaces:**
- Consumes: `worktree.CreateFromImport` (Task 3), `h.repos.FindByKey`, `h.runAsync`.
- Produces: `POST /v0/projects/:projectId/repos/:repoId/workspaces/import` body `{ "branches": [...] }` → 202.

- [ ] **Step 1: Write the failing regression test**

In `api/tests/regressions_test.go` (integration build tag), add a black-box test: import an env, ensure a remote branch `feat/child` exists whose open PR targets `feat/base` (which is NOT yet a workspace) and `feat/base`'s PR targets the default branch. POST `…/workspaces/import {"branches":["feat/child"]}`, `WaitAsync`, then assert via the workspaces list that BOTH `feat/base` and `feat/child` workspaces exist, `feat/child.parentId == feat/base.id`, and `feat/base.parentId == ""`. Reuse the suite's PR-stub mechanism (search: `PushProviderState` / how `PullRequestForBranch` is stubbed in `api/tests`); if the test env drives `gh` via a fake, stub `OpenPullRequests` there analogously. Model the POST + WaitAsync + list-assert shape on `createChildWorkspace` at `regressions_test.go:1148`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && go test -tags integration ./tests/ -run TestRegression_ImportParentsPRChain`
Expected: FAIL — route 404 / handler missing.

- [ ] **Step 3: Widen the Hierarchy interface**

In `handlers.go`, add to `Hierarchy`:

```go
CreateFromImport(
	ctx context.Context,
	in worktree.ImportInput,
) error
```

- [ ] **Step 4: Implement the handler**

Create `import.go`:

```go
package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
)

type importRequest struct {
	Branches []string `json:"branches"`
}

// Import handles POST …/workspaces/import: batch-imports branches as managed
// workspaces, PR-parented up to a protected/default root. Validates
// synchronously (repo exists, branches non-empty), returns 202, and resolves +
// creates the tree in the background. Each created workspace arrives on the
// per-repo workspace WS stream.
func (h *Handlers) Import(c *gin.Context) {
	var body importRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.Branches) == 0 {
		libs.WriteErr(c, http.StatusBadRequest, "branches is required")
		return
	}
	repo, err := h.repos.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, apperr.ErrNotFound.Error())
		return
	}
	in := worktree.ImportInput{
		RepoID:        repo.ID,
		ProjectID:     repo.ProjectID,
		RepoPath:      repo.Path,
		RemoteURL:     repo.RemoteURL,
		DefaultBranch: repo.DefaultBranch,
		Branches:      body.Branches,
	}
	libs.WriteAccepted(c)
	h.runAsync(
		c.Request.Context(),
		h.working,
		h.broadcastLastError,
		"",
		func(ctx context.Context) error {
			if importErr := h.hierarchy.CreateFromImport(ctx, in); importErr != nil {
				slog.WarnContext(ctx, "workspace import failed", "repo", in.RepoID, "err", importErr)
				return importErr
			}
			return nil
		},
	)
}
```

- [ ] **Step 5: Mount the route**

In `routes.go`, after `rg.POST("/workspaces", h.Create)`:

```go
rg.POST("/workspaces/import", h.Import)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd api && go test -tags integration ./tests/ -run TestRegression_ImportParentsPRChain && go test ./internal/api/v0/endpoints/workspaces/...`
Expected: PASS. Add `CreateFromImport` to any mock `Hierarchy` the compile flags.

- [ ] **Step 7: Commit**

```bash
git add api/internal/api/v0/endpoints/workspaces api/tests
git commit -m "feat(api): POST workspaces/import with PR-based auto-parenting + regression"
```

---

## Task 5: Web parent-plan hint math (pure)

**Files:**
- Create: `web/src/lib/import/parent-plan.ts`
- Test: `web/src/__tests__/lib/import/parent-plan.test.ts`

**Interfaces:**
- Produces: `PRLink{head,base}`, `BranchEntry{name,isProtected,hasWorkspace}`, `ImportPlan{importCount,parentCount}`, `computeImportPlan(selected, prLinks, branches, defaultBranch): ImportPlan`.

- [ ] **Step 1: Write the failing test**

`parent-plan.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { computeImportPlan } from '@/lib/import/parent-plan'

const branches = [
  { name: 'dev', isProtected: true, hasWorkspace: false },
  { name: 'feat/base', isProtected: false, hasWorkspace: false },
  { name: 'feat/9324', isProtected: false, hasWorkspace: false },
]
const prLinks = [
  { head: 'feat/9324', base: 'feat/base' },
  { head: 'feat/base', base: 'dev' },
]

describe('computeImportPlan', () => {
  it('counts the missing PR ancestor as a created parent', () => {
    const plan = computeImportPlan(['feat/9324'], prLinks, branches, 'dev')
    expect(plan).toEqual({ importCount: 1, parentCount: 1 }) // feat/base
  })

  it('does not double-count an ancestor that is itself selected', () => {
    const plan = computeImportPlan(['feat/9324', 'feat/base'], prLinks, branches, 'dev')
    expect(plan).toEqual({ importCount: 2, parentCount: 0 })
  })

  it('terminates at an already-imported ancestor', () => {
    const imported = branches.map((b) =>
      b.name === 'feat/base' ? { ...b, hasWorkspace: true } : b,
    )
    const plan = computeImportPlan(['feat/9324'], prLinks, imported, 'dev')
    expect(plan).toEqual({ importCount: 1, parentCount: 0 })
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `export PATH="$HOME/.bun/bin:$PATH" && cd web && bun run test parent-plan`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

`parent-plan.ts`:

```ts
export interface PRLink {
  head: string
  base: string
}

export interface BranchEntry {
  name: string
  isProtected: boolean
  hasWorkspace: boolean
}

export interface ImportPlan {
  importCount: number
  parentCount: number
}

/**
 * Mirrors the server import resolver's parenting for the dialog's hint. For
 * each selected branch it walks the open-PR base chain, counting ancestors that
 * would be CREATED — excluding already-imported branches (hasWorkspace),
 * protected branches, the default branch, and the selected branches themselves
 * (those are the importCount). Advisory only; the server re-resolves on import.
 */
export function computeImportPlan(
  selected: string[],
  prLinks: PRLink[],
  branches: BranchEntry[],
  defaultBranch: string,
): ImportPlan {
  const base = new Map<string, string>()
  for (const l of prLinks) if (l.head && l.base) base.set(l.head, l.base)

  const imported = new Set(branches.filter((b) => b.hasWorkspace).map((b) => b.name))
  const protectedSet = new Set(branches.filter((b) => b.isProtected).map((b) => b.name))
  const selectedSet = new Set(selected)

  const parents = new Set<string>()
  for (const branch of selected) {
    const visited = new Set<string>([branch])
    let cur = base.get(branch)
    while (cur && cur !== defaultBranch && !visited.has(cur)) {
      visited.add(cur)
      if (imported.has(cur) || protectedSet.has(cur)) break // terminal, not created
      if (!selectedSet.has(cur)) parents.add(cur)
      cur = base.get(cur)
    }
  }
  return { importCount: selected.length, parentCount: parents.size }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `export PATH="$HOME/.bun/bin:$PATH" && cd web && bun run test parent-plan`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/import/parent-plan.ts web/src/__tests__/lib/import/parent-plan.test.ts
git commit -m "feat(web): computeImportPlan pure hint math for branch import"
```

---

## Task 6: Web API helpers

**Files:**
- Modify: `web/src/lib/api.ts`

**Interfaces:**
- Produces: `getRepoPullRequests(projectId, repoId): Promise<PRLink[]>`; `importBranches(projectId, repoId, branches: string[]): Promise<void>`.

- [ ] **Step 1: Implement helpers**

In `api.ts`, near `postWorkspace`:

```ts
import type { PRLink } from '@/lib/import/parent-plan'

// Open-PR head→base graph for the import dialog's parent hint (advisory).
export function getRepoPullRequests(projectId: string, repoId: string): Promise<PRLink[]> {
  return apiFetch<PRLink[]>(`/v0/projects/${projectId}/repos/${repoId}/pull-requests`)
}

// Batch-import branches as managed workspaces; the daemon PR-parents them and
// creates missing ancestors (whole tree). Returns on 202-accept; created
// workspaces arrive on the workspaces WS stream.
export function importBranches(
  projectId: string,
  repoId: string,
  branches: string[],
): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}/repos/${repoId}/workspaces/import`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ branches }),
  })
}
```

- [ ] **Step 2: Typecheck**

Run: `export PATH="$HOME/.bun/bin:$PATH" && cd web && bun run tsc --noEmit`
Expected: no new errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/api.ts
git commit -m "feat(web): getRepoPullRequests + importBranches API helpers"
```

---

## Task 7: `repo-icon-popover.tsx`

**Files:**
- Create: `web/src/components/layout/repo-icon-popover.tsx`
- Test: `web/src/__tests__/components/layout/repo-icon-popover.test.tsx`

**Interfaces:**
- Consumes: existing icon handlers/endpoints (from `repo-settings-panel.tsx`), `Popover`/`PopoverTrigger`/`PopoverContent` from `@/components/ui/popover`, `useSidebarStore`.
- Produces: `RepoIconPopover({ repo }: { repo: Repo })` — renders the avatar as the popover trigger with a pencil hover overlay, popup body holds Upload/Emoji/GitHub/Reset.

- [ ] **Step 1: Write the failing test**

Test: renders `RepoIconPopover`, asserts the avatar trigger is present and the popup content (Upload/Emoji/GitHub buttons) is NOT in the DOM until the avatar is clicked; after `fireEvent.click(avatar)`, the three buttons appear. Also assert a click on the avatar calls `stopPropagation` (spy on the event). Mirror the existing `repo-settings-panel.test.tsx` mocks for `apiFetch`, `isTauri`, `openNativeDialog` (copy the mock block).

- [ ] **Step 2: Run test to verify it fails**

Run: `export PATH="$HOME/.bun/bin:$PATH" && cd web && bun run test repo-icon-popover`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

Extract the Icon block from `repo-settings-panel.tsx` verbatim (state: `emojiInput`, `showEmojiInput`, `iconLoading`, `iconVersion`, `fileRef`; handlers: `handleUpload`, `handleFileChange`, `handleEmojiSubmit`, `handleGithubAvatar`, `handleResetIcon`; the `avatarSrc`/`isEmoji` derivation). Render:

```tsx
export function RepoIconPopover({ repo }: { repo: Repo }) {
  // …extracted icon state + handlers, repoBase from repo.projectId/repo.id…
  const avatar = /* the avatar span/img/fallback — small 5x5 variant matching the row */
  return (
    <Popover>
      <PopoverTrigger
        className="group/repo-icon relative inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-md"
        aria-label={`Edit ${repo.name} icon`}
        onClick={(e) => e.stopPropagation()}
      >
        {avatar}
        {/* pencil overlay — only on hovering the avatar */}
        <span className="pointer-events-none absolute inset-0 hidden items-center justify-center rounded-md bg-black/40 group-hover/repo-icon:flex">
          <Pencil className="size-2.5 text-white" />
        </span>
      </PopoverTrigger>
      <PopoverContent side="right" align="start" className="w-64">
        {/* the extracted preview avatar (size-14) + Upload/Emoji/GitHub + emoji input + Reset */}
      </PopoverContent>
    </Popover>
  )
}
```

Notes: `Pencil` from `lucide-react`. Use CSS tokens for colors where a token exists; the `bg-black/40` scrim is acceptable (it's a photo overlay, not a themed surface). Keep the popup controls identical to the panel's icon section. The avatar variants (spinner when `repo.defaultWorking`, emoji, img, colored fallback) come from `repo-section.tsx:150-183` — reuse that exact JSX; when `defaultWorking`, render ONLY the spinner span and NOT the Popover (an agent turn is running, icon is not editable).

- [ ] **Step 4: Run test to verify it passes**

Run: `export PATH="$HOME/.bun/bin:$PATH" && cd web && bun run test repo-icon-popover`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/repo-icon-popover.tsx web/src/__tests__/components/layout/repo-icon-popover.test.tsx
git commit -m "feat(web): RepoIconPopover — avatar-anchored icon editor with pencil hover"
```

---

## Task 8: `repo-import-dialog.tsx` (virtualized)

**Files:**
- Create: `web/src/components/layout/repo-import-dialog.tsx`
- Test: `web/src/__tests__/components/layout/repo-import-dialog.test.tsx`

**Interfaces:**
- Consumes: `Dialog`/`DialogPopup`/`DialogHeader`/`DialogTitle`/`DialogDescription`, `Input`, `Button`, `Checkbox`, `useVirtualizer`, `computeImportPlan`, `getRepoPullRequests`, `importBranches`, `apiFetch`.
- Produces: `RepoImportDialog({ projectId, repoId, defaultBranch, open, onOpenChange })`.

- [ ] **Step 1: Write the failing test**

Test: mock `apiFetch` to return a branch list (1 protected `dev`, 1 `feat/base` with `hasWorkspace:false`, 1 `feat/9324`) and `getRepoPullRequests` to return the two links; render with `open={true}`; assert (a) the branch rows render, (b) protected/hasWorkspace rows are not selectable, (c) selecting `feat/9324` shows the hint text containing "creates 1 parent", (d) clicking Import calls `importBranches` with `['feat/9324']` and then `onOpenChange(false)`. Mock `importBranches`/`getRepoPullRequests` via `vi.mock('@/lib/api', …)`.

- [ ] **Step 2: Run test to verify it fails**

Run: `export PATH="$HOME/.bun/bin:$PATH" && cd web && bun run test repo-import-dialog`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

```tsx
const IMPORT_ROW_HEIGHT = 30

export function RepoImportDialog({
  projectId, repoId, defaultBranch, open, onOpenChange,
}: {
  projectId: string; repoId: string; defaultBranch: string
  open: boolean; onOpenChange: (open: boolean) => void
}) {
  const repoBase = `/v0/projects/${projectId}/repos/${repoId}`
  const [branches, setBranches] = useState<BranchEntry[]>([])
  const [prLinks, setPrLinks] = useState<PRLink[]>([])
  const [filter, setFilter] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [importing, setImporting] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    setSelected(new Set()); setFilter('')
    apiFetch<BranchEntry[]>(`${repoBase}/branches`).then(setBranches).catch(() => setBranches([]))
    getRepoPullRequests(projectId, repoId).then(setPrLinks).catch(() => setPrLinks([]))
  }, [open, repoBase, projectId, repoId])

  const visible = useMemo(() => {
    const q = filter.toLowerCase()
    const list = branches.filter((b) => b.name.toLowerCase().includes(q))
    return [...list.filter((b) => b.isProtected), ...list.filter((b) => !b.isProtected)]
  }, [branches, filter])

  const rowVirtualizer = useVirtualizer({
    count: visible.length,
    estimateSize: () => IMPORT_ROW_HEIGHT,
    getScrollElement: () => scrollRef.current,
    overscan: 10,
  })

  const plan = useMemo(
    () => computeImportPlan([...selected], prLinks, branches, defaultBranch),
    [selected, prLinks, branches, defaultBranch],
  )

  function toggle(name: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      next.has(name) ? next.delete(name) : next.add(name)
      return next
    })
  }

  async function handleImport() {
    if (selected.size === 0) return
    setImporting(true)
    try {
      await importBranches(projectId, repoId, [...selected])
      onOpenChange(false)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to import branches')
    } finally {
      setImporting(false)
    }
  }

  const hint =
    plan.parentCount > 0
      ? `Imports ${plan.importCount} branch${plan.importCount > 1 ? 'es' : ''} · creates ${plan.parentCount} parent${plan.parentCount > 1 ? 's' : ''}`
      : `Imports ${plan.importCount} branch${plan.importCount !== 1 ? 'es' : ''}`

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPopup className="flex h-[70vh] max-w-md flex-col p-0" showCloseButton>
        <DialogHeader className="p-4 pb-2">
          <DialogTitle>Import branches</DialogTitle>
          <DialogDescription>Bring remote branches into Crowbar as workspaces.</DialogDescription>
        </DialogHeader>
        <div className="flex min-h-0 flex-1 flex-col gap-2 px-4 pb-4">
          <Input
            placeholder="Search branches…"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="h-7 shrink-0 text-xs"
          />
          <div ref={scrollRef} className="min-h-0 flex-1 overflow-auto">
            <div style={{ height: rowVirtualizer.getTotalSize(), position: 'relative', width: '100%' }}>
              {rowVirtualizer.getVirtualItems().map((vi) => {
                const b = visible[vi.index]
                return (
                  <div
                    key={b.name}
                    style={{
                      position: 'absolute', top: 0, left: 0, width: '100%',
                      height: vi.size, transform: `translateY(${vi.start}px)`,
                    }}
                  >
                    <ImportRow branch={b} checked={selected.has(b.name)} onToggle={toggle} />
                  </div>
                )
              })}
            </div>
          </div>
          <p className="shrink-0 text-xs text-muted-foreground">{hint}</p>
          <Button size="sm" className="shrink-0" disabled={selected.size === 0 || importing} onClick={handleImport}>
            {importing ? 'Importing…' : 'Import'}
          </Button>
        </div>
      </DialogPopup>
    </Dialog>
  )
}
```

`ImportRow` (same package, local): protected → `Lock` + name at `opacity-40`; `hasWorkspace` → `Check` (text-green-500) + name at `opacity-40`; else a `<label>` with `Checkbox` (from the panel's row JSX at `repo-settings-panel.tsx:333-361`). Each row is a fixed `h-[30px]` flex item. Import `BranchEntry`/`PRLink` types from `@/lib/import/parent-plan`.

- [ ] **Step 4: Run test to verify it passes**

Run: `export PATH="$HOME/.bun/bin:$PATH" && cd web && bun run test repo-import-dialog`
Expected: PASS. (jsdom has zero layout, so the virtualizer may mount 0 rows; if so, the test should assert on selection→hint→import via a small count that fits overscan, or set a fixed `getBoundingClientRect` height on the scroll element in the test — see `agent-chats-panel.test.tsx` for the jsdom virtualizer pattern.)

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/repo-import-dialog.tsx web/src/__tests__/components/layout/repo-import-dialog.test.tsx
git commit -m "feat(web): RepoImportDialog — virtualized branch import with parent hint"
```

---

## Task 9: Wire into `repo-section.tsx`; remove `repo-settings-panel.tsx`

**Files:**
- Modify: `web/src/components/layout/repo-section.tsx`
- Delete: `web/src/components/layout/repo-settings-panel.tsx`
- Delete: `web/src/__tests__/components/layout/repo-settings-panel.test.tsx`
- Test: `web/src/__tests__/components/layout/repo-section.test.tsx` (create if absent) or extend the existing workspace-tree test.

**Interfaces:**
- Consumes: `RepoIconPopover` (Task 7), `RepoImportDialog` (Task 8), `Tooltip` (default export from `@/components/ui/tooltip`), `DownloadCloud`/remove `Settings` import.

- [ ] **Step 1: Write/adjust the failing test**

Add a `repo-section.test.tsx` asserting: (a) the avatar renders a `RepoIconPopover` trigger (aria-label `Edit … icon`), (b) an "Import branches" button exists (aria-label), clicking it makes the import dialog appear, (c) there is NO longer a "Repo settings" gear button, (d) clicking the avatar does NOT call `onWorkspaceClick` (navigation), (e) clicking the row body still calls `onWorkspaceClick`. Mock `RepoImportDialog`/`RepoIconPopover` as light stubs if needed to keep this focused on wiring.

- [ ] **Step 2: Run test to verify it fails**

Run: `export PATH="$HOME/.bun/bin:$PATH" && cd web && bun run test repo-section`
Expected: FAIL — gear still present / import button missing.

- [ ] **Step 3: Replace the avatar block + gear button**

In `repo-section.tsx`:
- Remove the `import { Settings } from 'lucide-react'` and the `RepoSettingsPanel` import; add `import { DownloadCloud } from 'lucide-react'`, `import Tooltip from '@/components/ui/tooltip'`, `import { RepoIconPopover } from './repo-icon-popover'`, `import { RepoImportDialog } from './repo-import-dialog'`.
- Add local state: `const [importOpen, setImportOpen] = useState(false)`.
- Replace the entire avatar conditional (`repo.defaultWorking ? … : …`, lines ~150-183) with `<RepoIconPopover repo={repo} />` (the popover component owns the spinner/emoji/img/fallback + working state internally).
- Replace the gear `<button aria-label="Repo settings">…</button>` (lines ~243-263) with:

```tsx
<Tooltip content="Import branches">
  <button
    type="button"
    aria-label="Import branches"
    className={ROW_SUB_ACTION}
    onClick={(e) => {
      e.stopPropagation()
      setImportOpen(true)
    }}
  >
    <DownloadCloud className="size-3" />
  </button>
</Tooltip>
```

- Render the dialog once inside the component (e.g. just before the closing `</div>` of the `mb-1` wrapper):

```tsx
<RepoImportDialog
  projectId={repo.projectId ?? ''}
  repoId={repo.id}
  defaultBranch={repo.defaultBranch ?? ''}
  open={importOpen}
  onOpenChange={setImportOpen}
/>
```

(If `Repo` lacks `defaultBranch`, check `@/lib/store/sidebar`; use the field that holds it, or pass `''` and let the server use `repo.DefaultBranch` — the client hint just treats unknown default as non-terminal, which is safe.)

- Remove the now-unused `useSidebarNavStore` import if nothing else in the file uses it (grep first).

- [ ] **Step 4: Delete the panel + its test**

```bash
git rm web/src/components/layout/repo-settings-panel.tsx web/src/__tests__/components/layout/repo-settings-panel.test.tsx
```

- [ ] **Step 5: Run tests + typecheck**

Run: `export PATH="$HOME/.bun/bin:$PATH" && cd web && bun run test repo-section && bun run tsc --noEmit`
Expected: PASS, no type errors, no dangling references to `RepoSettingsPanel`/`repo-settings:`.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/layout web/src/__tests__/components/layout
git commit -m "feat(web): deconstruct repo settings into icon popover + import dialog"
```

---

## Task 10: Full gate + live Tauri verification

**Files:** none (verification only)

- [ ] **Step 1: Backend gate**

Run: `cd api && go build ./... && go test ./... && go test -tags integration ./tests/ -run Import && gofmt -l . | head`
Expected: build clean, tests PASS, no gofmt drift.

- [ ] **Step 2: Web gate**

Run: `export PATH="$HOME/.bun/bin:$PATH" && cd web && bun run tsc --noEmit && bun run test && bun run lint`
Expected: all green.

- [ ] **Step 3: Rebuild dev sidecar + hot-restart daemon (reuse the running dev app)**

Follow memory `project_daemon_dev_restart`: rebuild the sidecar (noEmbed), kill+restart the dev `crowbar-api` on the fixed socket `crowbar-9e4cf99cff8053b.sock`; the running desktop FE auto-reconnects. Do NOT launch a second desktop/daemon (memory: one-dev-instance). Vite HMR picks up the web changes.

- [ ] **Step 4: Live-verify the icon popover (Tauri MCP)**

Drive the already-running `crowbar-desktop` via the Tauri MCP (`mcp__tauri__*`). Screenshot the sidebar; hover a repo avatar → confirm the pencil overlay appears ONLY over the icon; click the avatar → confirm the popover opens (NOT navigation to repo-home); set an emoji → confirm the row avatar updates; confirm clicking the repo name still renames and the row body still navigates.

- [ ] **Step 5: Live-verify the import modal + auto-parenting (Tauri MCP)**

Click the "Import branches" icon button → confirm the modal opens; scroll the branch list (confirm virtualization: only visible rows in the DOM via `webview_dom_snapshot`); type in the search box (confirm client-side filter, no refetch via `read_network_requests`); select a branch that has an open PR onto a not-yet-imported base → confirm the hint reads "creates N parents"; click Import → confirm the modal closes and the sidebar tree shows the imported branch nested under a newly-created parent workspace. This is the production bug's fix — verify the tree parents correctly.

- [ ] **Step 6: Report evidence**

Summarize with screenshots/DOM evidence for both interactions. Only claim "works" against live Tauri observation (memory: verify-in-tauri). Do NOT push or open a PR (memory: no-unrequested-prs).

---

## Self-Review

**Spec coverage:**
- Icon popover (avatar trigger + pencil hover, icon-only) → Task 7 + Task 9. ✓
- Gear → Import icon-button + tooltip → Task 9. ✓
- Import modal (CossUI Dialog) → Task 8. ✓
- Delete `RepoSettingsPanel` + nav push → Task 9. ✓
- Virtualized list → Task 8 (`useVirtualizer`). ✓
- Lazy/on-open fetch, client-side filter, list-render not blocked on network → Task 8 (fetch in `open` effect; `/branches` and `/pull-requests` fired independently). ✓
- Client-side hint math → Task 5 + Task 8. ✓
- Provider `OpenPullRequests` → Task 1. ✓
- PR-graph endpoint → Task 2. ✓
- Batch import endpoint + resolver (chain walk, dedup, topo, cycle guard, terminals) → Task 3 + Task 4. ✓
- Smart already-imported detection (terminate at existing ws; exclude from hint) → Task 3 (`existing` map) + Task 5 (`imported` set). ✓
- Regression test → Task 4. ✓
- Live Tauri verification → Task 10. ✓

**Placeholder scan:** No "TBD"/"handle edge cases" left; each code step carries real code. Two steps reference existing exemplars by exact path for boilerplate to copy (exec-stub helper in `github_test.go`, jsdom virtualizer pattern in `agent-chats-panel.test.tsx`) rather than reprinting unrelated code — intentional, not a gap.

**Type consistency:** `PRLink{head,base,number,status,url,title}` consistent across Task 1 (Go), Task 2 (DTO), Task 5/6 (TS). `computeImportPlan(selected, prLinks, branches, defaultBranch)` and `ImportPlan{importCount,parentCount}` consistent Task 5 ↔ Task 8. `CreateFromImport(ctx, ImportInput)` consistent Task 3 (usecase) ↔ Task 4 (Hierarchy iface + handler). `chainFor(branch, defaultBranch, base, existing)` consistent Task 3 test ↔ impl.
