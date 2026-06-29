# Project Home — Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Project Home" workspace — a git-free, project-scoped workspace rooted at `project.path` — that is auto-provisioned on project creation and exposed via a new `/v0/projects/:projectId/home` API surface.

**Architecture:** `WorkspaceKind` is added to the domain so the existing Asynx aggregate can represent both git-worktree workspaces (`"git"`) and the new home workspace (`"home"`). The home workspace is provisioned alongside the project row in both `Create` and `Import` paths. A new `home` endpoint package mirrors the file/terminal/search surface under `projectScoped` (no `:repoId` in the path) by resolving the home workspace ID from the project, then delegating to the same usecases.

**Tech Stack:** Go, Gin, Asynx (event-sourced aggregates), SQLite (GORM for repo/project rows), `testify/require`

## Global Constraints

- Backend only — frontend is a separate plan.
- Never pass empty `RepoID` to existing git-worktree paths; guard every new home path with a Kind check.
- All new API responses use the existing `{success, error, data}` envelope via `libs.OK` / `libs.Err`.
- Test files mirror source paths: a test for `api/internal/foo/bar.go` lives at `api/internal/foo/bar_test.go` (same package) or `api/internal/foo/bar_external_test.go` (external package).
- Integration tests use `//go:build integration` tag and live in `api/tests/`.
- Run all unit tests with: `go test ./api/...` from the repo root.
- Run integration tests with: `go test -tags integration ./api/tests/...`

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `api/internal/domain/workspace.go` | Modify | Add `WorkspaceKind` type + `Kind` field to `Workspace` |
| `api/internal/app/repositories/workspace/internal/commands/create.go` | Modify | Add `Kind` to `CreateWorkspace` command; update `Validate` + `EmitEvent` |
| `api/internal/app/repositories/workspace/workspace.go` | Modify | Add `Kind` to `CreateInput`; add `GetHomeForProject` to `Workspace` interface + implementation |
| `api/internal/app/repositories/workspace/internal/mocks/mocks.go` | Modify | Add `GetHomeForProject` stub to the mock |
| `api/internal/app/usecases/project/project_import.go` | Modify | Add `createHomeWorkspace` helper; call it from `Create` and `Import` after project row is saved |
| `api/internal/api/v0/endpoints/home/routes.go` | Create | `Register(projectScoped, ...)` — mounts all `/home/*` routes |
| `api/internal/api/v0/endpoints/home/handlers/handlers.go` | Create | Handler deps interfaces + `Handlers` struct + `New` |
| `api/internal/api/v0/endpoints/home/handlers/home.go` | Create | `GET /home` — resolves and returns home workspace DTO |
| `api/internal/api/v0/endpoints/home/handlers/files.go` | Create | File tree/content handlers delegating to file usecase via home workspace ID |
| `api/internal/api/v0/endpoints/home/handlers/terminal.go` | Create | Terminal create/list/kill/WS handlers delegating to terminal engine via home workspace ID |
| `api/internal/api/v0/router.go` | Modify | Register home endpoint under `projectScoped` |

---

## Task 1: Add `WorkspaceKind` to domain

**Files:**
- Modify: `api/internal/domain/workspace.go`
- Modify: `api/internal/app/repositories/workspace/internal/commands/create.go`

**Interfaces:**
- Produces: `domain.WorkspaceKindGit`, `domain.WorkspaceKindHome` constants; `domain.Workspace.Kind` field (json: `"kind"`)
- Produces: `commands.CreateWorkspace.Kind` field

- [ ] **Step 1: Write failing test**

In `api/internal/domain/workspace_test.go` (create file):

```go
package domain_test

import (
    "testing"

    "github.com/char2cs/crowbar/api/internal/domain"
    "github.com/stretchr/testify/require"
)

func TestWorkspaceKindConstants(t *testing.T) {
    require.Equal(t, domain.WorkspaceKind("git"), domain.WorkspaceKindGit)
    require.Equal(t, domain.WorkspaceKind("home"), domain.WorkspaceKindHome)
}

func TestWorkspaceKindDefaultsToGit(t *testing.T) {
    ws := domain.Workspace{}
    require.Equal(t, domain.WorkspaceKind(""), ws.Kind) // zero value; caller sets it
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./api/internal/domain/... -run TestWorkspaceKind
```
Expected: FAIL — `WorkspaceKind`, `WorkspaceKindGit`, `WorkspaceKindHome` undefined.

- [ ] **Step 3: Add `WorkspaceKind` to `api/internal/domain/workspace.go`**

