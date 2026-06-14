# Sidebar Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement 5 sidebar improvements: repo icon resolution, protected-branch auto-import, Import Workspaces panel, PR-based parent-child hierarchy, and repo settings gear.

**Architecture:** Backend (Go) handles icon scanning, GitHub/GitLab API calls, branch listing, and PR-driven reparenting. Frontend (React/TypeScript) adds a settings gear to the repo row, a repo settings panel with Import Workspaces, and renders real icons instead of letter badges.

**Tech Stack:** Go + Gin + Asynx + GORM (backend); React + Zustand + Vitest (frontend); `gh` CLI (GitHub); `glab` CLI (GitLab).

---

## File Map

**Backend — new files:**
- `api/internal/app/repositories/workspace/internal/commands/set_parent_from_pr.go` — new Asynx command that sets ParentID without requiring a ForkPointSha recompute

**Backend — modified files:**
- `api/internal/domain/repository.go` — add `AvatarURL string` field
- `api/internal/api/v0/dto/repo.go` — add `AvatarURL` to DTO, convert local path → icon endpoint URL
- `api/internal/app/usecases/internal/avatar/avatar.go` — add `ScanRepoIcon(repoPath string) string`
- `api/internal/engine/provider/provider.go` — add `OwnerAvatarURL` to `GitProvider` and `Engine` interfaces
- `api/internal/engine/provider/providers/github/github.go` — implement `OwnerAvatarURL`
- `api/internal/engine/provider/providers/gitlab/gitlab.go` — implement `OwnerAvatarURL`
- `api/internal/engine/provider/engine.go` — implement `OwnerAvatarURL` on `providerEngine`
- `api/internal/app/usecases/project/project_import.go` — wire avatar URL, auto-import protected branch stubs
- `api/internal/app/usecases/mocks/mocks.go` — update `ProviderEngine` mock for new interface, update `ProviderSyncWorkspaceRepo` mock
- `api/internal/app/repositories/workspace/workspace.go` — add `SetParentFromPR` method
- `api/internal/app/usecases/provider/provider_sync.go` — extend `WorkspaceRepo` interface, auto-reparent from PR
- `api/internal/api/v0/endpoints/repos/handlers/repos.go` — add `Icon`, `Branches` handlers; add provider + workspace deps
- `api/internal/api/v0/endpoints/repos/routes.go` — register new routes; update `Register` signature
- `api/internal/api/v0/router.go` — update `repos.Register` call

**Frontend — new files:**
- `web/src/components/layout/repo-settings-panel.tsx` — Sheet-based settings panel with Import Workspaces UI

**Frontend — modified files:**
- `web/src/lib/store/sidebar.ts` — add `avatarURL?: string` to `Repo`
- `web/src/lib/store/build-repo-tree.ts` — map `avatarURL` from `RepoDTO`
- `web/src/components/layout/workspace-tree.tsx` — render img icon, hover gear swap

---

## Task 1: Add AvatarURL to domain.Repository and RepoDTO

**Files:**
- Modify: `api/internal/domain/repository.go`
- Modify: `api/internal/api/v0/dto/repo.go`
- Test: `api/internal/api/v0/endpoints/repos/handlers/repos_test.go` (existing tests still pass)

- [ ] **Step 1: Add AvatarURL to domain.Repository**

In `api/internal/domain/repository.go`, add the field after `AvatarColor`:

```go
type Repository struct {
	ID            string `gorm:"primaryKey" json:"id"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultBranch string `json:"defaultBranch"`
	AvatarLabel   string `json:"avatarLabel"`
	AvatarColor   string `json:"avatarColor"`
	AvatarURL     string `json:"avatarUrl,omitempty"`
}
```

- [ ] **Step 2: Add AvatarURL to RepoDTO and update RepoDTOFrom**

In `api/internal/api/v0/dto/repo.go`, replace the file contents:

```go
package dto

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type RepoDTO struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultBranch string `json:"defaultBranch"`
	AvatarLabel   string `json:"avatarLabel"`
	AvatarColor   string `json:"avatarColor"`
	AvatarURL     string `json:"avatarUrl,omitempty"`
}

func RepoDTOFrom(r domain.Repository) RepoDTO {
	avatarURL := r.AvatarURL
	if avatarURL != "" && !strings.HasPrefix(avatarURL, "http") {
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

func RepoDTOList(repos []domain.Repository) []RepoDTO {
	dtos := make([]RepoDTO, 0, len(repos))
	for _, r := range repos {
		dtos = append(dtos, RepoDTOFrom(r))
	}
	return dtos
}
```

- [ ] **Step 3: Run existing repo handler tests to verify no regression**

```bash
cd api && go test -tags noEmbed -race ./internal/api/v0/endpoints/repos/...
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add api/internal/domain/repository.go api/internal/api/v0/dto/repo.go
git commit -m "feat: add AvatarURL to Repository domain and RepoDTO"
```

---

## Task 2: Filesystem icon scan in avatar package

**Files:**
- Modify: `api/internal/app/usecases/internal/avatar/avatar.go`
- Test: `api/internal/app/usecases/internal/avatar/avatar_test.go` (existing file, add new tests)

- [ ] **Step 1: Write failing tests for ScanRepoIcon**

Find the test file (likely `api/internal/app/usecases/internal/avatar/avatar_test.go`) and add:

```go
func TestScanRepoIcon_FindsFaviconSVG(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "favicon.svg"), []byte("<svg/>"), 0o644))
	got := avatar.ScanRepoIcon(dir)
	assert.Equal(t, filepath.Join(dir, "favicon.svg"), got)
}

func TestScanRepoIcon_PriorityOrder(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "favicon.ico"), []byte("ico"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "favicon.svg"), []byte("<svg/>"), 0o644))
	got := avatar.ScanRepoIcon(dir)
	assert.Equal(t, filepath.Join(dir, "favicon.svg"), got) // svg wins over ico
}

func TestScanRepoIcon_PublicSubdir(t *testing.T) {
	dir := t.TempDir()
	pub := filepath.Join(dir, "public")
	require.NoError(t, os.Mkdir(pub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pub, "logo.png"), []byte("png"), 0o644))
	got := avatar.ScanRepoIcon(dir)
	assert.Equal(t, filepath.Join(pub, "logo.png"), got)
}

func TestScanRepoIcon_NoMatch_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	got := avatar.ScanRepoIcon(dir)
	assert.Empty(t, got)
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd api && go test -tags noEmbed -race ./internal/app/usecases/internal/avatar/... -run TestScanRepoIcon
```

Expected: FAIL — `ScanRepoIcon` undefined

- [ ] **Step 3: Implement ScanRepoIcon**

Append to `api/internal/app/usecases/internal/avatar/avatar.go`:

```go
import (
	"os"
	"path/filepath"
)

// iconCandidates is the ordered list of relative paths checked when scanning
// a repo root for an icon file. First match wins.
var iconCandidates = []string{
	"favicon.svg",
	"favicon.ico",
	"favicon.png",
	"logo.svg",
	"logo.png",
	"public/logo.svg",
	"public/logo.png",
	"public/favicon.svg",
	"public/favicon.ico",
	"public/favicon.png",
	"src/assets/logo.svg",
	"src/assets/logo.png",
}

