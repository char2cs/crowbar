# Sidebar Nav + Repo Settings + Storage Unification — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Centralise all Crowbar state under `~/.crowbar/`, add a global iOS-style push navigation system to the sidebar, and replace the Sheet-based repo settings with an inline panel that includes an icon picker.

**Architecture:** Backend worktrees move from a sibling `.crowbar-worktrees/` dir to `~/.crowbar/projects/<host>/<owner>/<repo>/workspaces/<ws-id>/`. Three new icon REST endpoints write to `~/.crowbar/projects/<host>/<owner>/<repo>/icon.<ext>`. Frontend gets a `useSidebarNavStore` (Zustand) driving a `NavStack` component with CSS `translateX` push transitions; any component can push a screen into the sidebar.

**Tech Stack:** Go (gin, GORM, SQLite), React 18, Zustand, Vitest, Tailwind CSS.

---

## File Map

**Create:**
- `api/internal/app/usecases/internal/worktreepath/worktreepath.go` — rewrite (same file)
- `api/internal/app/usecases/internal/worktreepath/worktreepath_test.go` — rewrite
- `web/src/features/layout/stores/sidebar-nav.ts` — new Zustand nav stack store
- `web/src/components/layout/nav-stack.tsx` — new NavStack + NavScreen components
- `web/src/__tests__/features/layout/stores/sidebar-nav.test.ts` — new
- `web/src/__tests__/components/layout/nav-stack.test.tsx` — new

**Modify:**
- `api/internal/domain/repository.go` — add `RemoteURL` field
- `api/internal/app/usecases/project/project_import.go` — populate `RemoteURL` at import
- `api/internal/app/usecases/worktree/worktree.go` — inject crowbarHome, use new path
- `api/internal/app/usecases/worktree/worktree_test.go` — add crowbarHome param + RemoteURL
- `api/internal/app/usecases/worktree/worktree_integration_test.go` — crowbarHome + RemoteURL
- `api/internal/app/usecases/container.go` — pass worktreepath.DefaultCrowbarHome
- `api/internal/api/v0/endpoints/repos/handlers/repos.go` — add PutIcon/DeleteIcon/PutIconEmoji/PutIconGithub
- `api/internal/api/v0/endpoints/repos/handlers/repos_test.go` — icon handler tests
- `api/internal/api/v0/endpoints/repos/routes.go` — register new routes
- `api/internal/api/v0/dto/repo.go` — handle `emoji:` prefix in RepoDTOFrom
- `web/src/components/layout/sidebar-carousel.tsx` — wrap WorkspaceTree in NavStack
- `web/src/components/layout/workspace-tree.tsx` — gear click pushes nav instead of Sheet
- `web/src/components/layout/repo-settings-panel.tsx` — remove Sheet, add icon picker

---

## Task 1: Add RemoteURL to domain.Repository

**Files:**
- Modify: `api/internal/domain/repository.go`

- [ ] **Step 1: Add RemoteURL field**

```go
// api/internal/domain/repository.go
package domain

type Repository struct {
	ID            string `gorm:"primaryKey" json:"id"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultBranch string `json:"defaultBranch"`
	AvatarLabel   string `json:"avatarLabel"`
	AvatarColor   string `json:"avatarColor"`
	AvatarURL     string `json:"avatarUrl,omitempty"`
	RemoteURL     string `json:"remoteUrl,omitempty"`
}