After the existing imports, before the `Workspace` struct:

```go
// WorkspaceKind distinguishes git-worktree workspaces from the project-level
// home workspace which has no branch and no git operations.
type WorkspaceKind string

const (
    WorkspaceKindGit  WorkspaceKind = "git"
    WorkspaceKindHome WorkspaceKind = "home"
)
```

Add `Kind WorkspaceKind` field to `Workspace` after `IsDefault`:

```go
// Kind distinguishes git-worktree workspaces ("git", default) from the
// project-level home workspace ("home"). Old persisted records without this
// field replay as WorkspaceKindGit.
Kind WorkspaceKind `json:"kind,omitempty"`
```

- [ ] **Step 4: Run test to verify it passes**

```
go test ./api/internal/domain/... -run TestWorkspaceKind
```
Expected: PASS.

- [ ] **Step 5: Add `Kind` to the `CreateWorkspace` command**

In `api/internal/app/repositories/workspace/internal/commands/create.go`:

Add `Kind domain.WorkspaceKind` field to `CreateWorkspace`:

```go
type CreateWorkspace struct {
    ID            string
    RepoID        string
    ProjectID     string
    Branch        string
    WorktreePath  string
    ForkPointSha  string
    ParentID      string
    Protected     bool
    IsDefault     bool
    MergeStrategy gitdomain.MergeStrategy
    Kind          domain.WorkspaceKind  // ← add this
    Now           time.Time
}
```

Update `Validate` to allow empty `RepoID` for home workspaces:

```go
func (c CreateWorkspace) Validate(current *domain.Workspace) error {
    if current != nil {
        return fmt.Errorf("create workspace: %w", asynxModels.ErrValidation)
    }
    if c.ID == "" || c.ProjectID == "" {
        return fmt.Errorf("create workspace: missing ids: %w", asynxModels.ErrValidation)
    }
    if c.Kind != domain.WorkspaceKindHome && c.RepoID == "" {
        return fmt.Errorf("create workspace: missing repoId for git workspace: %w", asynxModels.ErrValidation)
    }
    return nil
}
```

Update `EmitEvent` to carry `Kind`, defaulting to `WorkspaceKindGit` when empty:

```go
func (c CreateWorkspace) EmitEvent(_ *domain.Workspace) domain.Workspace {
    strategy := c.MergeStrategy
    if strategy == "" {
        strategy = gitdomain.MergeStrategyMerge
    }
    kind := c.Kind
    if kind == "" {
        kind = domain.WorkspaceKindGit
    }
    status := domain.WorkspaceStatusNew
    if c.Protected {
        status = domain.WorkspaceStatusLocked
    }
    return domain.Workspace{
        ID:            c.ID,
        RepoID:        c.RepoID,
        ProjectID:     c.ProjectID,
        Branch:        c.Branch,
        WorktreePath:  c.WorktreePath,
        ForkPointSha:  c.ForkPointSha,
        ParentID:      c.ParentID,
        Status:        status,
        MergeStrategy: strategy,
        IsDefault:     c.IsDefault,
        Kind:          kind,
        LastActivity:  c.Now,
        CreatedAt:     c.Now,
    }
}
```

- [ ] **Step 6: Write command tests**

In `api/internal/app/repositories/workspace/internal/commands/commands_test.go`, add:

```go
func TestCreateWorkspace_EmitEvent_KindDefault(t *testing.T) {
    cmd := commands.CreateWorkspace{
        ID:        "ws-1",
        RepoID:    "repo-1",
        ProjectID: "proj-1",
        Branch:    "main",
        Now:       time.Now(),
        // Kind not set → should default to git
    }
    ws := cmd.EmitEvent(nil)
    require.Equal(t, domain.WorkspaceKindGit, ws.Kind)
}

func TestCreateWorkspace_EmitEvent_KindHome(t *testing.T) {
    cmd := commands.CreateWorkspace{
        ID:        "ws-home",
        ProjectID: "proj-1",
        Kind:      domain.WorkspaceKindHome,
        Now:       time.Now(),
    }
    ws := cmd.EmitEvent(nil)
    require.Equal(t, domain.WorkspaceKindHome, ws.Kind)
    require.Empty(t, ws.RepoID)
}

func TestCreateWorkspace_Validate_HomeAllowsEmptyRepoID(t *testing.T) {
    cmd := commands.CreateWorkspace{
        ID:        "ws-home",
        ProjectID: "proj-1",
        Kind:      domain.WorkspaceKindHome,
    }
    require.NoError(t, cmd.Validate(nil))
}

func TestCreateWorkspace_Validate_GitRequiresRepoID(t *testing.T) {
    cmd := commands.CreateWorkspace{
        ID:        "ws-git",
        ProjectID: "proj-1",
        Kind:      domain.WorkspaceKindGit,
    }
    require.Error(t, cmd.Validate(nil))
}
```