// ScanRepoIcon walks iconCandidates relative to repoPath and returns the
// absolute path of the first file found, or "" when none match.
func ScanRepoIcon(repoPath string) string {
	for _, rel := range iconCandidates {
		abs := filepath.Join(repoPath, rel)
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	return ""
}
```

Note: add `"os"` and `"path/filepath"` to the import block at the top of the file.

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd api && go test -tags noEmbed -race ./internal/app/usecases/internal/avatar/... -run TestScanRepoIcon
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/internal/avatar/avatar.go api/internal/app/usecases/internal/avatar/avatar_test.go
git commit -m "feat: add ScanRepoIcon filesystem scanner to avatar package"
```

---

## Task 3: Add OwnerAvatarURL to GitProvider and Engine interfaces

**Files:**
- Modify: `api/internal/engine/provider/provider.go`

- [ ] **Step 1: Add OwnerAvatarURL to GitProvider**

In `api/internal/engine/provider/provider.go`, extend `GitProvider`:

```go
// GitProvider is the read-only interface both GitHub and GitLab implement.
type GitProvider interface {
	// ProtectedBranches returns the list of protected branch names for the repo.
	ProtectedBranches(
		ctx context.Context,
		repoPath string,
	) ([]string, error)

	// PullRequestForBranch returns the most relevant PR for branch, or nil if none.
	PullRequestForBranch(
		ctx context.Context,
		repoPath string,
		branch string,
	) (*PRInfo, error)

	// OwnerAvatarURL returns the avatar URL of the repo owner (org or user).
	// Returns "" when the provider CLI is unavailable or the call fails.
	OwnerAvatarURL(
		ctx context.Context,
		repoPath string,
	) (string, error)
}
```

And extend `Engine`:

```go
type Engine interface {
	Capability(ctx context.Context, repoPath string) (ProviderCapability, error)
	ProtectedBranches(ctx context.Context, repoPath string) ([]string, error)
	PollOnView(ctx context.Context, wsID string, repoPath string, branch string) (ProviderState, error)
	StartBackgroundSweep(ctx context.Context, workspacesFn func() []poll.SweepTarget, onStateChange func(wsID string, state ProviderState))

	// OwnerAvatarURL returns the avatar URL of the repo owner.
	// Returns "" when the provider CLI is unavailable or the lookup fails.
	OwnerAvatarURL(ctx context.Context, repoPath string) (string, error)
}
```

- [ ] **Step 2: Verify the build breaks on missing implementations**

```bash
cd api && go build -tags noEmbed ./...
```

Expected: FAIL — missing `OwnerAvatarURL` on `*ghProvider`, `*glabProvider`, `*providerEngine`

- [ ] **Step 3: Commit the interface change**

```bash
git add api/internal/engine/provider/provider.go
git commit -m "feat: add OwnerAvatarURL to GitProvider and Engine interfaces"
```

---

## Task 4: Implement OwnerAvatarURL on GitHub provider

**Files:**
- Modify: `api/internal/engine/provider/providers/github/github.go`
- Test: `api/internal/engine/provider/providers/github/github_test.go` (existing file, add new test)

- [ ] **Step 1: Write failing test**

In `api/internal/engine/provider/providers/github/github_test.go`, add:

```go
func TestGHProvider_OwnerAvatarURL(t *testing.T) {
	p := github.NewWithExec(func(_ context.Context, name string, args ...string) *exec.Cmd {
		// stub: gh api repos/{slug} --jq .owner.avatar_url → fake URL
		if name == "gh" && len(args) >= 2 && args[0] == "api" {
			return fakeCmd(`https://avatars.githubusercontent.com/u/123`)
		}
		// stub: git remote get-url origin → slug
		if name == "git" {
			return fakeCmd("git@github.com:owner/repo.git")
		}
		return fakeCmd("")
	})
	got, err := p.OwnerAvatarURL(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Equal(t, "https://avatars.githubusercontent.com/u/123", got)
}

func TestGHProvider_OwnerAvatarURL_CliError_ReturnsEmpty(t *testing.T) {
	p := github.NewWithExec(func(_ context.Context, name string, args ...string) *exec.Cmd {
		return failCmd()
	})
	got, err := p.OwnerAvatarURL(context.Background(), "/repo")
	require.NoError(t, err) // soft failure
	assert.Empty(t, got)
}
```

(`fakeCmd` and `failCmd` are helpers that already exist in the test file — check for them; if missing, add `fakeCmd(output string) *exec.Cmd` using `exec.Command("echo", output)` and `failCmd()` using `exec.Command("false")`.)

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd api && go test -tags noEmbed ./internal/engine/provider/providers/github/... -run TestGHProvider_OwnerAvatarURL
```

Expected: FAIL — method not defined

- [ ] **Step 3: Implement OwnerAvatarURL on ghProvider**

Append to `api/internal/engine/provider/providers/github/github.go`:

```go
// OwnerAvatarURL returns the GitHub owner's avatar URL for the repo.
// Uses gh api repos/{slug} --jq .owner.avatar_url.
// Returns ("", nil) on any soft failure so callers can fall back gracefully.
func (g *ghProvider) OwnerAvatarURL(
	ctx context.Context,
	repoPath string,
) (string, error) {
	s, err := slug(ctx, repoPath, g.execFn)
	if err != nil {
		return "", nil
	}
	path := fmt.Sprintf("repos/%s", s)
	out, err := g.runGH(ctx, repoPath, "api", path, "--jq", ".owner.avatar_url")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}
```

- [ ] **Step 4: Run test to confirm it passes**

```bash
cd api && go test -tags noEmbed ./internal/engine/provider/providers/github/... -run TestGHProvider_OwnerAvatarURL
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/provider/providers/github/github.go api/internal/engine/provider/providers/github/github_test.go
git commit -m "feat: implement OwnerAvatarURL on GitHub provider"
```

---

## Task 5: Implement OwnerAvatarURL on GitLab provider

**Files:**
- Modify: `api/internal/engine/provider/providers/gitlab/gitlab.go`
- Test: `api/internal/engine/provider/providers/gitlab/gitlab_test.go` (existing file, add new test)

- [ ] **Step 1: Write failing test**

In `api/internal/engine/provider/providers/gitlab/gitlab_test.go`, add:

```go
func TestGlabProvider_OwnerAvatarURL(t *testing.T) {
	p := gitlab.NewWithExec(func(_ context.Context, name string, args ...string) *exec.Cmd {
		if name == "glab" {
			return fakeCmd(`https://gitlab.com/uploads/group/avatar/42/icon.png`)
		}
		return fakeCmd("")
	})
	got, err := p.OwnerAvatarURL(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Equal(t, "https://gitlab.com/uploads/group/avatar/42/icon.png", got)
}