func (Repository) TableName() string {
	return "repositories"
}
```

GORM AutoMigrate (already wired in `sqlite.go`) adds the new column automatically. No SQL migration needed.

- [ ] **Step 2: Run the build to confirm no compile errors**

```bash
cd api && go build ./...
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add api/internal/domain/repository.go
git commit -m "feat(domain): add RemoteURL field to Repository"
```

---

## Task 2: Populate RemoteURL at import time

**Files:**
- Modify: `api/internal/app/usecases/project/project_import.go`

- [ ] **Step 1: Add a helper to get the remote URL from a git repo path**

In `project_import.go`, find the section after the `repoPath` is resolved and before `repo := domain.Repository{...}`. Add a `gitRemoteURL` helper at the bottom of the file:

```go
// gitRemoteURL returns the origin remote URL for the repo at path, or ""
// on any failure so callers can fall back gracefully.
func gitRemoteURL(repoPath string) string {
	out, err := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
```

Add `"os/exec"` and `"strings"` to imports if not already present (check the existing import block first).

- [ ] **Step 2: Set RemoteURL on the repository record**

Find the `repo := domain.Repository{...}` block in `importRepo` (or the equivalent function that saves the repo). Add `RemoteURL: gitRemoteURL(repoPath)`:

```go
repo := domain.Repository{
	ID:            uuid.NewString(),
	ProjectID:     project.ID,
	Name:          name,
	Path:          repoPath,
	DefaultBranch: defaultbranch.Resolve(runner, defaultBranchCandidates),
	AvatarLabel:   avatar.Label(name),
	AvatarColor:   avatar.Color(name),
	AvatarURL:     avatarURL,
	RemoteURL:     gitRemoteURL(repoPath),
}
```

- [ ] **Step 3: Build to confirm no errors**

```bash
cd api && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add api/internal/app/usecases/project/project_import.go
git commit -m "feat(import): populate RemoteURL on repository at import time"
```

---

## Task 3: Rewrite worktreepath package

**Files:**
- Modify: `api/internal/app/usecases/internal/worktreepath/worktreepath.go`
- Modify: `api/internal/app/usecases/internal/worktreepath/worktreepath_test.go`

This replaces the old `For(repoPath, branch)` formula entirely. The package now derives paths from the git remote URL + workspace ID.

- [ ] **Step 1: Write the failing tests first**

Replace all of `worktreepath_test.go` with:

```go
package worktreepath

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFor_HTTPSRemote(t *testing.T) {
	path, err := For("/crow", "https://github.com/acme/my-repo.git", "ws-abc")
	require.NoError(t, err)
	assert.Equal(t, "/crow/projects/github.com/acme/my-repo/workspaces/ws-abc", path)
}

func TestFor_HTTPSNoGitSuffix(t *testing.T) {
	path, err := For("/crow", "https://github.com/acme/my-repo", "ws-xyz")
	require.NoError(t, err)
	assert.Equal(t, "/crow/projects/github.com/acme/my-repo/workspaces/ws-xyz", path)
}

func TestFor_SSHRemote(t *testing.T) {
	path, err := For("/crow", "git@github.com:acme/my-repo.git", "ws-001")
	require.NoError(t, err)
	assert.Equal(t, "/crow/projects/github.com/acme/my-repo/workspaces/ws-001", path)
}

func TestFor_EmptyRemoteURLErrors(t *testing.T) {
	_, err := For("/crow", "", "ws-001")
	assert.Error(t, err)
}

func TestFor_UnrecognisedURLErrors(t *testing.T) {
	_, err := For("/crow", "not-a-url", "ws-001")
	assert.Error(t, err)
}

func TestFor_DeterministicSameInputs(t *testing.T) {
	a, err := For("/crow", "https://github.com/acme/repo.git", "ws-1")
	require.NoError(t, err)
	b, err := For("/crow", "https://github.com/acme/repo.git", "ws-1")
	require.NoError(t, err)
	assert.Equal(t, a, b)
}

func TestFor_DifferentWorkspacesDiverge(t *testing.T) {
	a, _ := For("/crow", "https://github.com/acme/repo.git", "ws-1")
	b, _ := For("/crow", "https://github.com/acme/repo.git", "ws-2")
	assert.NotEqual(t, a, b)
}

func TestRepoDir_HTTPS(t *testing.T) {
	dir, err := RepoDir("/crow", "https://github.com/acme/my-repo.git")
	require.NoError(t, err)
	assert.Equal(t, "/crow/projects/github.com/acme/my-repo", dir)
}

func TestRepoDir_SSH(t *testing.T) {
	dir, err := RepoDir("/crow", "git@github.com:acme/my-repo.git")
	require.NoError(t, err)
	assert.Equal(t, "/crow/projects/github.com/acme/my-repo", dir)
}

func TestRepoDir_EmptyErrors(t *testing.T) {
	_, err := RepoDir("/crow", "")
	assert.Error(t, err)
}

func TestRepoRelPath_StripsTrailingSlash(t *testing.T) {
	path, err := For("/crow", "https://github.com/acme/repo", "ws-1")
	require.NoError(t, err)
	// Must not contain double slashes
	assert.False(t, strings.Contains(path, "//"))
	// Must be under expected base
	assert.True(t, strings.HasPrefix(path, filepath.Join("/crow", "projects")))
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd api && go test ./internal/app/usecases/internal/worktreepath/... -v 2>&1 | head -30
```
Expected: FAIL (functions not defined yet / wrong signatures).

- [ ] **Step 3: Rewrite worktreepath.go**

Replace all of `worktreepath.go` with:

```go
// Package worktreepath derives deterministic filesystem paths for git
// worktrees and per-repo directories, all rooted under ~/.crowbar.
package worktreepath

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// For returns the worktree directory for workspaceID under crowbarHome.
//
// Path: <crowbarHome>/projects/<host>/<owner>/<repo>/workspaces/<workspaceID>
//
// remoteURL accepts HTTPS (https://github.com/owner/repo.git) and SSH
// (git@github.com:owner/repo.git) formats. An empty or unrecognised URL
// returns an error.
func For(crowbarHome, remoteURL, workspaceID string) (string, error) {
	dir, err := RepoDir(crowbarHome, remoteURL)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "workspaces", workspaceID), nil
}

// RepoDir returns the per-repo directory under crowbarHome/projects/.
//
// Example: https://github.com/acme/foo.git →
//
//	<crowbarHome>/projects/github.com/acme/foo
func RepoDir(crowbarHome, remoteURL string) (string, error) {
	rel, err := repoRelPath(remoteURL)
	if err != nil {
		return "", err
	}
	return filepath.Join(crowbarHome, "projects", rel), nil
}

// DefaultCrowbarHome returns ~/.crowbar, the production root for all
// Crowbar-managed state.
func DefaultCrowbarHome() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("crowbar home: %w", err)
	}
	return filepath.Join(h, ".crowbar"), nil
}

// repoRelPath parses a git remote URL into <host>/<owner>/<repo>.
// It accepts HTTPS and SSH URL formats and strips any trailing ".git".
func repoRelPath(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("worktreepath: empty remote URL")
	}
	rawURL = strings.TrimSuffix(rawURL, ".git")

	// SSH: git@github.com:owner/repo
	if strings.HasPrefix(rawURL, "git@") {
		rest := rawURL[4:]
		idx := strings.Index(rest, ":")
		if idx < 0 {
			return "", fmt.Errorf("worktreepath: invalid SSH URL: %q", rawURL)
		}
		host := rest[:idx]
		path := strings.TrimPrefix(rest[idx+1:], "/")
		return host + "/" + path, nil
	}

	// HTTPS: https://github.com/owner/repo
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("worktreepath: unrecognised remote URL: %q", rawURL)
	}
	path := strings.TrimPrefix(u.Path, "/")
	return u.Host + "/" + path, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd api && go test ./internal/app/usecases/internal/worktreepath/... -v
```
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/internal/worktreepath/
git commit -m "refactor(worktreepath): derive path from remoteURL+wsID under ~/.crowbar"
```

---

## Task 4: Update worktree usecase to use new path formula

**Files:**
- Modify: `api/internal/app/usecases/worktree/worktree.go`

The usecase gains a `crowbarHome func() (string, error)` dependency and generates the workspace ID before computing the path (since the path now depends on the ID).

- [ ] **Step 1: Update the worktreeUsecase struct and New() signature**

In `worktree.go`, add `crowbarHome` to the struct and to `New`:

```go
type worktreeUsecase struct {
	workspaces  workspace.Workspace
	git         enginegit.Engine
	provider    engineprovider.Engine
	repos       store.Store[domain.Repository, string]
	now         func() time.Time
	crowbarHome func() (string, error)
}

func New(
	workspaces workspace.Workspace,
	git enginegit.Engine,
	provider engineprovider.Engine,
	repos store.Store[domain.Repository, string],
	now func() time.Time,
	crowbarHome func() (string, error),
) Usecase {
	return &worktreeUsecase{
		workspaces:  workspaces,
		git:         git,
		provider:    provider,
		repos:       repos,
		now:         now,
		crowbarHome: crowbarHome,
	}
}
```

- [ ] **Step 2: Add RemoteURL to CreateChildInput**

```go
type CreateChildInput struct {
	RepoID       string
	ProjectID    string
	RepoPath     string
	RemoteURL    string  // git remote origin URL; required for worktree path derivation
	Branch       string
	ParentID     string
	ParentBranch string
	ForceLocked  bool
}
```

- [ ] **Step 3: Update CreateChild to generate wsID first, then compute path**

Replace the path-computation section (currently `path := worktreepath.For(in.RepoPath, in.Branch)`) with:

```go
wsID := uuid.NewString()
home, err := u.crowbarHome()
if err != nil {
    return domain.Workspace{}, fmt.Errorf("create child: crowbar home: %w", err)
}
path, err := worktreepath.For(home, in.RemoteURL, wsID)
if err != nil {
    return domain.Workspace{}, fmt.Errorf("create child: worktree path: %w", err)
}
startSha, err := u.git.WorktreeAddBranch(ctx, in.RepoPath, path, in.Branch, in.ParentBranch)
if err != nil {
    return domain.Workspace{}, fmt.Errorf("create child: worktree add: %w", err)
}
locked, err := u.resolveLocked(ctx, in.RepoPath, in.Branch)
if err != nil {
    return domain.Workspace{}, fmt.Errorf("create child: locked: %w", err)
}
return u.workspaces.Create(ctx, workspace.CreateInput{
    ID:           wsID,   // ← use the pre-generated ID
    RepoID:       in.RepoID,
    ProjectID:    in.ProjectID,
    Branch:       in.Branch,
    WorktreePath: path,
    ForkPointSha: startSha,
    ParentID:     in.ParentID,
    Locked:       locked || in.ForceLocked,
}, u.now())
```

Also update `adoptMainWorktree` — it still uses `uuid.NewString()` for its own ID (no change needed there, it doesn't call worktreepath).

- [ ] **Step 4: Update the import of worktreepath — remove unused old imports**

`worktreepath` still imported: ✓. Remove the old `path/filepath` and `strings` that were used by the old formula if they're no longer needed elsewhere in the file (check with the build).

- [ ] **Step 5: Build to confirm it compiles (will fail on callers — that's ok)**

```bash
cd api && go build ./internal/app/usecases/worktree/... 2>&1
```
Expected: compile errors in callers (test files, container.go) — these are fixed in Tasks 5 & 6. The package itself should compile.

- [ ] **Step 6: Commit**

```bash
git add api/internal/app/usecases/worktree/worktree.go
git commit -m "refactor(worktree): inject crowbarHome; derive worktree path from remoteURL+wsID"
```

---

## Task 5: Update all worktree.New() callers

**Files:**
- Modify: `api/internal/app/usecases/worktree/worktree_test.go`
- Modify: `api/internal/app/usecases/worktree/worktree_integration_test.go`
- Modify: `api/internal/app/usecases/worktree/worktree_bench_test.go` (if it calls New)
- Modify: `api/internal/app/usecases/container.go`
- Modify: `api/internal/api/v0/endpoints/workspaces/handlers/crud.go`

This is mechanical. Every `worktree.New(...)` call adds one argument; every `CreateChildInput{...}` in real-path tests adds `RemoteURL`.

- [ ] **Step 1: Add fakeHome helper to worktree_test.go**

Near the top of the test file (after the `var errBoom` line), add:

```go
// fakeHome returns a crowbarHome function that resolves to a fixed test path.
// Unit tests use fakeGit so no real directories are created.
func fakeHome() func() (string, error) {
	return func() (string, error) { return "/tmp/crowbar-test", nil }
}
```

- [ ] **Step 2: Update every worktree.New(...) call in worktree_test.go**

Every occurrence of:
```go
worktree.New(ws, g, ..., newNow())
```
becomes:
```go
worktree.New(ws, g, ..., newNow(), fakeHome())
```

There are approximately 30 occurrences. Do a global search-and-replace within the file. The pattern is always `worktree.New(` ending with `, newNow())` — change to `, newNow(), fakeHome())`.

- [ ] **Step 3: Add RemoteURL to CreateChildInput in unit tests that exercise WorktreeAddBranch**

Find every `worktree.CreateChildInput{` in `worktree_test.go` that has a non-empty `RepoPath` (i.e., tests that go through the git path, not the `RepoPath == ""` shortcut). Add `RemoteURL: "https://github.com/test/repo.git"` to those structs.

Specifically, look for `CreateChildInput` blocks that also have `RepoPath: "/repo"` or similar non-empty paths. Tests with `RepoPath: ""` skip the worktreepath call entirely and don't need RemoteURL.

- [ ] **Step 4: Update integration test (worktree_integration_test.go)**

In `newRealUsecase`, make two changes:

1. Add `RemoteURL` to the repository Save call:
```go
require.NoError(t, repos.Save(context.Background(), domain.Repository{
    ID:        repoID,
    ProjectID: projectID,
    Name:      "repo",
    Path:      repoPath,
    RemoteURL: "https://github.com/test/integration-repo.git",
}))
```

2. Pass a temp crowbarHome to `worktree.New`:
```go
crowbarDir := t.TempDir()
uc := worktree.New(
    workspaces,
    enginegit.New(),
    prov,
    repos,
    func() time.Time { return time.Unix(1000, 0).UTC() },
    func() (string, error) { return crowbarDir, nil },
)
```

3. Update `realHarness.createChild` to include `RemoteURL`:
```go
in := worktree.CreateChildInput{
    RepoID:       h.repoID,
    ProjectID:    h.projectID,
    RepoPath:     h.repoPath,
    RemoteURL:    "https://github.com/test/integration-repo.git",
    Branch:       branch,
    ParentID:     parentID,
    ParentBranch: parentBranch,
}
```

- [ ] **Step 5: Update container.go**

```go
worktreeUsecase := worktree.New(
    repos.Workspace,
    engines.Git,
    engines.Provider,
    gormStores.Repositories,
    nowFunc,
    worktreepath.DefaultCrowbarHome,
)
```

Add the import for `worktreepath` at the top of `container.go`:
```go
"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
```

- [ ] **Step 6: Update workspaces/handlers/crud.go**

In `buildCreateInput`, add `RemoteURL` from the repo record:

```go
return worktree.CreateChildInput{
    RepoID:       repo.ID,
    ProjectID:    repo.ProjectID,
    RepoPath:     repo.Path,
    RemoteURL:    repo.RemoteURL,
    Branch:       body.Branch,
    ParentID:     body.ParentID,
    ParentBranch: parentBranch,
    ForceLocked:  locked,
}, nil
```

- [ ] **Step 7: Build and run all backend tests**

```bash
cd api && go build ./... && go test ./... -count=1 2>&1 | tail -30
```
Expected: all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add api/
git commit -m "refactor(worktree): wire crowbarHome + RemoteURL through all callers"
```

---

## Task 6: Icon HTTP endpoints

**Files:**
- Modify: `api/internal/api/v0/endpoints/repos/handlers/repos.go`
- Modify: `api/internal/api/v0/endpoints/repos/handlers/repos_test.go`
- Modify: `api/internal/api/v0/endpoints/repos/routes.go`
- Modify: `api/internal/api/v0/dto/repo.go`

### dto/repo.go — handle emoji: prefix

- [ ] **Step 1: Update RepoDTOFrom to pass emoji: values through unchanged**

```go
func RepoDTOFrom(r domain.Repository) RepoDTO {
	avatarURL := r.AvatarURL
	switch {
	case avatarURL == "":
		// no change
	case strings.HasPrefix(avatarURL, "emoji:"):
		// pass through; frontend renders emoji directly
	case strings.HasPrefix(avatarURL, "http"):
		// pass through HTTPS URLs
	default:
		// local file path — rewrite to API endpoint
		avatarURL = "/v0/repos/" + r.ID + "/icon"
	}
	return RepoDTO{
		ID:            r.ID,
		ProjectID:     r.ProjectID,
		Name:          r.Name,
		Path:          r.Path,
		DefaultBranch: r.DefaultBranch,
		AvatarLabel:   r.AvatarLabel,
		AvatarColor:   r.AvatarColor,
		AvatarURL:     avatarURL,
	}
}
```

### repos.go — add icon handlers

- [ ] **Step 2: Write tests for the new handlers first**

Add to `repos_test.go`:

```go
func TestPutIconEmoji_StoresEmojiURL(t *testing.T) {
	var saved domain.Repository
	store := &fakeStore{
		byKey: &domain.Repository{ID: "r1", Path: "/repo"},
	}
	store.SaveFn = func(_ context.Context, r domain.Repository) error {
		saved = r
		return nil
	}
	r := gin.New()
	h := repohandlers.New(store)
	r.Group("/v0").PUT("/repos/:id/icon/emoji", h.PutIconEmoji)

	body := strings.NewReader(`{"emoji":"🦊"}`)
	req := httptest.NewRequest(http.MethodPut, "/v0/repos/r1/icon/emoji", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "emoji:🦊", saved.AvatarURL)
}

func TestPutIconEmoji_MissingRepo_Returns404(t *testing.T) {
	store := &fakeStore{byKey: nil}
	r := gin.New()
	h := repohandlers.New(store)
	r.Group("/v0").PUT("/repos/:id/icon/emoji", h.PutIconEmoji)

	body := strings.NewReader(`{"emoji":"🦊"}`)
	req := httptest.NewRequest(http.MethodPut, "/v0/repos/missing/icon/emoji", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeleteIcon_ClearsAvatarURL(t *testing.T) {
	var saved domain.Repository
	store := &fakeStore{
		byKey: &domain.Repository{ID: "r1", AvatarURL: "/some/path/icon.png"},
	}
	store.SaveFn = func(_ context.Context, r domain.Repository) error {
		saved = r
		return nil
	}
	r := gin.New()
	h := repohandlers.New(store)
	r.Group("/v0").DELETE("/repos/:id/icon", h.DeleteIcon)

	req := httptest.NewRequest(http.MethodDelete, "/v0/repos/r1/icon", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "", saved.AvatarURL)
}
```

Note: `fakeStore` needs a `SaveFn` field. Add it:

```go
type fakeStore struct {
	all     []domain.Repository
	allErr  error
	byKey   *domain.Repository
	byKeErr error
	SaveFn  func(ctx context.Context, r domain.Repository) error
}

func (f *fakeStore) Save(ctx context.Context, r domain.Repository) error {
	if f.SaveFn != nil {
		return f.SaveFn(ctx, r)
	}
	return nil
}
```

Also add `"strings"` to the test imports.

- [ ] **Step 3: Run tests to see them fail**

```bash
cd api && go test ./internal/api/v0/endpoints/repos/... -v -run "TestPutIconEmoji|TestDeleteIcon" 2>&1 | head -20
```
Expected: FAIL (methods not defined).

- [ ] **Step 4: Implement PutIconEmoji, DeleteIcon, PutIcon, PutIconGithub in repos.go**

Add these methods to the `Handlers` type. Also add the helper `repoIconPath` and `repoSlugFromPath`. Add the following imports if not already present: `"io"`, `"mime"`, `"unicode"`, `"unicode/utf8"`.

```go
// PutIconEmoji handles PUT /v0/repos/:id/icon/emoji.
// Body: {"emoji":"🦊"} — stores "emoji:🦊" in avatar_url.
func (h *Handlers) PutIconEmoji(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("id"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	var body struct {
		Emoji string `json:"emoji"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Emoji == "" {
		libs.WriteErr(c, http.StatusBadRequest, "emoji required")
		return
	}
	// Validate: must be a single emoji (first rune must be the whole string after trim)
	body.Emoji = strings.TrimSpace(body.Emoji)
	if !isSingleEmoji(body.Emoji) {
		libs.WriteErr(c, http.StatusBadRequest, "emoji must be a single character")
		return
	}
	repo.AvatarURL = "emoji:" + body.Emoji
	if err := h.store.Save(c.Request.Context(), *repo); err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteIcon handles DELETE /v0/repos/:id/icon.
// Clears avatar_url and deletes any local icon file.
func (h *Handlers) DeleteIcon(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("id"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	// Delete local file if it exists
	if repo.AvatarURL != "" &&
		!strings.HasPrefix(repo.AvatarURL, "http") &&
		!strings.HasPrefix(repo.AvatarURL, "emoji:") {
		_ = os.Remove(repo.AvatarURL)
	}
	repo.AvatarURL = ""
	if err := h.store.Save(c.Request.Context(), *repo); err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

// PutIcon handles PUT /v0/repos/:id/icon (multipart/form-data, field "icon").
// Accepts image/png, image/jpeg, image/webp; max 5 MB.
func (h *Handlers) PutIcon(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("id"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	file, header, err := c.Request.FormFile("icon")
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "icon field required")
		return
	}
	defer file.Close()

	if header.Size > 5<<20 {
		libs.WriteErr(c, http.StatusBadRequest, "icon must be under 5 MB")
		return
	}

	ct := header.Header.Get("Content-Type")
	if ct == "" {
		ct = mime.TypeByExtension(filepath.Ext(header.Filename))
	}
	ext, ok := iconContentTypeExt(ct)
	if !ok {
		libs.WriteErr(c, http.StatusBadRequest, "icon must be png, jpeg, or webp")
		return
	}

	iconDir, err := repoIconDir(repo.RemoteURL)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "could not resolve icon directory")
		return
	}
	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "could not create icon directory")
		return
	}

	// Remove any previous icon files in the directory
	for _, prevExt := range []string{".png", ".jpg", ".jpeg", ".webp"} {
		_ = os.Remove(filepath.Join(iconDir, "icon"+prevExt))
	}

	iconPath := filepath.Join(iconDir, "icon"+ext)
	data, err := io.ReadAll(file)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "read error")
		return
	}
	if err := os.WriteFile(iconPath, data, 0o644); err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "write error")
		return
	}

	repo.AvatarURL = iconPath
	if err := h.store.Save(c.Request.Context(), *repo); err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"avatarUrl": "/v0/repos/" + repo.ID + "/icon"})
}

// PutIconGithub handles PUT /v0/repos/:id/icon/github.
// Re-fetches the repo owner's GitHub avatar and stores it in avatar_url.
func (h *Handlers) PutIconGithub(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("id"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	if repo.Path == "" {
		libs.WriteErr(c, http.StatusUnprocessableEntity, "repo has no local path")
		return
	}
	avatarURL := fetchGithubAvatar(c.Request.Context(), repo.Path)
	if avatarURL == "" {
		libs.WriteErr(c, http.StatusUnprocessableEntity, "could not fetch GitHub avatar")
		return
	}
	repo.AvatarURL = avatarURL
	if err := h.store.Save(c.Request.Context(), *repo); err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

// --- helpers ---

// repoIconDir returns the directory under ~/.crowbar/projects/... where
// the icon file for a repo should be stored.
func repoIconDir(remoteURL string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	crowbarHome := filepath.Join(home, ".crowbar")
	rel, err := repoRelPathFromURL(remoteURL)
	if err != nil {
		return "", err
	}
	return filepath.Join(crowbarHome, "projects", rel), nil
}

// repoRelPathFromURL parses a git remote URL into <host>/<owner>/<repo>.
// Accepts HTTPS and SSH formats.
func repoRelPathFromURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	rawURL = strings.TrimSuffix(rawURL, ".git")
	if rawURL == "" {
		return "", fmt.Errorf("empty remote URL")
	}
	if strings.HasPrefix(rawURL, "git@") {
		rest := rawURL[4:]
		idx := strings.Index(rest, ":")
		if idx < 0 {
			return "", fmt.Errorf("invalid SSH URL")
		}
		return rest[:idx] + "/" + strings.TrimPrefix(rest[idx+1:], "/"), nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("unrecognised URL: %q", rawURL)
	}
	return u.Host + "/" + strings.TrimPrefix(u.Path, "/"), nil
}

// iconContentTypeExt maps accepted content types to file extensions.
func iconContentTypeExt(ct string) (string, bool) {
	// Strip params (e.g. "image/png; charset=...")
	if idx := strings.Index(ct, ";"); idx >= 0 {
		ct = strings.TrimSpace(ct[:idx])
	}
	m := map[string]string{
		"image/png":  ".png",
		"image/jpeg": ".jpg",
		"image/webp": ".webp",
	}
	ext, ok := m[ct]
	return ext, ok
}

// isSingleEmoji returns true when s is a non-empty string containing exactly
// one Unicode code point that is not a plain ASCII letter/digit.
func isSingleEmoji(s string) bool {
	if s == "" {
		return false
	}
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return false
	}
	if size != len(s) {
		return false // more than one rune
	}
	return !unicode.IsLetter(r) || r > 127
}

// fetchGithubAvatar runs `gh api repos/<slug> --jq .owner.avatar_url` in
// repoPath. Returns "" on any failure.
func fetchGithubAvatar(ctx context.Context, repoPath string) string {
	raw, err := exec.CommandContext(ctx, "git", "-C", repoPath, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	remoteURL := strings.TrimSpace(string(raw))
	slug, err := githubSlugFromURL(remoteURL)
	if err != nil {
		return ""
	}
	out, err := exec.CommandContext(ctx, "gh", "api", "repos/"+slug, "--jq", ".owner.avatar_url").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// githubSlugFromURL extracts "owner/repo" from a GitHub remote URL.
func githubSlugFromURL(rawURL string) (string, error) {
	rawURL = strings.TrimSuffix(strings.TrimSpace(rawURL), ".git")
	if strings.HasPrefix(rawURL, "git@") {
		parts := strings.SplitN(rawURL, ":", 2)
		if len(parts) == 2 {
			return parts[1], nil
		}
	}
	if idx := strings.Index(rawURL, "://"); idx >= 0 {
		path := rawURL[idx+3:]
		slash := strings.Index(path, "/")
		if slash >= 0 {
			return path[slash+1:], nil
		}
	}
	return "", fmt.Errorf("unrecognised URL: %q", rawURL)
}
```

Add missing imports to `repos.go`: `"fmt"`, `"io"`, `"mime"`, `"net/url"`, `"unicode"`, `"unicode/utf8"`.

- [ ] **Step 5: Update routes.go to register new endpoints**

```go
func Register(
	rg *gin.RouterGroup,
	store repohandlers.Store,
	prov repohandlers.BranchProviderEngine,
	wsReader repohandlers.WorkspaceReader,
) {
	h := repohandlers.NewWithDeps(store, prov, wsReader)
	rg.POST("/repos", h.Create)
	rg.GET("/repos", h.List)
	rg.GET("/repos/:id", h.Detail)
	rg.GET("/repos/:id/icon", h.Icon)
	rg.PUT("/repos/:id/icon", h.PutIcon)
	rg.DELETE("/repos/:id/icon", h.DeleteIcon)
	rg.PUT("/repos/:id/icon/emoji", h.PutIconEmoji)
	rg.PUT("/repos/:id/icon/github", h.PutIconGithub)
	rg.GET("/repos/:id/branches", h.Branches)
}
```

- [ ] **Step 6: Run tests**

```bash
cd api && go test ./internal/api/v0/endpoints/repos/... -v 2>&1 | tail -20
```
Expected: all PASS including new emoji and delete tests.

- [ ] **Step 7: Run full backend build + tests**

```bash
cd api && go build ./... && go test ./... -count=1 2>&1 | tail -20
```
Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add api/
git commit -m "feat(repos): add icon upload/emoji/github/delete endpoints; centralise under ~/.crowbar"
```

---

## Task 7: Frontend — useSidebarNavStore

**Files:**
- Create: `web/src/features/layout/stores/sidebar-nav.ts`
- Create: `web/src/__tests__/features/layout/stores/sidebar-nav.test.ts`

- [ ] **Step 1: Write the failing tests**

```ts
// web/src/__tests__/features/layout/stores/sidebar-nav.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'

beforeEach(() => {
  useSidebarNavStore.getState().reset()
})

describe('useSidebarNavStore', () => {
  it('starts with empty stack', () => {
    expect(useSidebarNavStore.getState().stack).toHaveLength(0)
  })

  it('push adds a screen to the stack', () => {
    useSidebarNavStore.getState().push({ id: 'a', title: 'A', component: null })
    expect(useSidebarNavStore.getState().stack).toHaveLength(1)
    expect(useSidebarNavStore.getState().stack[0].id).toBe('a')
  })

  it('push with duplicate id is a no-op', () => {
    useSidebarNavStore.getState().push({ id: 'a', title: 'A', component: null })
    useSidebarNavStore.getState().push({ id: 'a', title: 'A2', component: null })
    expect(useSidebarNavStore.getState().stack).toHaveLength(1)
    expect(useSidebarNavStore.getState().stack[0].title).toBe('A') // original unchanged
  })

  it('pop removes the last screen', () => {
    useSidebarNavStore.getState().push({ id: 'a', title: 'A', component: null })
    useSidebarNavStore.getState().push({ id: 'b', title: 'B', component: null })
    useSidebarNavStore.getState().pop()
    expect(useSidebarNavStore.getState().stack).toHaveLength(1)
    expect(useSidebarNavStore.getState().stack[0].id).toBe('a')
  })

  it('pop on empty stack is a no-op', () => {
    expect(() => useSidebarNavStore.getState().pop()).not.toThrow()
    expect(useSidebarNavStore.getState().stack).toHaveLength(0)
  })

  it('reset clears the stack', () => {
    useSidebarNavStore.getState().push({ id: 'a', title: 'A', component: null })
    useSidebarNavStore.getState().push({ id: 'b', title: 'B', component: null })
    useSidebarNavStore.getState().reset()
    expect(useSidebarNavStore.getState().stack).toHaveLength(0)
  })
})
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd web && npx vitest run src/__tests__/features/layout/stores/sidebar-nav.test.ts 2>&1 | tail -10
```
Expected: FAIL (module not found).

- [ ] **Step 3: Create the store**

```ts
// web/src/features/layout/stores/sidebar-nav.ts
import type { ReactNode } from 'react'
import { create } from 'zustand'

export interface SidebarScreen {
  id: string
  title: string
  component: ReactNode
}

interface SidebarNavStore {
  stack: SidebarScreen[]
  push: (screen: SidebarScreen) => void
  pop: () => void
  reset: () => void
}

export const useSidebarNavStore = create<SidebarNavStore>((set, get) => ({
  stack: [],
  push: (screen) => {
    if (get().stack.some((s) => s.id === screen.id)) return
    set((state) => ({ stack: [...state.stack, screen] }))
  },
  pop: () => {
    set((state) => ({ stack: state.stack.slice(0, -1) }))
  },
  reset: () => set({ stack: [] }),
}))
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd web && npx vitest run src/__tests__/features/layout/stores/sidebar-nav.test.ts
```
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/layout/stores/sidebar-nav.ts web/src/__tests__/features/layout/stores/sidebar-nav.test.ts
git commit -m "feat(sidebar): add useSidebarNavStore with push/pop/reset"
```

---

## Task 8: Frontend — NavStack component

**Files:**
- Create: `web/src/components/layout/nav-stack.tsx`
- Create: `web/src/__tests__/components/layout/nav-stack.test.tsx`

- [ ] **Step 1: Write the failing tests**

```tsx
// web/src/__tests__/components/layout/nav-stack.test.tsx
import React from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach } from 'vitest'
import { NavStack } from '@/components/layout/nav-stack'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'

beforeEach(() => {
  useSidebarNavStore.getState().reset()
})

describe('NavStack', () => {
  it('renders children (root screen) when stack is empty', () => {
    render(<NavStack><div data-testid="root">Root</div></NavStack>)
    expect(screen.getByTestId('root')).toBeTruthy()
  })

  it('renders a pushed screen on top of root', () => {
    useSidebarNavStore.getState().push({
      id: 'test',
      title: 'Test Screen',
      component: <div data-testid="pushed">Pushed</div>,
    })
    render(<NavStack><div data-testid="root">Root</div></NavStack>)
    expect(screen.getByTestId('pushed')).toBeTruthy()
    expect(screen.getByText('Test Screen')).toBeTruthy()
  })

  it('back button pops the screen', async () => {
    useSidebarNavStore.getState().push({
      id: 'test',
      title: 'Test Screen',
      component: <div>Content</div>,
    })
    render(<NavStack><div>Root</div></NavStack>)
    await userEvent.click(screen.getByRole('button', { name: /back/i }))
    expect(useSidebarNavStore.getState().stack).toHaveLength(0)
  })
})
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd web && npx vitest run src/__tests__/components/layout/nav-stack.test.tsx 2>&1 | tail -10
```
Expected: FAIL.

- [ ] **Step 3: Create nav-stack.tsx**

```tsx
// web/src/components/layout/nav-stack.tsx
import { type ReactNode } from 'react'
import { ChevronLeft } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'

interface NavStackProps {
  children: ReactNode
}

export function NavStack({ children }: NavStackProps) {
  const stack = useSidebarNavStore((s) => s.stack)
  const pop = useSidebarNavStore((s) => s.pop)
  const hasScreen = stack.length > 0
  const topScreen = stack[stack.length - 1]

  return (
    <div className="relative flex flex-1 flex-col overflow-hidden">
      {/* Root layer — always mounted, pushed back when a screen is active */}
      <div
        className={cn(
          'absolute inset-0 flex flex-col transition-[transform,opacity] duration-[280ms] ease-[cubic-bezier(0.4,0,0.2,1)]',
          hasScreen ? '-translate-x-1/4 opacity-40 pointer-events-none' : 'translate-x-0 opacity-100',
        )}
      >
        {children}
      </div>

      {/* Pushed screens */}
      {stack.map((screen, i) => {
        const isTop = i === stack.length - 1
        return (
          <div
            key={screen.id}
            className={cn(
              'absolute inset-0 flex flex-col bg-sidebar transition-[transform,opacity] duration-[280ms] ease-[cubic-bezier(0.4,0,0.2,1)]',
              isTop ? 'translate-x-0 opacity-100' : '-translate-x-1/4 opacity-0 pointer-events-none',
              !isTop && 'translate-x-full',
            )}
          >
            {/* Header with back button */}
            <div className="flex items-center gap-2 border-b border-border px-2.5 py-2 flex-shrink-0">
              <button
                aria-label="Back"
                onClick={pop}
                className="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-md bg-accent/50 text-muted-foreground hover:bg-accent hover:text-foreground"
              >
                <ChevronLeft className="size-3.5" />
              </button>
              <span className="truncate font-mono text-[11px] text-foreground">
                {screen.title}
              </span>
            </div>
            <div className="flex flex-1 flex-col overflow-hidden">
              {screen.component}
            </div>
          </div>
        )
      })}
    </div>
  )
}
```

- [ ] **Step 4: Run tests**

```bash
cd web && npx vitest run src/__tests__/components/layout/nav-stack.test.tsx
```
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/nav-stack.tsx web/src/__tests__/components/layout/nav-stack.test.tsx
git commit -m "feat(sidebar): add NavStack component with iOS-style push transition"
```

---

## Task 9: Wire NavStack into SidebarCarousel + workspace-tree

**Files:**
- Modify: `web/src/components/layout/sidebar-carousel.tsx`
- Modify: `web/src/components/layout/workspace-tree.tsx`

- [ ] **Step 1: Wrap WorkspaceTree in NavStack inside sidebar-carousel.tsx**

In `sidebar-carousel.tsx`, find the workspaces panel div and replace `<WorkspaceTree />` with:

```tsx
import { NavStack } from './nav-stack'

{/* Workspaces panel */}
<div className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden">
  <NavStack>
    <WorkspaceTree />
  </NavStack>
</div>
```

- [ ] **Step 2: Update workspace-tree.tsx — gear click pushes nav instead of opening Sheet**

Remove the `openSettingsRepoId` / `setOpenSettingsRepoId` state and the `<RepoSettingsPanel>` rendering at the bottom. Replace the gear button click handler:

Remove:
```tsx
const [openSettingsRepoId, setOpenSettingsRepoId] = useState<string | null>(null)
```

Change gear button `onClick`:
```tsx
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import { RepoSettingsPanel } from './repo-settings-panel'

// in the gear button:
onClick={(e) => {
  e.stopPropagation()
  useSidebarNavStore.getState().push({
    id: `repo-settings:${repo.id}`,
    title: repo.name,
    component: <RepoSettingsPanel repoId={repo.id} repoName={repo.name} />,
  })
}}
```

Remove the entire block at the bottom of `WorkspaceTreeInner` that conditionally renders `<RepoSettingsPanel>`.

Also remove the `RepoSettingsPanel` import if it's now only used in the push call (keep the import — it's used in the `component` prop).

- [ ] **Step 3: Run frontend tests**

```bash
cd web && npx vitest run 2>&1 | tail -20
```
Expected: all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/components/layout/sidebar-carousel.tsx web/src/components/layout/workspace-tree.tsx
git commit -m "feat(sidebar): wire NavStack into carousel; gear click pushes nav"
```

---

## Task 10: RepoSettingsPanel — remove Sheet, add icon picker

**Files:**
- Modify: `web/src/components/layout/repo-settings-panel.tsx`

The panel becomes a plain scrollable component. It gains an icon section at the top.

- [ ] **Step 1: Rewrite repo-settings-panel.tsx**

```tsx
// web/src/components/layout/repo-settings-panel.tsx
import { useState, useEffect, useRef } from 'react'
import { Checkbox } from '@/components/ui/checkbox'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { apiFetch } from '@/lib/api'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useSidebarStore } from '@/lib/store/sidebar'
import { Lock, GitBranch, Trash2 } from 'lucide-react'
import { cn } from '@/lib/utils'

interface BranchEntry {
  name: string
  isProtected: boolean
  hasWorkspace: boolean
}

interface RepoSettingsPanelProps {
  repoId: string
  repoName: string
}

export function RepoSettingsPanel({ repoId, repoName }: RepoSettingsPanelProps) {
  const [branches, setBranches] = useState<BranchEntry[]>([])
  const [filter, setFilter] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [importing, setImporting] = useState(false)
  const [emojiInput, setEmojiInput] = useState('')
  const [iconLoading, setIconLoading] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  // Current repo avatar from store
  const repo = useSidebarStore((s) => s.repos.find((r) => r.id === repoId))

  useEffect(() => {
    setBranches([])
    setSelected(new Set())
    setFilter('')
    apiFetch<BranchEntry[]>(`/v0/repos/${repoId}/branches`)
      .then(setBranches)
      .catch(() => {})
  }, [repoId])

  const visible = branches.filter((b) =>
    b.name.toLowerCase().includes(filter.toLowerCase())
  )

  async function handleImport() {
    if (selected.size === 0) return
    setImporting(true)
    try {
      await Promise.all(
        Array.from(selected).map((branch) =>
          apiFetch('/v0/workspaces', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ repoId, branch }),
          }).catch(() => {})
        )
      )
      void useWorkspaceListStore.getState().fetch()
    } finally {
      setImporting(false)
    }
  }

  function toggleBranch(name: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    setIconLoading(true)
    try {
      const form = new FormData()
      form.append('icon', file)
      await apiFetch(`/v0/repos/${repoId}/icon`, { method: 'PUT', body: form })
      void useWorkspaceListStore.getState().fetch()
    } catch {
      // ignore
    } finally {
      setIconLoading(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  async function handleEmojiSubmit() {
    const emoji = emojiInput.trim()
    if (!emoji) return
    setIconLoading(true)
    try {
      await apiFetch(`/v0/repos/${repoId}/icon/emoji`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ emoji }),
      })
      setEmojiInput('')
      void useWorkspaceListStore.getState().fetch()
    } catch {
      // ignore
    } finally {
      setIconLoading(false)
    }
  }

  async function handleGithubAvatar() {
    setIconLoading(true)
    try {
      await apiFetch(`/v0/repos/${repoId}/icon/github`, { method: 'PUT' })
      void useWorkspaceListStore.getState().fetch()
    } catch {
      // ignore
    } finally {
      setIconLoading(false)
    }
  }

  async function handleResetIcon() {
    setIconLoading(true)
    try {
      await apiFetch(`/v0/repos/${repoId}/icon`, { method: 'DELETE' })
      void useWorkspaceListStore.getState().fetch()
    } catch {
      // ignore
    } finally {
      setIconLoading(false)
    }
  }

  const importable = selected.size

  return (
    <ScrollArea className="flex-1">
      <div className="flex flex-col gap-4 p-3">

        {/* Icon section */}
        <div className="flex flex-col gap-2">
          <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            Icon
          </p>

          {/* Current icon preview */}
          <div className="flex items-center gap-3 rounded-md border border-border bg-accent/30 p-2.5">
            {repo?.avatarURL ? (
              repo.avatarURL.startsWith('emoji:') ? (
                <span className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-md text-2xl">
                  {repo.avatarURL.slice(6)}
                </span>
              ) : (
                <img
                  src={repo.avatarURL}
                  alt={repoName}
                  className="h-9 w-9 flex-shrink-0 rounded-md object-cover"
                />
              )
            ) : (
              <span
                className={cn(
                  'flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-md text-sm font-bold text-primary-foreground',
                  repo?.avatarColor,
                )}
              >
                {repo?.avatarLabel}
              </span>
            )}
            <div className="flex flex-col gap-1.5 flex-1">
              {/* Upload */}
              <input
                ref={fileRef}
                type="file"
                accept="image/png,image/jpeg,image/webp"
                className="hidden"
                onChange={handleFileChange}
              />
              <button
                onClick={() => fileRef.current?.click()}
                disabled={iconLoading}
                className="text-left text-[10.5px] text-muted-foreground hover:text-foreground disabled:opacity-50"
              >
                📁 Upload image
              </button>
              {/* Emoji */}
              <div className="flex items-center gap-1.5">
                <input
                  value={emojiInput}
                  onChange={(e) => setEmojiInput(e.target.value)}
                  onKeyDown={(e) => { if (e.key === 'Enter') void handleEmojiSubmit() }}
                  placeholder="😀 Type emoji…"
                  maxLength={4}
                  className="h-6 w-24 rounded border border-border bg-background px-1.5 text-[10.5px] outline-none focus:border-ring"
                />
                {emojiInput && (
                  <button
                    onClick={() => void handleEmojiSubmit()}
                    disabled={iconLoading}
                    className="text-[10px] text-muted-foreground hover:text-foreground"
                  >
                    Set
                  </button>
                )}
              </div>
              {/* GitHub */}
              <button
                onClick={() => void handleGithubAvatar()}
                disabled={iconLoading}
                className="text-left text-[10.5px] text-muted-foreground hover:text-foreground disabled:opacity-50"
              >
                🐙 Use GitHub avatar
              </button>
            </div>
            {/* Reset */}
            {repo?.avatarURL && (
              <button
                onClick={() => void handleResetIcon()}
                disabled={iconLoading}
                aria-label="Reset icon"
                className="flex-shrink-0 text-muted-foreground/50 hover:text-destructive"
              >
                <Trash2 className="size-3" />
              </button>
            )}
          </div>
        </div>

        {/* Branch import section */}
        <div className="flex flex-col gap-2">
          <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            Import Workspaces
          </p>

          <Input
            placeholder="Filter branches…"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="h-7 text-xs"
          />

          <div className="flex flex-col gap-0.5">
            {visible.some((b) => b.isProtected) && (
              <>
                <p className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                  Protected — auto-imported
                </p>
                {visible.filter((b) => b.isProtected).map((b) => (
                  <label
                    key={b.name}
                    className="flex cursor-default items-center gap-2 rounded px-2 py-1.5 text-xs opacity-60"
                  >
                    <Checkbox checked={true} disabled={true} ariaLabel={b.name} />
                    <Lock className="size-3 shrink-0 text-muted-foreground" />
                    <span className="min-w-0 flex-1 truncate font-mono">{b.name}</span>
                  </label>
                ))}
              </>
            )}

            {visible.some((b) => !b.isProtected) && (
              <>
                <p className="mb-1 mt-3 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                  Other branches
                </p>
                {visible.filter((b) => !b.isProtected).map((b) => (
                  <label
                    key={b.name}
                    className={cn(
                      'flex items-center gap-2 rounded px-2 py-1.5 text-xs hover:bg-accent',
                      b.hasWorkspace ? 'cursor-default opacity-60' : 'cursor-pointer',
                    )}
                  >
                    {!b.hasWorkspace ? (
                      <Checkbox
                        checked={selected.has(b.name)}
                        onChange={() => toggleBranch(b.name)}
                        ariaLabel={b.name}
                      />
                    ) : (
                      <Checkbox checked={true} disabled={true} ariaLabel={b.name} />
                    )}
                    <GitBranch className="size-3 shrink-0 text-muted-foreground" />
                    <span className="min-w-0 flex-1 truncate font-mono">{b.name}</span>
                    {b.hasWorkspace && (
                      <span className="shrink-0 text-[10px] text-green-500">imported</span>
                    )}
                  </label>
                ))}
              </>
            )}
          </div>

          <Button
            size="sm"
            disabled={selected.size === 0 || importing}
            onClick={handleImport}
          >
            {importing
              ? 'Importing…'
              : importable > 0
              ? `Import ${importable} branch${importable > 1 ? 'es' : ''}`
              : 'Import'}
          </Button>
        </div>

      </div>
    </ScrollArea>
  )
}
```

- [ ] **Step 2: Update avatarURL rendering in workspace-tree.tsx to handle emoji: prefix**

In `workspace-tree.tsx`, find the avatar rendering block (currently checks `repo.avatarURL` to decide between `<img>` and initials span). Update it to handle the emoji prefix:

```tsx
{repo.avatarURL?.startsWith('emoji:') ? (
  <span className="inline-flex h-4 w-4 shrink-0 items-center justify-center text-base leading-none">
    {repo.avatarURL.slice(6)}
  </span>
) : repo.avatarURL ? (
  <img
    src={repo.avatarURL}
    alt={repo.name}
    className="h-4 w-4 shrink-0 rounded object-cover"
  />
) : (
  <span
    className={cn(
      'inline-flex h-4 w-4 shrink-0 items-center justify-center rounded px-1 text-[10px] font-bold text-primary-foreground',
      repo.avatarColor,
    )}
  >
    {repo.avatarLabel}
  </span>
)}
```

- [ ] **Step 3: Run frontend tests**

```bash
cd web && npx vitest run 2>&1 | tail -20
```
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/components/layout/repo-settings-panel.tsx web/src/components/layout/workspace-tree.tsx
git commit -m "feat(repo-settings): inline panel with icon picker; push onto sidebar nav"
```

---

## Post-implementation checklist

- [ ] `cd api && go build ./... && go test ./... -count=1` — all green
- [ ] `cd web && npx vitest run` — all green
- [ ] `cd web && npx tsc --noEmit` — no type errors
- [ ] Manually: import a repo → sidebar shows new path under `~/.crowbar/projects/`
- [ ] Manually: hover repo row → click gear → sidebar pushes settings screen
- [ ] Manually: upload icon → avatar updates in sidebar immediately
- [ ] Manually: set emoji icon → emoji shows in sidebar
- [ ] Manually: back button → sidebar animates back to workspace tree
- [ ] Clear `~/.crowbar-worktrees/` on dev machine and reimport repos