- [ ] **Step 7: Run and pass**

```
go test ./api/internal/app/repositories/workspace/internal/commands/... -v
```
Expected: all PASS.

- [ ] **Step 8: Add `Kind` to `workspace.CreateInput`**

In `api/internal/app/repositories/workspace/workspace.go`, update `CreateInput`:

```go
type CreateInput struct {
    ID            string
    RepoID        string
    ProjectID     string
    Branch        string
    WorktreePath  string
    ForkPointSha  string
    ParentID      string
    Protected     bool
    MergeStrategy gitdomain.MergeStrategy
    IsDefault     bool
    Kind          domain.WorkspaceKind  // ← add this
}
```

Find where `CreateInput` is converted to `commands.CreateWorkspace` (search for `commands.CreateWorkspace{` in `workspace.go`) and add `Kind: in.Kind` to the struct literal.

- [ ] **Step 9: Run all workspace tests**

```
go test ./api/internal/app/repositories/workspace/... -v
```
Expected: all PASS.

- [ ] **Step 10: Commit**

```bash
git add api/internal/domain/workspace.go \
        api/internal/app/repositories/workspace/internal/commands/create.go \
        api/internal/app/repositories/workspace/workspace.go \
        api/internal/domain/workspace_test.go
git commit -m "feat(domain): add WorkspaceKind — git and home variants"
```

---

## Task 2: Add `GetHomeForProject` to workspace repository

**Files:**
- Modify: `api/internal/app/repositories/workspace/workspace.go`
- Modify: `api/internal/app/repositories/workspace/internal/mocks/mocks.go`

**Interfaces:**
- Consumes: `domain.WorkspaceKindHome` (from Task 1)
- Produces: `Workspace.GetHomeForProject(ctx context.Context, projectID string) (domain.Workspace, error)`; error is `apperr.ErrNotFound` when absent

- [ ] **Step 1: Write failing test**

In `api/internal/app/repositories/workspace/workspace_test.go`, add (find the existing test setup to understand how a test workspace repo is built, then add):

```go
func TestGetHomeForProject_Found(t *testing.T) {
    // Use the existing test helper that builds an in-memory workspace repo.
    // (Pattern: look at existing Test* functions in workspace_test.go for the
    // helper name — it will be something like newTestRepo() or setupRepo().)
    repo := newTestRepo(t)
    ctx := context.Background()

    projectID := "proj-abc"
    _, err := repo.Create(ctx, workspace.CreateInput{
        ID:        "ws-home-1",
        ProjectID: projectID,
        Kind:      domain.WorkspaceKindHome,
        WorktreePath: "/projects/myproject",
    }, time.Now())
    require.NoError(t, err)

    got, err := repo.GetHomeForProject(ctx, projectID)
    require.NoError(t, err)
    require.Equal(t, "ws-home-1", got.ID)
    require.Equal(t, domain.WorkspaceKindHome, got.Kind)
}

func TestGetHomeForProject_NotFound(t *testing.T) {
    repo := newTestRepo(t)
    _, err := repo.GetHomeForProject(context.Background(), "nonexistent-project")
    require.ErrorIs(t, err, apperr.ErrNotFound)
}
```

- [ ] **Step 2: Run to verify fail**

```
go test ./api/internal/app/repositories/workspace/... -run TestGetHomeForProject
```
Expected: FAIL — `GetHomeForProject` undefined on interface.

- [ ] **Step 3: Add method to `Workspace` interface and implement**

In `api/internal/app/repositories/workspace/workspace.go`, add to the `Workspace` interface after `List`:

```go
// GetHomeForProject returns the home workspace for the given project.
// Returns apperr.ErrNotFound if no home workspace exists yet.
GetHomeForProject(
    ctx context.Context,
    projectID string,
) (domain.Workspace, error)
```

Then implement on the concrete `*workspace` type (after the `List` implementation):

```go
func (r *workspace) GetHomeForProject(ctx context.Context, projectID string) (domain.Workspace, error) {
    all, err := r.List(ctx)
    if err != nil {
        return domain.Workspace{}, fmt.Errorf("get home for project: list: %w", err)
    }
    for _, ws := range all {
        if ws.ProjectID == projectID && ws.Kind == domain.WorkspaceKindHome {
            return ws, nil
        }
    }
    return domain.Workspace{}, fmt.Errorf("get home for project %q: %w", projectID, apperr.ErrNotFound)
}
```