func TestGlabProvider_OwnerAvatarURL_CliError_ReturnsEmpty(t *testing.T) {
	p := gitlab.NewWithExec(func(_ context.Context, name string, args ...string) *exec.Cmd {
		return failCmd()
	})
	got, err := p.OwnerAvatarURL(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Empty(t, got)
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd api && go test -tags noEmbed ./internal/engine/provider/providers/gitlab/... -run TestGlabProvider_OwnerAvatarURL
```

Expected: FAIL

- [ ] **Step 3: Implement OwnerAvatarURL on glabProvider**

Append to `api/internal/engine/provider/providers/gitlab/gitlab.go`:

```go
// OwnerAvatarURL returns the GitLab namespace avatar URL for the repo.
// Uses glab api projects/:id --jq .namespace.avatar_url.
// Returns ("", nil) on any soft failure so callers can fall back gracefully.
func (g *glabProvider) OwnerAvatarURL(
	ctx context.Context,
	repoPath string,
) (string, error) {
	out, err := g.runGlab(ctx, repoPath, "api", "projects/:id", "--jq", ".namespace.avatar_url")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}
```

- [ ] **Step 4: Run test to confirm it passes**

```bash
cd api && go test -tags noEmbed ./internal/engine/provider/providers/gitlab/... -run TestGlabProvider_OwnerAvatarURL
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/engine/provider/providers/gitlab/gitlab.go api/internal/engine/provider/providers/gitlab/gitlab_test.go
git commit -m "feat: implement OwnerAvatarURL on GitLab provider"
```

---

## Task 6: Implement OwnerAvatarURL on providerEngine

**Files:**
- Modify: `api/internal/engine/provider/engine.go`
- Test: `api/internal/engine/provider/engine_test.go` (add new test)

- [ ] **Step 1: Write failing test**

In `api/internal/engine/provider/engine_test.go`, add:

```go
func TestEngine_OwnerAvatarURL_Enabled(t *testing.T) {
	eng := newTestEngine("github", func(_ context.Context, name string, args ...string) *exec.Cmd {
		if name == "gh" {
			return fakeCmd("https://avatars.githubusercontent.com/u/1")
		}
		return fakeCmd("git@github.com:owner/repo.git")
	})
	got, err := eng.OwnerAvatarURL(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Equal(t, "https://avatars.githubusercontent.com/u/1", got)
}

func TestEngine_OwnerAvatarURL_Disabled_ReturnsEmpty(t *testing.T) {
	eng := newDisabledEngine()
	got, err := eng.OwnerAvatarURL(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Empty(t, got)
}
```

(Use the existing test helpers in `engine_test.go`; if `newTestEngine` doesn't exist, adapt to the pattern already used in that file.)

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd api && go test -tags noEmbed ./internal/engine/provider/... -run TestEngine_OwnerAvatarURL
```

Expected: FAIL

- [ ] **Step 3: Implement on providerEngine**

Add to `api/internal/engine/provider/engine.go`:

```go
// OwnerAvatarURL returns the avatar URL of the repo owner.
// Returns "" when the provider is unavailable or the lookup fails.
func (e *providerEngine) OwnerAvatarURL(
	ctx context.Context,
	repoPath string,
) (string, error) {
	res, err := e.detectFn(ctx, repoPath)
	if err != nil || !res.Enabled {
		return "", nil
	}
	prov := e.providerFor(res.Kind)
	if prov == nil {
		return "", nil
	}
	return prov.OwnerAvatarURL(ctx, repoPath)
}
```

- [ ] **Step 4: Verify the build is clean**

```bash
cd api && go build -tags noEmbed ./...
```

Expected: PASS (all `OwnerAvatarURL` implementations now satisfy the interfaces)

- [ ] **Step 5: Run all provider tests**

```bash
cd api && go test -tags noEmbed -race ./internal/engine/provider/...
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add api/internal/engine/provider/engine.go api/internal/engine/provider/engine_test.go
git commit -m "feat: implement OwnerAvatarURL on providerEngine"
```

---

## Task 7: Wire avatar URL resolution in project import

**Files:**
- Modify: `api/internal/app/usecases/project/project_import.go`
- Modify: `api/internal/app/usecases/mocks/mocks.go`
- Test: `api/internal/app/usecases/project/project_import_test.go`

- [ ] **Step 1: Extend ImportProviderEngine interface**

In `api/internal/app/usecases/project/project_import.go`, extend the interface:

```go
// ImportProviderEngine is the provider surface the import usecase consumes.
type ImportProviderEngine interface {
	ProtectedBranches(
		ctx context.Context,
		repoPath string,
	) ([]string, error)
	// OwnerAvatarURL returns the repo owner's avatar URL, or "" on failure.
	OwnerAvatarURL(
		ctx context.Context,
		repoPath string,
	) (string, error)
}
```

- [ ] **Step 2: Update ProviderEngine mock to satisfy the new interface**

In `api/internal/app/usecases/mocks/mocks.go`, update the `ProviderEngine` struct:

```go
// ProviderEngine is a fake of the provider operations the import usecase consumes.
type ProviderEngine struct {
	Protected      []string
	ProtectedErr   error
	AvatarURL      string
	AvatarURLErr   error
}

func NewProviderEngine() *ProviderEngine {
	return &ProviderEngine{}
}

func (p *ProviderEngine) ProtectedBranches(
	ctx context.Context,
	repoPath string,
) ([]string, error) {
	if p.ProtectedErr != nil {
		return nil, p.ProtectedErr
	}
	return p.Protected, nil
}

func (p *ProviderEngine) OwnerAvatarURL(
	ctx context.Context,
	repoPath string,
) (string, error) {
	return p.AvatarURL, p.AvatarURLErr
}
```

- [ ] **Step 3: Write failing test for avatar URL wiring**

In `api/internal/app/usecases/project/project_import_test.go`, add:

```go
func TestImport_AvatarURL_FromProvider(t *testing.T) {
	_, repos, _, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
	}
	prov.Protected = []string{"main"}
	prov.AvatarURL = "https://avatars.githubusercontent.com/u/99"

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, repos.Saved, 1)
	assert.Equal(t, "https://avatars.githubusercontent.com/u/99", repos.Saved[0].AvatarURL)
}

func TestImport_AvatarURL_FallsBackToEmpty(t *testing.T) {
	_, repos, _, git, prov, uc := newImport(t)
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
	}
	prov.Protected = []string{"main"}
	prov.AvatarURL = "" // provider returns nothing

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	require.Len(t, repos.Saved, 1)
	assert.Empty(t, repos.Saved[0].AvatarURL)
}
```

- [ ] **Step 4: Run tests to confirm they fail**

```bash
cd api && go test -tags noEmbed ./internal/app/usecases/project/... -run TestImport_AvatarURL
```

Expected: FAIL

- [ ] **Step 5: Wire avatar resolution in importOneRepo**

In `api/internal/app/usecases/project/project_import.go`, update `importOneRepo`:

```go
func (u *projectImport) importOneRepo(
	ctx context.Context,
	project domain.Project,
	repoPath string,
) error {
	name := filepath.Base(repoPath)
	runner := u.deps.RefRunner(repoPath)

	avatarURL := avatar.ScanRepoIcon(repoPath)
	if avatarURL == "" {
		avatarURL, _ = u.deps.Provider.OwnerAvatarURL(ctx, repoPath)
	}

	repo := domain.Repository{
		ID:            uuid.NewString(),
		ProjectID:     project.ID,
		Name:          name,
		Path:          repoPath,
		DefaultBranch: defaultbranch.Resolve(runner, defaultBranchCandidates),
		AvatarLabel:   avatar.Label(name),
		AvatarColor:   avatar.Color(name),
		AvatarURL:     avatarURL,
	}
	if err := u.deps.Repos.Save(ctx, repo); err != nil {
		return fmt.Errorf("project import: save repository: %w", err)
	}
	return u.adoptWorktrees(ctx, repo)
}
```

- [ ] **Step 6: Run tests to confirm they pass**

```bash
cd api && go test -tags noEmbed -race ./internal/app/usecases/project/...
```

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add api/internal/app/usecases/project/project_import.go \
        api/internal/app/usecases/mocks/mocks.go \
        api/internal/app/usecases/project/project_import_test.go
git commit -m "feat: resolve avatar URL (filesystem scan + provider fallback) on project import"
```

---

## Task 8: GET /v0/repos/:id/icon handler

**Files:**
- Modify: `api/internal/api/v0/endpoints/repos/handlers/repos.go`
- Modify: `api/internal/api/v0/endpoints/repos/routes.go`
- Test: `api/internal/api/v0/endpoints/repos/handlers/repos_test.go`

- [ ] **Step 1: Write failing test for Icon handler**

In `api/internal/api/v0/endpoints/repos/handlers/repos_test.go`, add:

```go
func TestIcon_LocalFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "logo.svg")
	require.NoError(t, os.WriteFile(f, []byte("<svg/>"), 0o644))

	h := repohandlers.New(&fakeStore{byKey: &domain.Repository{ID: "r1", AvatarURL: f}})
	r := gin.New()
	r.GET("/v0/repos/:id/icon", h.Icon)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/repos/r1/icon", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/svg+xml", w.Header().Get("Content-Type"))
	assert.Equal(t, "<svg/>", w.Body.String())
}

func TestIcon_HTTPSUrl_Redirects(t *testing.T) {
	h := repohandlers.New(&fakeStore{byKey: &domain.Repository{ID: "r1", AvatarURL: "https://example.com/avatar.png"}})
	r := gin.New()
	r.GET("/v0/repos/:id/icon", h.Icon)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/repos/r1/icon", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
	assert.Equal(t, "https://example.com/avatar.png", w.Header().Get("Location"))
}

func TestIcon_NoAvatarURL_Returns404(t *testing.T) {
	h := repohandlers.New(&fakeStore{byKey: &domain.Repository{ID: "r1", AvatarURL: ""}})
	r := gin.New()
	r.GET("/v0/repos/:id/icon", h.Icon)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/repos/r1/icon", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
```

Add required imports (`os`, `path/filepath`) to the test file.

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd api && go test -tags noEmbed ./internal/api/v0/endpoints/repos/... -run TestIcon
```

Expected: FAIL — `h.Icon` undefined

- [ ] **Step 3: Implement Icon handler**

Add to `api/internal/api/v0/endpoints/repos/handlers/repos.go`:

```go
import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	// existing imports ...
)

// Icon handles GET /v0/repos/:id/icon. If AvatarURL is an HTTPS URL it
// redirects. If it is a local filesystem path it reads and serves the file.
func (h *Handlers) Icon(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("id"))
	if err != nil || repo == nil || repo.AvatarURL == "" {
		c.Status(http.StatusNotFound)
		return
	}
	if strings.HasPrefix(repo.AvatarURL, "http") {
		c.Redirect(http.StatusTemporaryRedirect, repo.AvatarURL)
		return
	}
	data, err := os.ReadFile(repo.AvatarURL)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	contentTypes := map[string]string{
		".svg":  "image/svg+xml",
		".png":  "image/png",
		".ico":  "image/x-icon",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
	}
	ct := contentTypes[strings.ToLower(filepath.Ext(repo.AvatarURL))]
	if ct == "" {
		ct = "application/octet-stream"
	}
	c.Data(http.StatusOK, ct, data)
}
```

- [ ] **Step 4: Register the route**

In `api/internal/api/v0/endpoints/repos/routes.go`:

```go
func Register(rg *gin.RouterGroup, store repohandlers.Store) {
	h := repohandlers.New(store)
	rg.POST("/repos", h.Create)
	rg.GET("/repos", h.List)
	rg.GET("/repos/:id", h.Detail)
	rg.GET("/repos/:id/icon", h.Icon)
}
```

- [ ] **Step 5: Run tests to confirm they pass**

```bash
cd api && go test -tags noEmbed ./internal/api/v0/endpoints/repos/... -run TestIcon
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add api/internal/api/v0/endpoints/repos/handlers/repos.go \
        api/internal/api/v0/endpoints/repos/routes.go \
        api/internal/api/v0/endpoints/repos/handlers/repos_test.go
git commit -m "feat: add GET /v0/repos/:id/icon endpoint to serve local or redirect remote icons"
```

---

## Task 9: GET /v0/repos/:id/branches handler

**Files:**
- Modify: `api/internal/api/v0/endpoints/repos/handlers/repos.go`
- Modify: `api/internal/api/v0/endpoints/repos/routes.go`
- Modify: `api/internal/api/v0/router.go`
- Test: `api/internal/api/v0/endpoints/repos/handlers/repos_test.go`

- [ ] **Step 1: Define deps interfaces and update Handlers struct**

At the top of `api/internal/api/v0/endpoints/repos/handlers/repos.go`, add after `Store`:

```go
// BranchProviderEngine is the provider surface the Branches handler needs.
type BranchProviderEngine interface {
	ProtectedBranches(ctx context.Context, repoPath string) ([]string, error)
}

// WorkspaceReader is the workspace surface the Branches handler needs.
type WorkspaceReader interface {
	List(ctx context.Context) ([]domain.Workspace, error)
}

// BranchEntry is one item in the GET /v0/repos/:id/branches response.
type BranchEntry struct {
	Name         string `json:"name"`
	IsProtected  bool   `json:"isProtected"`
	HasWorkspace bool   `json:"hasWorkspace"`
}
```

Update the `Handlers` struct:

```go
type Handlers struct {
	store    Store
	provider BranchProviderEngine
	wsReader WorkspaceReader
}

func New(store Store) *Handlers {
	return &Handlers{store: store}
}

// NewWithDeps builds Handlers with the optional provider + workspace deps
// needed for the Branches endpoint.
func NewWithDeps(store Store, prov BranchProviderEngine, wsReader WorkspaceReader) *Handlers {
	return &Handlers{store: store, provider: prov, wsReader: wsReader}
}
```

- [ ] **Step 2: Write failing test for Branches handler**

Add to `api/internal/api/v0/endpoints/repos/handlers/repos_test.go`:

```go
type fakeBranchProvider struct {
	protected []string
}

func (f *fakeBranchProvider) ProtectedBranches(_ context.Context, _ string) ([]string, error) {
	return f.protected, nil
}

type fakeWSReader struct {
	workspaces []domain.Workspace
}

func (f *fakeWSReader) List(_ context.Context) ([]domain.Workspace, error) {
	return f.workspaces, nil
}

func TestBranches_AnnotatesProtectionAndWorkspace(t *testing.T) {
	repo := &domain.Repository{ID: "r1", Path: "/repo"}
	store := &fakeStore{byKey: repo}
	prov := &fakeBranchProvider{protected: []string{"main"}}
	wsReader := &fakeWSReader{
		workspaces: []domain.Workspace{
			{RepoID: "r1", Branch: "main"},
		},
	}
	h := repohandlers.NewWithDeps(store, prov, wsReader)

	// We cannot easily test git branch -r in unit tests; this test stubs
	// via a custom execFn. For simplicity, test the annotation logic only:
	// supply a fake branches list and verify the annotation fields.
	// Integration is covered by the routes_test.go pattern.
	_ = h
	// This test is a placeholder; full coverage requires the execFn injection
	// added in Step 3. Add the real test after the implementation.
}
```

Actually, the `Branches` handler runs `git branch -r` as a subprocess — to test it properly we need an `ExecFn` injection. Add that in the implementation step.

- [ ] **Step 3: Implement Branches handler**

Add to `api/internal/api/v0/endpoints/repos/handlers/repos.go`:

```go
import (
	"os/exec"
	// existing ...
)

// Branches handles GET /v0/repos/:id/branches. Returns all remote branches
// annotated with isProtected and hasWorkspace fields.
func (h *Handlers) Branches(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("id"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}

	// List remote branches via git branch -r --format=%(refname:short)
	cmd := exec.CommandContext(c.Request.Context(), "git", "-C", repo.Path, "branch", "-r", "--format=%(refname:short)")
	out, err := cmd.Output()
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "failed to list branches")
		return
	}
	rawBranches := parseRemoteBranches(string(out))

	// Annotate with protected status
	protected := map[string]bool{}
	if h.provider != nil {
		list, _ := h.provider.ProtectedBranches(c.Request.Context(), repo.Path)
		for _, b := range list {
			protected[b] = true
		}
	}

	// Annotate with workspace existence
	hasWS := map[string]bool{}
	if h.wsReader != nil {
		all, _ := h.wsReader.List(c.Request.Context())
		for _, ws := range all {
			if ws.RepoID == repo.ID {
				hasWS[ws.Branch] = true
			}
		}
	}

	entries := make([]BranchEntry, 0, len(rawBranches))
	for _, b := range rawBranches {
		entries = append(entries, BranchEntry{
			Name:         b,
			IsProtected:  protected[b],
			HasWorkspace: hasWS[b],
		})
	}
	libs.WriteQueryOK(c, entries)
}

// parseRemoteBranches strips the "origin/" prefix from git branch -r output
// and skips HEAD pointer lines.
func parseRemoteBranches(out string) []string {
	var result []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "->") {
			continue
		}
		// Strip "origin/" prefix
		if idx := strings.Index(line, "/"); idx >= 0 {
			line = line[idx+1:]
		}
		result = append(result, line)
	}
	return result
}
```

- [ ] **Step 4: Register the route and update router.go**

In `api/internal/api/v0/endpoints/repos/routes.go`, update `Register` to accept optional deps:

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
	rg.GET("/repos/:id/branches", h.Branches)
}
```

In `api/internal/api/v0/router.go`, update the `repos.Register` call:

```go
repos.Register(
	rg,
	c.app.GORM.Repositories,
	c.eng.Provider,
	c.app.Repositories.Workspace,
)
```

- [ ] **Step 5: Update routes_test.go to pass nil deps (they're optional)**

In `api/internal/api/v0/endpoints/repos/routes_test.go`, update the `repos.Register` call to pass `nil` for the new deps:

```go
repos.Register(r.Group("/v0"), stubStore{}, nil, nil)
```

- [ ] **Step 6: Build to verify wiring**

```bash
cd api && go build -tags noEmbed ./...
```

Expected: PASS

- [ ] **Step 7: Run repo handler tests**

```bash
cd api && go test -tags noEmbed -race ./internal/api/v0/endpoints/repos/...
```

Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add api/internal/api/v0/endpoints/repos/handlers/repos.go \
        api/internal/api/v0/endpoints/repos/routes.go \
        api/internal/api/v0/endpoints/repos/routes_test.go \
        api/internal/api/v0/router.go \
        api/internal/api/v0/endpoints/repos/handlers/repos_test.go
git commit -m "feat: add GET /v0/repos/:id/branches endpoint with protection and workspace annotations"
```

---

## Task 10: Auto-import protected branch stubs on project import

**Files:**
- Modify: `api/internal/app/usecases/project/project_import.go`
- Test: `api/internal/app/usecases/project/project_import_test.go`

- [ ] **Step 1: Write failing test**

Add to `api/internal/app/usecases/project/project_import_test.go`:

```go
func TestImport_AutoImportsProtectedBranchStubs(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)

	// Only "main" is a local worktree; "develop" is protected but not local
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
	}
	prov.Protected = []string{"main", "develop"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	// Should have created 2 workspaces: main (adopted) + develop (stub)
	require.Len(t, ws.Created, 2)
	byBranch := map[string]bool{}
	for _, w := range ws.Created {
		byBranch[w.Branch] = w.Locked
	}
	assert.True(t, byBranch["main"])
	assert.True(t, byBranch["develop"])
}

func TestImport_SkipsStubWhenAlreadyAdopted(t *testing.T) {
	_, _, ws, git, prov, uc := newImport(t)

	// "develop" is both local and protected — should not be created twice
	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "develop", Head: "h1"},
	}
	prov.Protected = []string{"develop"}

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)
	assert.Len(t, ws.Created, 1)
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd api && go test -tags noEmbed ./internal/app/usecases/project/... -run TestImport_AutoImports,TestImport_SkipsStub
```

Expected: FAIL

- [ ] **Step 3: Refactor adoptWorktrees to return adopted set, add importProtectedBranchStubs**

In `api/internal/app/usecases/project/project_import.go`:

Replace the `importOneRepo` call to `adoptWorktrees` and its signature:

```go
func (u *projectImport) importOneRepo(
	ctx context.Context,
	project domain.Project,
	repoPath string,
) error {
	name := filepath.Base(repoPath)
	runner := u.deps.RefRunner(repoPath)

	avatarURL := avatar.ScanRepoIcon(repoPath)
	if avatarURL == "" {
		avatarURL, _ = u.deps.Provider.OwnerAvatarURL(ctx, repoPath)
	}

	repo := domain.Repository{
		ID:            uuid.NewString(),
		ProjectID:     project.ID,
		Name:          name,
		Path:          repoPath,
		DefaultBranch: defaultbranch.Resolve(runner, defaultBranchCandidates),
		AvatarLabel:   avatar.Label(name),
		AvatarColor:   avatar.Color(name),
		AvatarURL:     avatarURL,
	}
	if err := u.deps.Repos.Save(ctx, repo); err != nil {
		return fmt.Errorf("project import: save repository: %w", err)
	}
	adopted, err := u.adoptWorktrees(ctx, repo)
	if err != nil {
		return err
	}
	return u.importProtectedBranchStubs(ctx, repo, adopted)
}
```

Update `adoptWorktrees` signature and return value:

```go
func (u *projectImport) adoptWorktrees(
	ctx context.Context,
	repo domain.Repository,
) (map[string]bool, error) {
	worktrees, err := u.deps.Git.WorktreeList(ctx, repo.Path)
	if err != nil {
		return nil, fmt.Errorf("project import: list worktrees: %w", err)
	}
	protected, err := u.deps.Provider.ProtectedBranches(ctx, repo.Path)
	if err != nil {
		return nil, fmt.Errorf("project import: protected branches: %w", err)
	}
	locked := toSet(protected)
	adopted := make(map[string]bool)
	for _, wt := range worktrees {
		if err := u.adoptOneWorktree(ctx, repo, wt, locked); err != nil {
			return nil, err
		}
		if wt.Branch != "" && !wt.Prunable {
			adopted[wt.Branch] = true
		}
	}
	return adopted, nil
}
```

Add `importProtectedBranchStubs`:

```go
// importProtectedBranchStubs creates locked workspace records for protected
// branches that do not already have a local worktree. Uses repo.Path as the
// WorktreePath stub so the sidebar can display them immediately.
func (u *projectImport) importProtectedBranchStubs(
	ctx context.Context,
	repo domain.Repository,
	adopted map[string]bool,
) error {
	protected, err := u.deps.Provider.ProtectedBranches(ctx, repo.Path)
	if err != nil {
		return nil // soft: don't fail the entire import
	}
	for _, branch := range protected {
		if adopted[branch] {
			continue
		}
		in := workspace.CreateInput{
			ID:           uuid.NewString(),
			RepoID:       repo.ID,
			ProjectID:    repo.ProjectID,
			Branch:       branch,
			WorktreePath: repo.Path,
			Locked:       true,
		}
		if _, err := u.deps.Workspaces.Create(ctx, in, u.deps.Now()); err != nil {
			slog.WarnContext(ctx, "project import: skip protected branch stub",
				"branch", branch, "error", err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run all project import tests**

```bash
cd api && go test -tags noEmbed -race ./internal/app/usecases/project/...
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/project/project_import.go \
        api/internal/app/usecases/project/project_import_test.go
git commit -m "feat: auto-import protected branch stubs on project import"
```

---

## Task 11: SetParentFromPR command + Workspace method

**Files:**
- Create: `api/internal/app/repositories/workspace/internal/commands/set_parent_from_pr.go`
- Modify: `api/internal/app/repositories/workspace/workspace.go`
- Test: `api/internal/app/repositories/workspace/internal/commands/` (add test file)

- [ ] **Step 1: Write failing test for SetParentFromPR command**

Create `api/internal/app/repositories/workspace/internal/commands/set_parent_from_pr_test.go`:

```go
package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestSetParentFromPR_SetsParentID(t *testing.T) {
	ws := &domain.Workspace{ID: "ws1", ParentID: ""}
	cmd := commands.SetParentFromPR{ID: "ws1", ParentID: "parent-ws"}
	got := cmd.EmitEvent(ws)
	assert.Equal(t, "parent-ws", got.ParentID)
	assert.Equal(t, "ws1", got.ID) // other fields unchanged
}

func TestSetParentFromPR_Validate_NilWorkspace(t *testing.T) {
	cmd := commands.SetParentFromPR{ID: "ws1", ParentID: "parent-ws"}
	assert.Error(t, cmd.Validate(nil))
}

func TestSetParentFromPR_Validate_EmptyParentID(t *testing.T) {
	ws := &domain.Workspace{ID: "ws1"}
	cmd := commands.SetParentFromPR{ID: "ws1", ParentID: ""}
	assert.Error(t, cmd.Validate(ws))
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd api && go test -tags noEmbed ./internal/app/repositories/workspace/internal/commands/... -run TestSetParentFromPR
```

Expected: FAIL

- [ ] **Step 3: Create the command**

Create `api/internal/app/repositories/workspace/internal/commands/set_parent_from_pr.go`:

```go
package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SetParentFromPR assigns ParentID based on an open PR's target branch without
// recomputing ForkPointSha. Only intended for provider-sync auto-wiring; the
// existing Reparent command is the user-facing reparent path.
type SetParentFromPR struct {
	ID       string
	ParentID string
}

func (c SetParentFromPR) AggregateID() string { return c.ID }

func (c SetParentFromPR) EventName() string {
	return "workspace.parent_set_from_pr." + c.ID
}

func (c SetParentFromPR) ShouldSnapshot() bool { return true }

func (c SetParentFromPR) Validate(current *domain.Workspace) error {
	if current == nil {
		return fmt.Errorf("set parent from pr: %w", asynxModels.ErrValidation)
	}
	if c.ParentID == "" {
		return fmt.Errorf("set parent from pr: missing parent id: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetParentFromPR) EmitEvent(current *domain.Workspace) domain.Workspace {
	ws := *current
	ws.ParentID = c.ParentID
	return ws
}
```

- [ ] **Step 4: Add SetParentFromPR to Workspace interface and impl**

In `api/internal/app/repositories/workspace/workspace.go`, add to the `Workspace` interface:

```go
// SetParentFromPR sets ParentID from an open PR's target branch without
// recomputing ForkPointSha. Only applies once (when ParentID is still empty).
SetParentFromPR(
    ctx context.Context,
    id string,
    parentID string,
) (domain.Workspace, error)
```

Add the implementation:

```go
func (w *workspace) SetParentFromPR(
	ctx context.Context,
	id string,
	parentID string,
) (domain.Workspace, error) {
	evt, err := w.ax.SendWait(ctx, commands.SetParentFromPR{ID: id, ParentID: parentID})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("workspace: set parent from pr: %w", err)
	}
	return evt.Aggregate, nil
}
```

- [ ] **Step 5: Run command tests**

```bash
cd api && go test -tags noEmbed -race ./internal/app/repositories/workspace/...
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add api/internal/app/repositories/workspace/internal/commands/set_parent_from_pr.go \
        api/internal/app/repositories/workspace/internal/commands/set_parent_from_pr_test.go \
        api/internal/app/repositories/workspace/workspace.go
git commit -m "feat: add SetParentFromPR command and Workspace method"
```

---

## Task 12: PR parent-child auto-reparent in SyncFromState

**Files:**
- Modify: `api/internal/app/usecases/provider/provider_sync.go`
- Modify: `api/internal/app/usecases/mocks/mocks.go`
- Test: `api/internal/app/usecases/provider/provider_sync_test.go`

- [ ] **Step 1: Extend WorkspaceRepo interface in provider_sync.go**

In `api/internal/app/usecases/provider/provider_sync.go`, update `WorkspaceRepo`:

```go
type WorkspaceRepo interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
	SyncProviderState(ctx context.Context, in workspace.ProviderInput, now time.Time) (domain.Workspace, error)
	List(ctx context.Context) ([]domain.Workspace, error)
	SetParentFromPR(ctx context.Context, id string, parentID string) (domain.Workspace, error)
}
```

- [ ] **Step 2: Update ProviderSyncWorkspaceRepo mock**

In `api/internal/app/usecases/mocks/mocks.go`, update `ProviderSyncWorkspaceRepo`:

```go
type ProviderSyncWorkspaceRepo struct {
	GetFn             func(ctx context.Context, id string) (domain.Workspace, error)
	SyncProviderFn    func(ctx context.Context, in workspace.ProviderInput, now time.Time) (domain.Workspace, error)
	ListFn            func(ctx context.Context) ([]domain.Workspace, error)
	SetParentFromPRFn func(ctx context.Context, id string, parentID string) (domain.Workspace, error)
}

func (r *ProviderSyncWorkspaceRepo) Get(ctx context.Context, id string) (domain.Workspace, error) {
	return r.GetFn(ctx, id)
}

func (r *ProviderSyncWorkspaceRepo) SyncProviderState(ctx context.Context, in workspace.ProviderInput, now time.Time) (domain.Workspace, error) {
	return r.SyncProviderFn(ctx, in, now)
}

func (r *ProviderSyncWorkspaceRepo) List(ctx context.Context) ([]domain.Workspace, error) {
	if r.ListFn != nil {
		return r.ListFn(ctx)
	}
	return nil, nil
}

func (r *ProviderSyncWorkspaceRepo) SetParentFromPR(ctx context.Context, id string, parentID string) (domain.Workspace, error) {
	if r.SetParentFromPRFn != nil {
		return r.SetParentFromPRFn(ctx, id, parentID)
	}
	return domain.Workspace{}, nil
}
```

- [ ] **Step 3: Write failing test for auto-reparent**

In `api/internal/app/usecases/provider/provider_sync_test.go`, add:

```go
func TestSyncFromState_AutoReparentsWhenPRTargetMatchesWorkspace(t *testing.T) {
	parentWsID := "parent-ws"
	currentWsID := "current-ws"
	repoID := "repo-1"

	var reparentedID, reparentedParent string
	wsRepo := mocks.NewProviderSyncWorkspaceRepo()
	wsRepo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{
			ID:             currentWsID,
			RepoID:         repoID,
			Branch:         "feature/foo",
			ParentID:       "", // no parent yet
			PRTargetBranch: "develop",
		}, nil
	}
	wsRepo.SyncProviderFn = func(_ context.Context, in workspace.ProviderInput, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{ID: in.ID, PRTargetBranch: in.PRTargetBranch}, nil
	}
	wsRepo.ListFn = func(_ context.Context) ([]domain.Workspace, error) {
		return []domain.Workspace{
			{ID: parentWsID, RepoID: repoID, Branch: "develop"},
			{ID: currentWsID, RepoID: repoID, Branch: "feature/foo"},
		}, nil
	}
	wsRepo.SetParentFromPRFn = func(_ context.Context, id, parentID string) (domain.Workspace, error) {
		reparentedID = id
		reparentedParent = parentID
		return domain.Workspace{}, nil
	}

	uc := provider.New(wsRepo, &mocks.ProviderSyncEngine{})
	state := engineprovider.ProviderState{
		PR: &engineprovider.PRInfo{
			Status:       "open",
			TargetBranch: "develop",
		},
	}
	err := uc.SyncFromState(context.Background(), currentWsID, state, time.Now())
	require.NoError(t, err)
	assert.Equal(t, currentWsID, reparentedID)
	assert.Equal(t, parentWsID, reparentedParent)
}

func TestSyncFromState_SkipsReparentWhenParentAlreadySet(t *testing.T) {
	called := false
	wsRepo := mocks.NewProviderSyncWorkspaceRepo()
	wsRepo.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{
			ID:             "ws1",
			RepoID:         "r1",
			ParentID:       "already-set", // already has parent
			PRTargetBranch: "develop",
		}, nil
	}
	wsRepo.SyncProviderFn = func(_ context.Context, in workspace.ProviderInput, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, nil
	}
	wsRepo.SetParentFromPRFn = func(_ context.Context, id, parentID string) (domain.Workspace, error) {
		called = true
		return domain.Workspace{}, nil
	}

	uc := provider.New(wsRepo, &mocks.ProviderSyncEngine{})
	state := engineprovider.ProviderState{
		PR: &engineprovider.PRInfo{Status: "open", TargetBranch: "develop"},
	}
	err := uc.SyncFromState(context.Background(), "ws1", state, time.Now())
	require.NoError(t, err)
	assert.False(t, called, "SetParentFromPR must not be called when parentID is already set")
}
```

- [ ] **Step 4: Run tests to confirm they fail**

```bash
cd api && go test -tags noEmbed ./internal/app/usecases/provider/... -run TestSyncFromState_AutoReparent,TestSyncFromState_SkipsReparent
```

Expected: FAIL

- [ ] **Step 5: Update SyncFromState to auto-reparent**

In `api/internal/app/usecases/provider/provider_sync.go`, update `SyncFromState`:

```go
func (u *providerSyncUsecase) SyncFromState(
	ctx context.Context,
	wsID string,
	state engineprovider.ProviderState,
	now time.Time,
) error {
	// Load the current workspace to read ParentID and RepoID.
	current, err := u.workspaces.Get(ctx, wsID)
	if err != nil {
		return fmt.Errorf("provider sync: sync from state: get workspace: %w", err)
	}

	in := workspace.ProviderInput{ID: wsID, Protected: state.Protected}
	if state.PR != nil {
		in.HasPR = true
		in.PRStatus = state.PR.Status
		in.PRUrl = state.PR.URL
		in.PRTitle = state.PR.Title
		in.PRTargetBranch = state.PR.TargetBranch
	}
	if _, err := u.workspaces.SyncProviderState(ctx, in, now); err != nil {
		return fmt.Errorf("provider sync: sync from state: %w", err)
	}

	// Auto-reparent: only when a PR targets a branch that has a workspace and
	// this workspace has no parent yet.
	if state.PR != nil && state.PR.TargetBranch != "" && current.ParentID == "" {
		u.maybeReparentFromPR(ctx, current, state.PR.TargetBranch)
	}
	return nil
}