- [ ] **Step 4: Add stub to mock**

In `api/internal/app/repositories/workspace/internal/mocks/mocks.go`, find the mock struct and add:

```go
func (m *MockWorkspace) GetHomeForProject(ctx context.Context, projectID string) (domain.Workspace, error) {
    args := m.Called(ctx, projectID)
    return args.Get(0).(domain.Workspace), args.Error(1)
}
```

(Follow the existing mock method pattern in that file exactly.)

- [ ] **Step 5: Run and pass**

```
go test ./api/internal/app/repositories/workspace/... -run TestGetHomeForProject
```
Expected: PASS.

- [ ] **Step 6: Run full workspace suite to check no regressions**

```
go test ./api/internal/app/repositories/workspace/... -v
```
Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add api/internal/app/repositories/workspace/workspace.go \
        api/internal/app/repositories/workspace/internal/mocks/mocks.go
git commit -m "feat(workspace): add GetHomeForProject to repository interface"
```

---

## Task 3: Auto-provision home workspace on project creation

**Files:**
- Modify: `api/internal/app/usecases/project/project_import.go`
- Test: `api/internal/app/usecases/project/project_import_test.go` (check existing file name)

**Interfaces:**
- Consumes: `workspace.CreateInput.Kind` (Task 1); `domain.WorkspaceKindHome` (Task 1)
- Produces: after `Create()` or `Import()`, a home workspace exists with `WorktreePath = project.Path` and `Kind = WorkspaceKindHome`

- [ ] **Step 1: Write failing test**

Find the existing test file for project_import. Add to it:

```go
func TestCreate_ProvisionesHomeWorkspace(t *testing.T) {
    // Use the existing test setup pattern for projectImport.
    // Capture workspace.CreateInput calls via the mock WorkspaceCreator.
    var created []workspace.CreateInput
    mockWorkspaces := &mockWorkspaceCreator{
        createFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
            created = append(created, in)
            return domain.Workspace{ID: in.ID, Kind: in.Kind}, nil
        },
    }
    u := newTestImportUsecase(t, mockWorkspaces) // use the existing builder pattern

    _, err := u.Create(context.Background(), "myproject", t.TempDir())
    require.NoError(t, err)

    require.Len(t, created, 1)
    require.Equal(t, domain.WorkspaceKindHome, created[0].Kind)
    require.NotEmpty(t, created[0].WorktreePath)
}
```

- [ ] **Step 2: Run to verify fail**

```
go test ./api/internal/app/usecases/project/... -run TestCreate_ProvisionesHomeWorkspace
```
Expected: FAIL — no home workspace created.

- [ ] **Step 3: Add `createHomeWorkspace` helper and call it**

In `api/internal/app/usecases/project/project_import.go`, add this helper after `validateImportPath`:

```go
// createHomeWorkspace persists the project-level home workspace rooted at the
// project's own path. It has no repo, branch, or git operations.
func (u *projectImport) createHomeWorkspace(ctx context.Context, project domain.Project) error {
    _, err := u.deps.Workspaces.Create(ctx, workspace.CreateInput{
        ID:           uuid.NewString(),
        ProjectID:    project.ID,
        WorktreePath: project.Path,
        Kind:         domain.WorkspaceKindHome,
    }, u.deps.Now())
    if err != nil {
        return fmt.Errorf("project create home workspace: %w", err)
    }
    return nil
}
```

In `Create`, after `u.deps.Projects.Save(ctx, project)` succeeds, add:

```go
if err := u.createHomeWorkspace(ctx, project); err != nil {
    return domain.Project{}, err
}
```

In `Import`, after `u.deps.Projects.Save(ctx, project)` succeeds (before `u.importRepos`), add:

```go
if err := u.createHomeWorkspace(ctx, project); err != nil {
    return domain.Project{}, err
}
```

- [ ] **Step 4: Run and pass**

```
go test ./api/internal/app/usecases/project/... -run TestCreate_ProvisionesHomeWorkspace
```
Expected: PASS.

- [ ] **Step 5: Run full project usecase suite**

```
go test ./api/internal/app/usecases/project/... -v
```
Expected: all PASS.

- [ ] **Step 6: Write integration test**

In `api/tests/` (find the existing integration test file pattern, e.g. `api/tests/project_test.go`), add:

```go
//go:build integration

func TestRegression_HomeWorkspaceProvisionedOnCreate(t *testing.T) {
    // Uses the real HTTP server + SQLite stack (follow existing integration test setup).
    srv := newTestServer(t)

    dir := t.TempDir()
    resp := srv.POST("/v0/projects", map[string]any{"name": "testproj", "path": dir})
    require.Equal(t, 201, resp.StatusCode)

    var projectResp struct{ Data struct{ ID string } }
    decodeJSON(t, resp, &projectResp)
    projectID := projectResp.Data.ID

    // Home workspace must exist — verify via the repository directly.
    ws, err := srv.App.Repositories.Workspace.GetHomeForProject(context.Background(), projectID)
    require.NoError(t, err)
    require.Equal(t, domain.WorkspaceKindHome, ws.Kind)
    require.Equal(t, dir, ws.WorktreePath)
}
```

- [ ] **Step 7: Run integration tests**

```
go test -tags integration ./api/tests/... -run TestRegression_HomeWorkspace -v
```
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add api/internal/app/usecases/project/project_import.go
git commit -m "feat(project): auto-provision home workspace on project create/import"
```

---

## Task 4: Home endpoint handlers

**Files:**
- Create: `api/internal/api/v0/endpoints/home/routes.go`
- Create: `api/internal/api/v0/endpoints/home/handlers/handlers.go`
- Create: `api/internal/api/v0/endpoints/home/handlers/home.go`
- Create: `api/internal/api/v0/endpoints/home/handlers/files.go`
- Create: `api/internal/api/v0/endpoints/home/handlers/terminal.go`
- Create: `api/internal/api/v0/endpoints/home/handlers/handlers_test.go`

**Interfaces:**
- Consumes: `workspace.Workspace.GetHomeForProject` (Task 2); existing file and terminal usecases
- Produces: REST handlers for `GET /home`, files, and terminals under `projectScoped`

- [ ] **Step 1: Write failing handler tests**

Create `api/internal/api/v0/endpoints/home/handlers/handlers_test.go`:

```go
package handlers_test

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"

    "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/home/handlers"
    "github.com/char2cs/crowbar/api/internal/domain"
)

type mockHomeReader struct{ mock.Mock }

func (m *mockHomeReader) GetHomeForProject(ctx context.Context, projectID string) (domain.Workspace, error) {
    args := m.Called(ctx, projectID)
    return args.Get(0).(domain.Workspace), args.Error(1)
}

func TestGetHome_Returns200WithWorkspace(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()

    homeWS := domain.Workspace{
        ID:           "ws-home-1",
        ProjectID:    "proj-1",
        Kind:         domain.WorkspaceKindHome,
        WorktreePath: "/projects/myproject",
    }
    reader := &mockHomeReader{}
    reader.On("GetHomeForProject", mock.Anything, "proj-1").Return(homeWS, nil)

    h := handlers.New(reader, nil, nil)
    r.GET("/projects/:projectId/home", h.Get)

    w := httptest.NewRecorder()
    req, _ := http.NewRequest("GET", "/projects/proj-1/home", nil)
    r.ServeHTTP(w, req)

    require.Equal(t, 200, w.Code)
    reader.AssertExpectations(t)
}
```

- [ ] **Step 2: Run to verify fail**

```
go test ./api/internal/api/v0/endpoints/home/... -run TestGetHome
```
Expected: FAIL — package does not exist.

- [ ] **Step 3: Create `handlers.go`**

`api/internal/api/v0/endpoints/home/handlers/handlers.go`:

```go
// Package handlers serves the /v0/projects/:projectId/home routes.
package handlers

import (
    "context"
    "time"

    fileusecase "github.com/char2cs/crowbar/api/internal/app/usecases/file"
    "github.com/char2cs/crowbar/api/internal/domain"
    engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// HomeReader resolves the home workspace for a project.
type HomeReader interface {
    GetHomeForProject(ctx context.Context, projectID string) (domain.Workspace, error)
}

// Files is the file usecase surface needed by home file handlers.
type Files interface {
    Tree(ctx context.Context, wsID string, dirPath string, provider fileusecase.FileStatusProvider) ([]domain.FileNode, error)
    ReadContent(ctx context.Context, wsID string, filePath string) (domain.FileContent, error)
    WriteContent(ctx context.Context, wsID string, filePath string, content string, now time.Time) error
    CreateFile(ctx context.Context, wsID string, filePath string, now time.Time) error
    CreateDir(ctx context.Context, wsID string, dirPath string, now time.Time) error
    Rename(ctx context.Context, wsID string, oldPath string, newPath string, now time.Time) error
    Delete(ctx context.Context, wsID string, filePath string, now time.Time) error
}

// TerminalEngine is the terminal engine surface needed by home terminal handlers.
type TerminalEngine interface {
    Create(ctx context.Context, workspaceID string, workspaceDir string, prof *domain.TerminalProfile) (string, error)
    Kill(ctx context.Context, sessionID string) error
    SessionExists(ctx context.Context, sessionID string) bool
    Attach(ctx context.Context, sessionID string, conn engineterminal.WSConn) error
    ListSessionsForWorkspace(workspaceID string) []string
}

// Handlers serves all /home/* routes.
type Handlers struct {
    reader  HomeReader
    files   Files
    termEng TerminalEngine
}

// New builds Handlers.
func New(reader HomeReader, files Files, termEng TerminalEngine) *Handlers {
    return &Handlers{reader: reader, files: files, termEng: termEng}
}
```