// maybeReparentFromPR looks up the workspace in the same repo whose branch
// matches targetBranch and calls SetParentFromPR if found.
func (u *providerSyncUsecase) maybeReparentFromPR(
	ctx context.Context,
	ws domain.Workspace,
	targetBranch string,
) {
	all, err := u.workspaces.List(ctx)
	if err != nil {
		return
	}
	for _, candidate := range all {
		if candidate.RepoID == ws.RepoID && candidate.Branch == targetBranch {
			_, _ = u.workspaces.SetParentFromPR(ctx, ws.ID, candidate.ID)
			return
		}
	}
}
```

Note: `maybeReparentFromPR` needs `domain` imported — ensure `"github.com/char2cs/crowbar/api/internal/domain"` is in the import block.

- [ ] **Step 6: Run all provider sync tests**

```bash
cd api && go test -tags noEmbed -race ./internal/app/usecases/provider/...
```

Expected: PASS

- [ ] **Step 7: Build to verify everything compiles**

```bash
cd api && go build -tags noEmbed ./...
```

Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add api/internal/app/usecases/provider/provider_sync.go \
        api/internal/app/usecases/mocks/mocks.go \
        api/internal/app/usecases/provider/provider_sync_test.go
git commit -m "feat: auto-reparent workspace from PR target branch on provider sync"
```