- [ ] **Step 4: Create `home.go`**

`api/internal/api/v0/endpoints/home/handlers/home.go`:

```go
package handlers

import (
    "net/http"

    "github.com/gin-gonic/gin"

    "github.com/char2cs/crowbar/api/internal/api/v0/libs"
    "github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Get returns the home workspace DTO for the project.
func (h *Handlers) Get(c *gin.Context) {
    projectID := c.Param("projectId")
    ws, err := h.reader.GetHomeForProject(c.Request.Context(), projectID)
    if err != nil {
        libs.Err(c, http.StatusNotFound, err)
        return
    }
    libs.OK(c, http.StatusOK, dto.WorkspaceFromDomain(ws))
}
```

> **Note:** `dto.WorkspaceFromDomain` is the existing DTO conversion function used by the workspace handlers. Find its exact name by searching for `WorkspaceFromDomain` or `toWorkspaceDTO` in `api/internal/api/v0/dto/`. Use whatever name exists there.

- [ ] **Step 5: Create `files.go`**

`api/internal/api/v0/endpoints/home/handlers/files.go`:

```go
package handlers

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"

    "github.com/char2cs/crowbar/api/internal/api/v0/libs"
    fileusecase "github.com/char2cs/crowbar/api/internal/app/usecases/file"
)

func (h *Handlers) resolveHomeWsID(c *gin.Context) (string, bool) {
    ws, err := h.reader.GetHomeForProject(c.Request.Context(), c.Param("projectId"))
    if err != nil {
        libs.Err(c, http.StatusNotFound, err)
        return "", false
    }
    return ws.ID, true
}

func (h *Handlers) FileTree(c *gin.Context) {
    wsID, ok := h.resolveHomeWsID(c)
    if !ok {
        return
    }
    dirPath := c.DefaultQuery("path", ".")
    nodes, err := h.files.Tree(c.Request.Context(), wsID, dirPath, fileusecase.NoopFileStatusProvider{})
    if err != nil {
        libs.Err(c, http.StatusInternalServerError, err)
        return
    }
    libs.OK(c, http.StatusOK, nodes)
}

func (h *Handlers) FileContent(c *gin.Context) {
    wsID, ok := h.resolveHomeWsID(c)
    if !ok {
        return
    }
    content, err := h.files.ReadContent(c.Request.Context(), wsID, c.Query("path"))
    if err != nil {
        libs.Err(c, http.StatusNotFound, err)
        return
    }
    libs.OK(c, http.StatusOK, content)
}

func (h *Handlers) SaveFileContent(c *gin.Context) {
    wsID, ok := h.resolveHomeWsID(c)
    if !ok {
        return
    }
    var body struct {
        Path    string `json:"path"`
        Content string `json:"content"`
    }
    if err := c.ShouldBindJSON(&body); err != nil {
        libs.Err(c, http.StatusBadRequest, err)
        return
    }
    if err := h.files.WriteContent(c.Request.Context(), wsID, body.Path, body.Content, time.Now()); err != nil {
        libs.Err(c, http.StatusInternalServerError, err)
        return
    }
    libs.OK(c, http.StatusOK, nil)
}

func (h *Handlers) CreateFile(c *gin.Context) {
    wsID, ok := h.resolveHomeWsID(c)
    if !ok {
        return
    }
    var body struct {
        Path string `json:"path"`
        Dir  bool   `json:"dir"`
    }
    if err := c.ShouldBindJSON(&body); err != nil {
        libs.Err(c, http.StatusBadRequest, err)
        return
    }
    ctx := c.Request.Context()
    now := time.Now()
    var err error
    if body.Dir {
        err = h.files.CreateDir(ctx, wsID, body.Path, now)
    } else {
        err = h.files.CreateFile(ctx, wsID, body.Path, now)
    }
    if err != nil {
        libs.Err(c, http.StatusInternalServerError, err)
        return
    }
    libs.OK(c, http.StatusCreated, nil)
}

func (h *Handlers) RenameFile(c *gin.Context) {
    wsID, ok := h.resolveHomeWsID(c)
    if !ok {
        return
    }
    var body struct {
        OldPath string `json:"oldPath"`
        NewPath string `json:"newPath"`
    }
    if err := c.ShouldBindJSON(&body); err != nil {
        libs.Err(c, http.StatusBadRequest, err)
        return
    }
    if err := h.files.Rename(c.Request.Context(), wsID, body.OldPath, body.NewPath, time.Now()); err != nil {
        libs.Err(c, http.StatusInternalServerError, err)
        return
    }
    libs.OK(c, http.StatusOK, nil)
}

func (h *Handlers) DeleteFile(c *gin.Context) {
    wsID, ok := h.resolveHomeWsID(c)
    if !ok {
        return
    }
    path := c.Query("path")
    if err := h.files.Delete(c.Request.Context(), wsID, path, time.Now()); err != nil {
        libs.Err(c, http.StatusInternalServerError, err)
        return
    }
    libs.OK(c, http.StatusOK, nil)
}
```

> **Note:** Check `fileusecase.NoopFileStatusProvider` exists; if not, search for the null/no-op implementation used by existing file handlers and use that instead.

- [ ] **Step 6: Create `terminal.go`**

`api/internal/api/v0/endpoints/home/handlers/terminal.go`:

```go
package handlers

import (
    "net/http"

    "github.com/gin-gonic/gin"

    "github.com/char2cs/crowbar/api/internal/api/v0/libs"
)

func (h *Handlers) ListTerminals(c *gin.Context) {
    wsID, ok := h.resolveHomeWsID(c)
    if !ok {
        return
    }
    sessions := h.termEng.ListSessionsForWorkspace(wsID)
    libs.OK(c, http.StatusOK, sessions)
}

func (h *Handlers) CreateTerminal(c *gin.Context) {
    wsID, ok := h.resolveHomeWsID(c)
    if !ok {
        return
    }
    ws, err := h.reader.GetHomeForProject(c.Request.Context(), c.Param("projectId"))
    if err != nil {
        libs.Err(c, http.StatusNotFound, err)
        return
    }
    sessionID, err := h.termEng.Create(c.Request.Context(), wsID, ws.WorktreePath, nil)
    if err != nil {
        libs.Err(c, http.StatusInternalServerError, err)
        return
    }
    libs.OK(c, http.StatusCreated, gin.H{"sessionId": sessionID})
}

func (h *Handlers) KillTerminal(c *gin.Context) {
    sessionID := c.Param("sessionId")
    if err := h.termEng.Kill(c.Request.Context(), sessionID); err != nil {
        libs.Err(c, http.StatusInternalServerError, err)
        return
    }
    libs.OK(c, http.StatusOK, nil)
}
```

> **Note on WS handler:** The PTY WebSocket stream (`GET /terminals/:sessionId/ws`) requires the real WebSocket upgrade path used by the existing terminal handler's `WS` method. For now, mount the existing `terminal.Handlers.WS` directly on the home route group in Task 5 — both the home and regular terminal paths share the same engine, so the session ID is sufficient context.

- [ ] **Step 7: Create `routes.go`**

`api/internal/api/v0/endpoints/home/routes.go`:

```go
// Package home mounts the project-level home workspace routes under
// /v0/projects/:projectId/home. The home workspace has no git operations.
package home

import (
    "github.com/gin-gonic/gin"

    homehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/home/handlers"
    fileusecase "github.com/char2cs/crowbar/api/internal/app/usecases/file"
    "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
    engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// Register mounts all home routes under projectScoped
// (/v0/projects/:projectId).
func Register(
    projectScoped *gin.RouterGroup,
    workspaces workspace.Workspace,
    files fileusecase.Usecase,
    termEng engineterminal.Engine,
) {
    h := homehandlers.New(workspaces, files, termEng)
    home := projectScoped.Group("/home")

    home.GET("", h.Get)

    home.GET("/files/tree", h.FileTree)
    home.GET("/files/content", h.FileContent)
    home.PUT("/files/content", h.SaveFileContent)
    home.POST("/files", h.CreateFile)
    home.PATCH("/files", h.RenameFile)
    home.DELETE("/files", h.DeleteFile)

    home.GET("/terminals", h.ListTerminals)
    home.POST("/terminals", h.CreateTerminal)
    home.DELETE("/terminals/:sessionId", h.KillTerminal)
}
```