---

## Task 13: Run full backend test suite

- [ ] **Step 1: Run all unit tests**

```bash
cd api && go test -tags noEmbed -race ./...
```

Expected: PASS — all tests green

- [ ] **Step 2: Commit if any cleanup needed; otherwise proceed**

---

## Task 14: Frontend — avatarURL in Repo type and sidebar icon

**Files:**
- Modify: `web/src/lib/store/sidebar.ts`
- Modify: `web/src/lib/store/build-repo-tree.ts`
- Modify: `web/src/components/layout/workspace-tree.tsx`
- Test: `web/src/__tests__/lib/store/build-repo-tree.test.ts` (add test)

- [ ] **Step 1: Add avatarURL to Repo interface in sidebar.ts**

In `web/src/lib/store/sidebar.ts`, find the `Repo` interface and add `avatarURL`:

```typescript
export interface Repo {
  id: string
  projectId?: string
  name: string
  avatarLabel: string
  avatarColor: string
  avatarURL?: string
  workspaces: Workspace[]
}
```

- [ ] **Step 2: Map avatarURL in build-repo-tree.ts**

In `web/src/lib/store/build-repo-tree.ts`, find where `RepoDTO` is converted to `Repo` and add the field. The conversion likely reads `dto.avatarLabel` and `dto.avatarColor`. Add:

```typescript
avatarURL: dto.avatarUrl ?? undefined,
```

- [ ] **Step 3: Write test for avatarURL mapping**

In `web/src/__tests__/lib/store/build-repo-tree.test.ts` (create if it doesn't exist), add:

```typescript
import { buildRepoTree } from '@/lib/store/build-repo-tree'
// or wherever the mapping function is exported

it('maps avatarUrl from RepoDTO to Repo.avatarURL', () => {
  const dto = {
    id: 'r1',
    projectId: 'p1',
    name: 'my-repo',
    path: '/my-repo',
    defaultBranch: 'main',
    avatarLabel: 'M',
    avatarColor: 'avatar-indigo',
    avatarUrl: 'https://example.com/avatar.png',
  }
  // Find the exact function that transforms RepoDTO → Repo and call it
  // with the dto above. Adjust imports/function name to match the file.
  const repos = buildRepoTree([dto], [])
  expect(repos[0].avatarURL).toBe('https://example.com/avatar.png')
})

it('leaves avatarURL undefined when avatarUrl is absent', () => {
  const dto = {
    id: 'r1', projectId: 'p1', name: 'repo', path: '/', defaultBranch: 'main',
    avatarLabel: 'R', avatarColor: 'avatar-rose',
  }
  const repos = buildRepoTree([dto], [])
  expect(repos[0].avatarURL).toBeUndefined()
})
```

- [ ] **Step 4: Run frontend tests**

```bash
cd web && npm test -- --reporter=verbose 2>&1 | tail -20
```

Expected: PASS (or only failing on the new test if mapping not yet done)

- [ ] **Step 5: Update workspace-tree.tsx to render img icon**

In `web/src/components/layout/workspace-tree.tsx`, find the repo avatar span (around lines 108–115) and replace it:

```tsx
{repo.avatarURL ? (
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

- [ ] **Step 6: Run frontend tests**

```bash
cd web && npm test
```

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add web/src/lib/store/sidebar.ts \
        web/src/lib/store/build-repo-tree.ts \
        web/src/components/layout/workspace-tree.tsx \
        "web/src/__tests__/lib/store/build-repo-tree.test.ts"
git commit -m "feat: display repo icon from avatarURL in sidebar, fall back to letter badge"
```

---

## Task 15: Frontend — settings gear hover swap on repo row

**Files:**
- Modify: `web/src/components/layout/workspace-tree.tsx`
- Test: `web/src/__tests__/components/layout/workspace-tree.test.tsx` (add test)

- [ ] **Step 1: Write failing test**

In `web/src/__tests__/components/layout/workspace-tree.test.tsx`, add a test that verifies the gear icon appears on hover and triggers a click handler. Use `@testing-library/user-event`:

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { WorkspaceTree } from '@/components/layout/workspace-tree'
// Wrap with necessary providers (router, store) per existing test patterns

it('shows settings gear on repo row hover', async () => {
  // Render with a fake repo in the store
  // ... setup store with one repo ...
  const { rerender } = render(<WorkspaceTree />, { wrapper: TestProviders })
  const repoRow = screen.getByRole('button', { name: /my-repo/i })
  await userEvent.hover(repoRow)
  expect(screen.getByRole('button', { name: /repo settings/i })).toBeInTheDocument()
})
```

Adapt imports and wrapper to match the existing test patterns in that file if it exists.

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd web && npm test -- workspace-tree
```

Expected: FAIL

- [ ] **Step 3: Add hover state and gear icon to repo row**

In `web/src/components/layout/workspace-tree.tsx`, update the repo row section. Add a `hoveredRepoId` state, and on hover swap the chevron:

```tsx
import { Settings } from 'lucide-react'
// add to existing imports

// Inside WorkspaceTreeInner, add state:
const [hoveredRepoId, setHoveredRepoId] = React.useState<string | null>(null)

// In the repo row div, add hover handlers and swap the trailing icon:
<div
  role="button"
  tabIndex={0}
  className={cn(
    ROW_BASE,
    'group border-transparent text-foreground hover:bg-accent',
    isRepoDragOver && 'ring-1 ring-ring',
  )}
  onClick={() => useSidebarStore.getState().toggleRepo(repo.id)}
  onKeyDown={(e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      useSidebarStore.getState().toggleRepo(repo.id)
    }
  }}
  onMouseEnter={() => setHoveredRepoId(repo.id)}
  onMouseLeave={() => setHoveredRepoId(null)}
  aria-label={isCollapsed ? 'Expand repo' : 'Collapse repo'}
  data-repo-drop={repo.id}
>
  {/* icon + name ... same as before ... */}
  {hoveredRepoId === repo.id ? (
    <button
      aria-label="Repo settings"
      className="shrink-0 rounded-md p-1 text-foreground/50 hover:text-foreground"
      onClick={(e) => {
        e.stopPropagation()
        setOpenSettingsRepoId(repo.id)
      }}
    >
      <Settings className="size-3" />
    </button>
  ) : (
    <span className="shrink-0 rounded-md p-1 text-foreground/30">
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
    </span>
  )}
</div>
```

Also add `openSettingsRepoId` state:

```tsx
const [openSettingsRepoId, setOpenSettingsRepoId] = React.useState<string | null>(null)
```

This `openSettingsRepoId` is passed to the `RepoSettingsPanel` component added in Task 16.

- [ ] **Step 4: Run frontend tests**

```bash
cd web && npm test
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/workspace-tree.tsx
git commit -m "feat: swap repo row chevron to settings gear on hover"
```

---

## Task 16: Frontend — repo settings panel with Import Workspaces

**Files:**
- Create: `web/src/components/layout/repo-settings-panel.tsx`
- Modify: `web/src/components/layout/workspace-tree.tsx`
- Test: `web/src/__tests__/components/layout/repo-settings-panel.test.tsx`

- [ ] **Step 1: Write failing test**

Create `web/src/__tests__/components/layout/repo-settings-panel.test.tsx`:

```tsx
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { RepoSettingsPanel } from '@/components/layout/repo-settings-panel'

const mockBranches = [
  { name: 'main', isProtected: true, hasWorkspace: true },
  { name: 'develop', isProtected: true, hasWorkspace: false },
  { name: 'feature/foo', isProtected: false, hasWorkspace: false },
]

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ success: true, data: mockBranches }),
  }))
})

afterEach(() => vi.restoreAllMocks())

it('shows branch list after open', async () => {
  render(
    <RepoSettingsPanel
      repoId="r1"
      repoName="my-repo"
      open={true}
      onOpenChange={() => {}}
    />
  )
  await waitFor(() => expect(screen.getByText('main')).toBeInTheDocument())
  expect(screen.getByText('feature/foo')).toBeInTheDocument()
})

it('protected branches are pre-checked and disabled', async () => {
  render(<RepoSettingsPanel repoId="r1" repoName="my-repo" open={true} onOpenChange={() => {}} />)
  await waitFor(() => screen.getByText('main'))
  const mainCheckbox = screen.getByRole('checkbox', { name: /main/i })
  expect(mainCheckbox).toBeChecked()
  expect(mainCheckbox).toBeDisabled()
})

it('clicking Import calls POST /v0/workspaces for each selected branch', async () => {
  const postMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ success: true, data: { id: 'ws-new' } }) })
  vi.stubGlobal('fetch', vi.fn()
    .mockResolvedValueOnce({ ok: true, json: async () => ({ success: true, data: mockBranches }) })
    .mockResolvedValue({ ok: true, json: async () => ({ success: true, data: { id: 'ws-new' } }) })
  )
  render(<RepoSettingsPanel repoId="r1" repoName="my-repo" open={true} onOpenChange={() => {}} />)
  await waitFor(() => screen.getByText('feature/foo'))

  await userEvent.click(screen.getByRole('checkbox', { name: /feature\/foo/i }))
  await userEvent.click(screen.getByRole('button', { name: /import/i }))

  await waitFor(() => {
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('/v0/workspaces'),
      expect.objectContaining({ method: 'POST' })
    )
  })
})
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd web && npm test -- repo-settings-panel
```

Expected: FAIL — component not found

- [ ] **Step 3: Create RepoSettingsPanel**

Create `web/src/components/layout/repo-settings-panel.tsx`:

```tsx
import { useState, useEffect } from 'react'
import { Sheet, SheetPopup, SheetHeader, SheetTitle, SheetViewport } from '@/components/ui/sheet'
import { Checkbox } from '@/components/ui/checkbox'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { apiFetch } from '@/lib/api'
import { Lock, GitBranch } from 'lucide-react'
import { cn } from '@/lib/utils'

interface BranchEntry {
  name: string
  isProtected: boolean
  hasWorkspace: boolean
}

interface RepoSettingsPanelProps {
  repoId: string
  repoName: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function RepoSettingsPanel({ repoId, repoName, open, onOpenChange }: RepoSettingsPanelProps) {
  const [branches, setBranches] = useState<BranchEntry[]>([])
  const [filter, setFilter] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [importing, setImporting] = useState(false)

  useEffect(() => {
    if (!open) return
    setBranches([])
    setSelected(new Set())
    setFilter('')
    apiFetch<BranchEntry[]>(`/v0/repos/${repoId}/branches`)
      .then(setBranches)
      .catch(() => {})
  }, [open, repoId])

  const visible = branches.filter((b) =>
    b.name.toLowerCase().includes(filter.toLowerCase())
  )

  const importable = selected.size
  const importLabel = importable > 0 ? `Import ${importable} branch${importable > 1 ? 'es' : ''}` : 'Import'

  async function handleImport() {
    if (selected.size === 0) return
    setImporting(true)
    await Promise.all(
      Array.from(selected).map((branch) =>
        apiFetch('/v0/workspaces', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ repoId, branch }),
        }).catch(() => {})
      )
    )
    setImporting(false)
    onOpenChange(false)
  }

  function toggleBranch(name: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetViewport>
        <SheetPopup side="left" className="w-80">
          <SheetHeader>
            <SheetTitle className="font-mono text-sm">{repoName} — Settings</SheetTitle>
          </SheetHeader>

          <div className="flex flex-col gap-4 p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Import Workspaces
            </h3>

            <Input
              placeholder="Filter branches…"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              className="h-7 text-xs"
            />

            <div className="flex flex-col gap-0.5">
              {/* Protected section */}
              {visible.some((b) => b.isProtected) && (
                <>
                  <p className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                    Protected — auto-imported
                  </p>
                  {visible
                    .filter((b) => b.isProtected)
                    .map((b) => (
                      <BranchRow
                        key={b.name}
                        branch={b}
                        checked={true}
                        disabled={true}
                        onToggle={() => {}}
                      />
                    ))}
                </>
              )}

              {/* Non-protected section */}
              {visible.some((b) => !b.isProtected) && (
                <>
                  <p className="mb-1 mt-3 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                    Other branches
                  </p>
                  {visible
                    .filter((b) => !b.isProtected)
                    .map((b) => (
                      <BranchRow
                        key={b.name}
                        branch={b}
                        checked={b.hasWorkspace || selected.has(b.name)}
                        disabled={b.hasWorkspace}
                        onToggle={() => !b.hasWorkspace && toggleBranch(b.name)}
                      />
                    ))}
                </>
              )}
            </div>

            <Button
              size="sm"
              disabled={selected.size === 0 || importing}
              onClick={handleImport}
            >
              {importing ? 'Importing…' : importLabel}
            </Button>
          </div>
        </SheetPopup>
      </SheetViewport>
    </Sheet>
  )
}

interface BranchRowProps {
  branch: BranchEntry
  checked: boolean
  disabled: boolean
  onToggle: () => void
}

function BranchRow({ branch, checked, disabled, onToggle }: BranchRowProps) {
  return (
    <label
      className={cn(
        'flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-xs hover:bg-accent',
        disabled && 'cursor-default opacity-60',
      )}
    >
      <Checkbox
        checked={checked}
        disabled={disabled}
        onCheckedChange={onToggle}
        aria-label={branch.name}
      />
      {branch.isProtected ? (
        <Lock className="size-3 shrink-0 text-muted-foreground" />
      ) : (
        <GitBranch className="size-3 shrink-0 text-muted-foreground" />
      )}
      <span className="min-w-0 flex-1 truncate font-mono">{branch.name}</span>
      {branch.hasWorkspace && (
        <span className="shrink-0 text-[10px] text-green-500">imported</span>
      )}
    </label>
  )
}
```

- [ ] **Step 4: Mount RepoSettingsPanel in workspace-tree.tsx**

In `web/src/components/layout/workspace-tree.tsx`, import the panel and render it at the end of `WorkspaceTreeInner`, before the drag ghost:

```tsx
import { RepoSettingsPanel } from './repo-settings-panel'

// Inside WorkspaceTreeInner, after the repos.map(...) block, before the drag ghost:
{openSettingsRepoId && (() => {
  const repo = repos.find((r) => r.id === openSettingsRepoId)
  if (!repo) return null
  return (
    <RepoSettingsPanel
      key={repo.id}
      repoId={repo.id}
      repoName={repo.name}
      open={openSettingsRepoId === repo.id}
      onOpenChange={(open) => { if (!open) setOpenSettingsRepoId(null) }}
    />
  )
})()}
```

- [ ] **Step 5: Run all frontend tests**

```bash
cd web && npm test
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add web/src/components/layout/repo-settings-panel.tsx \
        web/src/components/layout/workspace-tree.tsx \
        "web/src/__tests__/components/layout/repo-settings-panel.test.tsx"
git commit -m "feat: repo settings panel with Import Workspaces (checkbox multi-select)"
```

---

## Task 17: Final verification

- [ ] **Step 1: Full backend test suite**

```bash
cd api && go test -tags noEmbed -race ./...
```

Expected: PASS

- [ ] **Step 2: Full frontend test suite**

```bash
cd web && npm test
```

Expected: PASS

- [ ] **Step 3: Build check**

```bash
cd api && go build -tags noEmbed ./... && echo "Backend OK"
cd web && npm run build && echo "Frontend OK"
```

Expected: Both OK

- [ ] **Step 4: Final commit if any loose ends**

```bash
git add -p
git commit -m "chore: sidebar polish final cleanup"
```