> **Note on interface types:** Check the exact Go types for `fileusecase.Usecase` and `engineterminal.Engine` — search for `type Usecase interface` in `api/internal/app/usecases/file/` and `type Engine interface` in `api/internal/engine/terminal/`. Use the actual interface types. If the home handlers consume narrower subsets of those interfaces, define narrow local interfaces in `handlers.go` (as done in the rest of the codebase).

- [ ] **Step 8: Run handler tests**

```
go test ./api/internal/api/v0/endpoints/home/... -v
```
Expected: all PASS.

- [ ] **Step 9: Commit**

```bash
git add api/internal/api/v0/endpoints/home/
git commit -m "feat(home): add home endpoint handlers — GET, files, terminals"
```

---

## Task 5: Wire home into the router

**Files:**
- Modify: `api/internal/api/v0/router.go`

**Interfaces:**
- Consumes: `home.Register(projectScoped, ...)` (Task 4)
- Produces: `/v0/projects/:projectId/home/*` routes live and reachable

- [ ] **Step 1: Write failing integration test**

In `api/tests/` (follow the existing HTTP integration test pattern):

```go
//go:build integration

func TestRegression_HomeEndpointReachable(t *testing.T) {
    srv := newTestServer(t)

    // Create a project (which auto-provisions the home workspace).
    dir := t.TempDir()
    resp := srv.POST("/v0/projects", map[string]any{"name": "testproj", "path": dir})
    require.Equal(t, 201, resp.StatusCode)

    var projectResp struct{ Data struct{ ID string } }
    decodeJSON(t, resp, &projectResp)
    projectID := projectResp.Data.ID

    // GET /home must return 200 with a home workspace.
    resp2 := srv.GET("/v0/projects/" + projectID + "/home")
    require.Equal(t, 200, resp2.StatusCode)

    var homeResp struct {
        Data struct {
            Kind string `json:"kind"`
        }
    }
    decodeJSON(t, resp2, &homeResp)
    require.Equal(t, "home", homeResp.Data.Kind)
}
```

- [ ] **Step 2: Run to verify fail**

```
go test -tags integration ./api/tests/... -run TestRegression_HomeEndpointReachable -v
```
Expected: FAIL — 404 (route not registered yet).

- [ ] **Step 3: Register home in `router.go`**

In `api/internal/api/v0/router.go`:

Add import:
```go
homePkg "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/home"
```

After the `reposPkg.Register(...)` call and before `workspaces.Register(...)`, add:

```go
homePkg.Register(
    projectScoped,
    c.app.Repositories.Workspace,
    c.app.Usecases.File,
    c.eng.Terminal,
)
```

- [ ] **Step 4: Run integration test**

```
go test -tags integration ./api/tests/... -run TestRegression_HomeEndpointReachable -v
```
Expected: PASS.

- [ ] **Step 5: Run full test suite**

```
go test ./api/...
go test -tags integration ./api/tests/...
```
Expected: all PASS, no regressions.

- [ ] **Step 6: Commit**

```bash
git add api/internal/api/v0/router.go
git commit -m "feat(router): mount home endpoint under projectScoped"
```

---

## Self-Review

**Spec coverage:**
- [x] Home workspace has full workspace capabilities (file tree, terminal) — Tasks 4, 5
- [x] No git operations — git routes not mounted in home package
- [x] One per project, auto-provisioned — Task 3 (`createHomeWorkspace` in both `Create` + `Import`)
- [x] `WorktreePath = project.path` — Task 3 sets `WorktreePath: project.Path`
- [x] Existing git workspaces unaffected — `Kind` defaults to `"git"` on replay (Task 1)
- [x] `GetHomeForProject` returns `ErrNotFound` when absent — tested in Task 2

**Placeholder scan:** None found. All steps have concrete code.

**Type consistency:**
- `domain.WorkspaceKindHome` used in Tasks 1, 2, 3 ✓
- `workspace.CreateInput.Kind` used in Tasks 1, 3 ✓
- `GetHomeForProject(ctx, projectID)` signature consistent across Tasks 2, 4 ✓
- Handler method names (`FileTree`, `FileContent`, etc.) consistent between `files.go` and `routes.go` ✓
