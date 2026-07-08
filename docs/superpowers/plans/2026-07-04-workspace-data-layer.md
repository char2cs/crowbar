# Workspace Data Layer & Git Lock Granularity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop the Workspaces sidebar tree from hanging/tanking performance by giving the workspace data layer a real scoped query path (instead of scanning every workspace in the whole install on every request), and split the git engine's per-repo lock so a background network fetch can't block a concurrent local read.

**Architecture:** A new query-only `workspace_directory` projection in the existing global view.db lets list/snapshot reads be scoped by repo instead of opening every workspace's per-entity SQLite files; a separate, narrower `workspace.Workspace.ListInRepo` (reading directly from the per-entity stores, same mechanism as today's `List`) keeps the merge-eligibility broadcast overlay's sibling lookup exactly as consistent as it is today, just repo-scoped. The global view.db's connection pool is raised now that it serves this new read-heavy table. The git engine's per-repo `sync.Mutex` becomes a `sync.RWMutex` so reads no longer share a lock with mutations/network fetches.

**Tech Stack:** Go backend (`api/`), GORM/SQLite, no frontend changes.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-04-workspace-data-layer-design.md`.
- Backend unit tests: `cd api && go test -tags noEmbed -race ./...`. Backend integration tests: `cd api && go test -tags 'integration noEmbed' -race -v -timeout 600s -p 1 ./...`.
- The per-entity event-sourced workspace storage model (one `event_stream.db` + `view.db` per workspace, `rm -rf`-on-delete) is unchanged by this plan — every new table/method is additive.
- `workspace_directory` lives in the existing global view.db (`adapter.Container.GlobalView()`), not a new database file.
- The global view.db connection pool size is `8` (`globalViewMaxOpenConns = 8` — pinned constant, not user-configurable).
- New/changed exported names used across tasks (for consistency — do not rename mid-plan):
  - `workspace.Workspace.ListInRepo(ctx, projectID, repoID string) ([]domain.Workspace, error)` (Task 1)
  - package `github.com/char2cs/crowbar/api/internal/app/repositories/workspace/directory`, type `directory.Directory`, `directory.Row`, func `directory.New(db *gorm.DB) (Directory, error)` (Task 2)
  - `(*repositories.Container).ListWorkspacesInRepo(ctx, projectID, repoID string) ([]domain.Workspace, error)` (Task 3)
  - `(*repositories.Container).syncDirectory(ctx, ws)`, `(*repositories.Container).rebuildDirectory(ctx)` (unexported, Task 3)

---

### Task 1: `workspace.Workspace.ListInRepo` — repo-scoped, per-entity-authoritative list

**Files:**
- Modify: `api/internal/app/repositories/workspace/workspace.go:150-152` (interface), and after `List` (currently ending at line 733)
- Test: `api/internal/app/repositories/workspace/workspace_test.go`

**Interfaces:**
- Produces: `workspace.Workspace.ListInRepo(ctx context.Context, projectID string, repoID string) ([]domain.Workspace, error)` — consumed by Task 3's `eligibilityFor`.
- Consumes: existing `w.locations.List(ctx)` (returns `[]locations.Location{ID, ProjectID, RepoID}`) and `w.readRow(ctx, loc)` (both already defined in `workspace.go`).

This method exists to fix the "broadcast amplification" bug: today, `repositories.Container.eligibilityFor` calls the full unscoped `List()` (opening every workspace's per-entity view.db in the whole install) just to find one sibling row, on every broadcast of a workspace that has a parent. `ListInRepo` reads from the exact same per-entity stores as `List()` (so it has identical consistency characteristics — no new eventual-consistency window), just skips entities outside the requested repo before paying the per-entity open cost.

- [ ] **Step 1: Write the failing tests**

Add to `api/internal/app/repositories/workspace/workspace_test.go` (same file, uses the existing `newRepo(t)` helper already defined at the top of this file):

```go
func TestListInRepo_ScopesToRepo(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, workspace.CreateInput{
		ID: "w1", ProjectID: "p1", RepoID: "r1", Branch: "main",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = repo.Create(ctx, workspace.CreateInput{
		ID: "w2", ProjectID: "p1", RepoID: "r2", Branch: "main",
	}, time.Unix(2, 0).UTC())
	require.NoError(t, err)
	_, err = repo.Create(ctx, workspace.CreateInput{
		ID: "w3", ProjectID: "p2", RepoID: "r1", Branch: "main",
	}, time.Unix(3, 0).UTC())
	require.NoError(t, err)

	rows, err := repo.ListInRepo(ctx, "p1", "r1")

	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "w1", rows[0].ID)
}

func TestListInRepo_NoMatchesReturnsEmpty(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.Create(ctx, workspace.CreateInput{
		ID: "w1", ProjectID: "p1", RepoID: "r1", Branch: "main",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)

	rows, err := repo.ListInRepo(ctx, "p1", "does-not-exist")

	require.NoError(t, err)
	assert.Empty(t, rows)
}
```

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run: `cd api && go test -tags noEmbed ./internal/app/repositories/workspace/... -run TestListInRepo -v`
Expected: FAIL — compile error, `repo.ListInRepo undefined (type workspace.Workspace has no field or method ListInRepo)`.

- [ ] **Step 3: Add `ListInRepo` to the `Workspace` interface**

In `api/internal/app/repositories/workspace/workspace.go`, change lines 150-152 from:

```go
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
```

to:

```go
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
	// ListInRepo returns every workspace scoped to one project+repo. It reads
	// from the same per-entity stores as List — just skipping entities outside
	// the requested repo before opening them — so it has identical read-after-
	// write consistency to List, only cheaper. Used by the merge-eligibility
	// broadcast overlay so a broadcast of a parented workspace no longer scans
	// the whole install to find its one sibling.
	ListInRepo(
		ctx context.Context,
		projectID string,
		repoID string,
	) ([]domain.Workspace, error)
```

- [ ] **Step 4: Implement `ListInRepo`**

In `api/internal/app/repositories/workspace/workspace.go`, immediately after the closing brace of `List` (the function ending at line 733), add:

```go
// ListInRepo returns every workspace scoped to projectID+repoID. It filters
// the (cheap, index-only) location list before acquiring any per-entity
// store, so it only opens entities inside the requested repo — unlike List,
// which opens every entity in the whole install.
func (w *workspace) ListInRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	locs, err := w.locations.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace: list in repo: list locations: %w", err)
	}
	rows := make([]domain.Workspace, 0, len(locs))
	for _, loc := range locs {
		if loc.ProjectID != projectID || loc.RepoID != repoID {
			continue
		}
		ws, err := w.readRow(ctx, loc)
		if err != nil {
			return nil, fmt.Errorf("workspace: list in repo: %w", err)
		}
		if ws == nil {
			continue
		}
		rows = append(rows, *ws)
	}
	return rows, nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd api && go test -tags noEmbed ./internal/app/repositories/workspace/... -run TestListInRepo -v`
Expected: PASS.

- [ ] **Step 6: Run the full package suite to confirm nothing else broke**

Run: `cd api && go test -tags noEmbed -race ./internal/app/repositories/... ./internal/app/...`
Expected: PASS. (`listErrWorkspaceRepo`/`errWorkspaceRepo` fakes in `container_test.go`/`snapshots_internal_test.go` embed `workspace.Workspace`, so the new interface method doesn't break their compilation.)

- [ ] **Step 7: Commit**

```bash
cd api
git add internal/app/repositories/workspace/workspace.go internal/app/repositories/workspace/workspace_test.go
git commit -m "feat(workspace): add repo-scoped ListInRepo"
```

---

### Task 2: `workspace_directory` projection package

**Files:**
- Create: `api/internal/app/repositories/workspace/directory/directory.go`
- Create: `api/internal/app/repositories/workspace/directory/directory_test.go`

**Interfaces:**
- Produces: `directory.Row`, `directory.Directory` (interface: `Upsert`, `Delete`, `ListByRepo`, `Rebuild`), `directory.New(db *gorm.DB) (Directory, error)` — consumed by Task 3.
- Consumes: `domain.Workspace` (existing), `storesqlite.OpenDB` (existing, used only in the test file to build an in-memory db).

- [ ] **Step 1: Write the failing test file**

Create `api/internal/app/repositories/workspace/directory/directory_test.go`:

```go
package directory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/directory"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newDirectory(
	t *testing.T,
) directory.Directory {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	d, err := directory.New(db)
	require.NoError(t, err)
	return d
}

func TestDirectory_UpsertAndListByRepo(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	ws := domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1", Branch: "main"}

	require.NoError(t, d.Upsert(ctx, ws))

	rows, err := d.ListByRepo(ctx, "p1", "r1")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "w1", rows[0].ID)
	assert.Equal(t, "main", rows[0].Branch)
}

func TestDirectory_ListByRepo_IsolatesOtherRepos(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1"}))
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w2", ProjectID: "p1", RepoID: "r2"}))

	rows, err := d.ListByRepo(ctx, "p1", "r1")

	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "w1", rows[0].ID)
}

func TestDirectory_ListByRepo_EmptyRepoID_MatchesWholeProject(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1"}))
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w2", ProjectID: "p1", RepoID: "r2"}))
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w3", ProjectID: "p2", RepoID: "r3"}))

	rows, err := d.ListByRepo(ctx, "p1", "")

	require.NoError(t, err)
	assert.Len(t, rows, 2)
}

func TestDirectory_ListByRepo_BlankScope_MatchesEverything(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1"}))
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w2", ProjectID: "p2", RepoID: "r2"}))

	rows, err := d.ListByRepo(ctx, "", "")

	require.NoError(t, err)
	assert.Len(t, rows, 2)
}

func TestDirectory_Upsert_OverwritesExistingRow(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1", Branch: "main"}))
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1", Branch: "renamed"}))

	rows, err := d.ListByRepo(ctx, "p1", "r1")

	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "renamed", rows[0].Branch)
}

func TestDirectory_Delete_RemovesRow(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1"}))

	require.NoError(t, d.Delete(ctx, "w1"))

	rows, err := d.ListByRepo(ctx, "p1", "r1")
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestDirectory_Delete_UnknownID_NoOp(t *testing.T) {
	d := newDirectory(t)
	require.NoError(t, d.Delete(context.Background(), "missing"))
}

func TestDirectory_Rebuild_ReplacesEntireTable(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "stale", ProjectID: "p1", RepoID: "r1"}))

	require.NoError(t, d.Rebuild(ctx, []domain.Workspace{
		{ID: "w1", ProjectID: "p1", RepoID: "r1"},
		{ID: "w2", ProjectID: "p1", RepoID: "r1"},
	}))

	rows, err := d.ListByRepo(ctx, "p1", "r1")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	ids := []string{rows[0].ID, rows[1].ID}
	assert.ElementsMatch(t, []string{"w1", "w2"}, ids)
	assert.NotContains(t, ids, "stale")
}

func TestDirectory_Rebuild_EmptyInput_ClearsTable(t *testing.T) {
	d := newDirectory(t)
	ctx := context.Background()
	require.NoError(t, d.Upsert(ctx, domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1"}))

	require.NoError(t, d.Rebuild(ctx, nil))

	rows, err := d.ListByRepo(ctx, "", "")
	require.NoError(t, err)
	assert.Empty(t, rows)
}
```

- [ ] **Step 2: Run the test file to verify it fails to compile**

Run: `cd api && go test -tags noEmbed ./internal/app/repositories/workspace/directory/... -v`
Expected: FAIL — compile error, e.g. `undefined: directory.New` (the production file doesn't exist yet).

- [ ] **Step 3: Write the production file**

Create `api/internal/app/repositories/workspace/directory/directory.go`:

```go
// Package directory provides a queryable, rebuildable projection of workspace
// rows scoped by project and repo. The workspace aggregate itself is
// event-sourced per entity (one event_stream.db + view.db per workspace, so
// deleting a workspace is a clean directory removal); that per-entity storage
// has no way to answer "every workspace in repo R" without opening every
// entity in the whole install. This package holds a denormalized copy of
// every workspace row, keyed for that one query, in the shared global
// view.db. The per-entity stores remain the sole source of truth — this
// table is derived and safe to wipe and Rebuild at any time.
package directory

import (
	"context"
	"encoding/json"
	"fmt"

	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Row is the GORM model backing the workspace_directory table: the indexed
// scope columns a query needs, plus the full workspace JSON-marshaled so a
// read never drops a field the domain type gains later.
type Row struct {
	ID        string `gorm:"primaryKey;column:id"`
	ProjectID string `gorm:"column:project_id;index:idx_workspace_directory_scope,priority:1"`
	RepoID    string `gorm:"column:repo_id;index:idx_workspace_directory_scope,priority:2"`
	ParentID  string `gorm:"column:parent_id;index"`
	Data      []byte `gorm:"column:data"`
}

// TableName pins the table name explicitly (GORM would otherwise pluralize
// Row to "rows", which collides with the SQL keyword).
func (Row) TableName() string {
	return "workspace_directory"
}

// Directory is the queryable, rebuildable workspace projection. The
// per-entity event-sourced store remains authoritative; Directory only serves
// list/tree/snapshot reads.
type Directory interface {
	// Upsert writes ws's current row. Called on every workspace broadcast.
	Upsert(ctx context.Context, ws domain.Workspace) error
	// Delete removes id's row. Called on a workspace's deleted tombstone.
	Delete(ctx context.Context, id string) error
	// ListByRepo returns every workspace matching projectID and repoID. An
	// empty component matches every value at that level (mirroring
	// api/v0's scopeWorkspacesToRepo semantics it replaces), so a
	// project-level or blank scope still returns the wider set.
	ListByRepo(ctx context.Context, projectID string, repoID string) ([]domain.Workspace, error)
	// Rebuild atomically replaces the entire table's contents with all. Used
	// once at Container construction to seed/reconcile the projection from a
	// full per-entity scan, and safe to call again anytime as a recovery
	// action since the table is fully derived.
	Rebuild(ctx context.Context, all []domain.Workspace) error
}

type gormDirectory struct {
	db *gormdb.DB
}

// New opens the workspace_directory table on db (the shared global view.db),
// auto-migrating its schema.
func New(
	db *gormdb.DB,
) (Directory, error) {
	if err := db.AutoMigrate(&Row{}); err != nil {
		return nil, fmt.Errorf("directory: migrate: %w", err)
	}
	return &gormDirectory{db: db}, nil
}

func toRow(
	ws domain.Workspace,
) (Row, error) {
	data, err := json.Marshal(ws)
	if err != nil {
		return Row{}, fmt.Errorf("directory: marshal: %w", err)
	}
	return Row{
		ID:        ws.ID,
		ProjectID: ws.ProjectID,
		RepoID:    ws.RepoID,
		ParentID:  ws.ParentID,
		Data:      data,
	}, nil
}

func fromRow(
	row Row,
) (domain.Workspace, error) {
	var ws domain.Workspace
	if err := json.Unmarshal(row.Data, &ws); err != nil {
		return domain.Workspace{}, fmt.Errorf("directory: unmarshal: %w", err)
	}
	return ws, nil
}

func fromRows(
	rows []Row,
) ([]domain.Workspace, error) {
	out := make([]domain.Workspace, 0, len(rows))
	for _, row := range rows {
		ws, err := fromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, ws)
	}
	return out, nil
}

func (g *gormDirectory) Upsert(
	ctx context.Context,
	ws domain.Workspace,
) error {
	row, err := toRow(ws)
	if err != nil {
		return err
	}
	if err := g.db.WithContext(ctx).Save(&row).Error; err != nil {
		return fmt.Errorf("directory: upsert: %w", err)
	}
	return nil
}

func (g *gormDirectory) Delete(
	ctx context.Context,
	id string,
) error {
	if err := g.db.WithContext(ctx).Where("id = ?", id).Delete(&Row{}).Error; err != nil {
		return fmt.Errorf("directory: delete: %w", err)
	}
	return nil
}

func (g *gormDirectory) ListByRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	q := g.db.WithContext(ctx)
	if projectID != "" {
		q = q.Where("project_id = ?", projectID)
	}
	if repoID != "" {
		q = q.Where("repo_id = ?", repoID)
	}
	var rows []Row
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("directory: list by repo: %w", err)
	}
	return fromRows(rows)
}

func (g *gormDirectory) Rebuild(
	ctx context.Context,
	all []domain.Workspace,
) error {
	return g.db.WithContext(ctx).Transaction(func(tx *gormdb.DB) error {
		if err := tx.Where("1 = 1").Delete(&Row{}).Error; err != nil {
			return fmt.Errorf("directory: rebuild: clear: %w", err)
		}
		for _, ws := range all {
			row, err := toRow(ws)
			if err != nil {
				return err
			}
			if err := tx.Create(&row).Error; err != nil {
				return fmt.Errorf("directory: rebuild: insert %s: %w", ws.ID, err)
			}
		}
		return nil
	})
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd api && go test -tags noEmbed -race ./internal/app/repositories/workspace/directory/... -v`
Expected: PASS — all `TestDirectory_*` tests pass.

- [ ] **Step 5: Commit**

```bash
cd api
git add internal/app/repositories/workspace/directory/
git commit -m "feat(workspace): add workspace_directory projection package"
```

---

### Task 3: Wire `directory` + `ListInRepo` into `repositories.Container`

**Files:**
- Modify: `api/internal/app/repositories/container.go`
- Test: `api/internal/app/repositories/container_test.go`, and a new `api/internal/app/repositories/directory_internal_test.go`

**Interfaces:**
- Consumes: `workspace.Workspace.ListInRepo` (Task 1), `directory.Directory`/`directory.New` (Task 2).
- Produces: `(*Container).ListWorkspacesInRepo(ctx, projectID, repoID string) ([]domain.Workspace, error)` — consumed by Task 4.

This task does NOT change `repositories.New`'s exported signature — `Directory` is constructed internally from the `adapters *adapter.Container` parameter `New` already receives, via `adapters.GlobalView()`. This means `container_test.go`'s existing `newContainer(t, h)` helper needs no changes.

**Why `eligibilityFor` uses `ListInRepo` (Task 1) and not the new `directory`:** `eligibilityFor` runs *inside* `broadcastWorkspace`, itself invoked by an async per-entity event-projection callback — the same mechanism that updates the per-entity store `List`/`ListInRepo` read from. Reading `ListInRepo` there keeps the exact same consistency behavior `eligibilityFor` has today (still reading live per-entity state, just scoped to fewer entities — no new race). The `directory` table, by contrast, is populated by that *same* callback but is consumed by *different* call sites (Task 4's REST/snapshot reads) that already tolerate the read model's existing eventual-consistency window today (see Task 4's notes) — using it inside `eligibilityFor` itself would add a cross-entity ordering dependency (sibling's directory row vs. this entity's own broadcast) that doesn't exist today and isn't needed.

- [ ] **Step 1: Write the failing tests**

Add to `api/internal/app/repositories/container_test.go` (uses the existing `newContainer`/`captureHub` helpers already defined in this file):

```go
func TestContainer_ListWorkspacesInRepo_ScopesToRepo(t *testing.T) {
	ctx := context.Background()
	c := newContainer(t, &captureHub{})

	_, err := c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", ProjectID: "p1", RepoID: "r1", Branch: "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	_, err = c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w2", ProjectID: "p1", RepoID: "r2", Branch: "b",
	}, time.Unix(2, 0).UTC())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		rows, listErr := c.ListWorkspacesInRepo(ctx, "p1", "r1")
		return listErr == nil && len(rows) == 1
	}, time.Second, 5*time.Millisecond)

	rows, err := c.ListWorkspacesInRepo(ctx, "p1", "r1")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "w1", rows[0].ID)
}

func TestContainer_BroadcastWorkspace_DeletedRemovesFromDirectory(t *testing.T) {
	ctx := context.Background()
	c := newContainer(t, &captureHub{})

	_, err := c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", ProjectID: "p1", RepoID: "r1", Branch: "b",
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		rows, listErr := c.ListWorkspacesInRepo(ctx, "p1", "r1")
		return listErr == nil && len(rows) == 1
	}, time.Second, 5*time.Millisecond)

	require.NoError(t, c.Workspace.Delete(ctx, "w1"))

	require.Eventually(t, func() bool {
		rows, listErr := c.ListWorkspacesInRepo(ctx, "p1", "r1")
		return listErr == nil && len(rows) == 0
	}, time.Second, 5*time.Millisecond)
}
```

Add a new internal (white-box) test file, `api/internal/app/repositories/directory_internal_test.go`, in `package repositories` so it can set the unexported `directory` field directly:

```go
package repositories

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type errDirectory struct{}

func (errDirectory) Upsert(context.Context, domain.Workspace) error { return nil }
func (errDirectory) Delete(context.Context, string) error           { return nil }
func (errDirectory) ListByRepo(context.Context, string, string) ([]domain.Workspace, error) {
	return nil, errFake
}
func (errDirectory) Rebuild(context.Context, []domain.Workspace) error { return nil }

func TestListWorkspacesInRepo_DirectoryErrorPropagates(t *testing.T) {
	c := &Container{directory: errDirectory{}, inflight: map[string]int{}}

	rows, err := c.ListWorkspacesInRepo(context.Background(), "p1", "r1")

	require.Error(t, err)
	assert.Nil(t, rows)
}
```

(`errFake` is already declared in `container_test.go` — but that file is package `repositories_test`, a *different* package from this new internal test file's `package repositories`, so it is not visible here. Declare a package-local sentinel instead: add `var errFake = errors.New("fake error")` at the top of `directory_internal_test.go` alongside the `"errors"` import, matching the existing style in `container_test.go`.)

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run: `cd api && go test -tags noEmbed ./internal/app/repositories/... -run 'TestContainer_ListWorkspacesInRepo|TestContainer_BroadcastWorkspace_DeletedRemovesFromDirectory|TestListWorkspacesInRepo_DirectoryErrorPropagates' -v`
Expected: FAIL — compile errors (`c.ListWorkspacesInRepo undefined`, `unknown field directory in struct literal`).

- [ ] **Step 3: Modify `container.go`**

In `api/internal/app/repositories/container.go`, change the import block (currently lines 1-18) from:

```go
import (
	"context"
	"fmt"
	"sync"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	wsusecase "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)
```

to:

```go
import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	workspacedir "github.com/char2cs/crowbar/api/internal/app/repositories/workspace/directory"
	wsusecase "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)
```

Change the `Container` struct from:

```go
type Container struct {
	Workspace    workspace.Workspace
	Chat         chat.Chat
	ReviewThread reviewthread.ReviewThread
	hub          hub.WebSocketHub
	git          wsusecase.MergeConflictChecker
	// inflight counts the background mutations currently running per workspace
	// id (00 §4 fail-fast/good-path-async). It backs the derived Working overlay:
	// the API layer brackets each async op with BeginWork/EndWork, and every
	// serving path (live broadcast, snapshot, REST reads) overlays IsWorking so
	// the client spinner tracks real daemon activity.
	mu       sync.Mutex
	inflight map[string]int
}
```

to:

```go
type Container struct {
	Workspace    workspace.Workspace
	Chat         chat.Chat
	ReviewThread reviewthread.ReviewThread
	hub          hub.WebSocketHub
	git          wsusecase.MergeConflictChecker
	// directory is the queryable, rebuildable workspace_directory projection
	// (workspace/directory package) backing ListWorkspacesInRepo and the
	// repo-scoped snapshot-on-subscribe builders. The per-entity Workspace
	// store remains authoritative; directory is a query-only convenience index.
	directory workspacedir.Directory
	// inflight counts the background mutations currently running per workspace
	// id (00 §4 fail-fast/good-path-async). It backs the derived Working overlay:
	// the API layer brackets each async op with BeginWork/EndWork, and every
	// serving path (live broadcast, snapshot, REST reads) overlays IsWorking so
	// the client spinner tracks real daemon activity.
	mu       sync.Mutex
	inflight map[string]int
}
```

Change `New` from:

```go
func New(
	ctx context.Context,
	adapters *adapter.Container,
	h hub.WebSocketHub,
	axChat asynx.Asynx[domain.Chat],
	axReviewThread asynx.Asynx[domain.ReviewThread],
	asynxFactory workspace.AsynxFactory,
	git wsusecase.MergeConflictChecker,
) (*Container, error) {
	c := &Container{hub: h, git: git, inflight: map[string]int{}}
	ws, err := workspace.New(adapters, func(ctx context.Context, w domain.Workspace) {
		c.broadcastWorkspace(ctx, w)
	}, asynxFactory)
	if err != nil {
		return nil, err
	}
	db := adapters.GlobalView()
	ch, err := chat.New(ctx, axChat, adapters.ChatES(), db, func(domain.Chat) {})
	if err != nil {
		return nil, err
	}
	rt, err := reviewthread.New(ctx, axReviewThread, adapters.ReviewThreadES(), db, func(domain.ReviewThread) {})
	if err != nil {
		return nil, err
	}
	c.Workspace = ws
	c.Chat = ch
	c.ReviewThread = rt
	return c, nil
}
```

to:

```go
func New(
	ctx context.Context,
	adapters *adapter.Container,
	h hub.WebSocketHub,
	axChat asynx.Asynx[domain.Chat],
	axReviewThread asynx.Asynx[domain.ReviewThread],
	asynxFactory workspace.AsynxFactory,
	git wsusecase.MergeConflictChecker,
) (*Container, error) {
	db := adapters.GlobalView()
	dir, err := workspacedir.New(db)
	if err != nil {
		return nil, fmt.Errorf("repositories: directory: %w", err)
	}
	c := &Container{hub: h, git: git, directory: dir, inflight: map[string]int{}}
	ws, err := workspace.New(adapters, func(ctx context.Context, w domain.Workspace) {
		c.broadcastWorkspace(ctx, w)
	}, asynxFactory)
	if err != nil {
		return nil, err
	}
	c.Workspace = ws
	c.rebuildDirectory(ctx)
	ch, err := chat.New(ctx, axChat, adapters.ChatES(), db, func(domain.Chat) {})
	if err != nil {
		return nil, err
	}
	rt, err := reviewthread.New(ctx, axReviewThread, adapters.ReviewThreadES(), db, func(domain.ReviewThread) {})
	if err != nil {
		return nil, err
	}
	c.Chat = ch
	c.ReviewThread = rt
	return c, nil
}

// rebuildDirectory repopulates the workspace_directory projection from a full
// per-entity scan. It runs once at container construction, covering both a
// fresh install (table empty) and an upgrade (table missing rows from before
// this projection existed). Best-effort: a failure here never fails Container
// construction — the per-entity stores remain the source of truth, and future
// broadcasts keep populating the table going forward regardless.
func (c *Container) rebuildDirectory(
	ctx context.Context,
) {
	rows, err := c.Workspace.List(ctx)
	if err != nil {
		slog.WarnContext(ctx, "repositories: rebuild directory: list", "err", err)
		return
	}
	if err := c.directory.Rebuild(ctx, rows); err != nil {
		slog.WarnContext(ctx, "repositories: rebuild directory", "err", err)
	}
}
```

Change `broadcastWorkspace` from:

```go
func (c *Container) broadcastWorkspace(
	ctx context.Context,
	ws domain.Workspace,
) {
	ws.Working = c.IsWorking(ws.ID)
	elig := c.eligibilityFor(ctx, ws)
	c.hub.BroadcastWorkspace(dto.WorkspaceDTOFrom(ws, elig))
}
```

to:

```go
func (c *Container) broadcastWorkspace(
	ctx context.Context,
	ws domain.Workspace,
) {
	ws.Working = c.IsWorking(ws.ID)
	c.syncDirectory(ctx, ws)
	elig := c.eligibilityFor(ctx, ws)
	c.hub.BroadcastWorkspace(dto.WorkspaceDTOFrom(ws, elig))
}

// syncDirectory keeps the workspace_directory projection in sync with every
// broadcasted row: deleted on the tombstone, upserted otherwise. Best-effort —
// a failure is logged and swallowed; the per-entity store already committed by
// the time this runs, so a projection write failure can never lose data, only
// cause a transient list omission until the next event or a rebuild.
func (c *Container) syncDirectory(
	ctx context.Context,
	ws domain.Workspace,
) {
	if ws.Status == domain.WorkspaceStatusDeleted {
		if err := c.directory.Delete(ctx, ws.ID); err != nil {
			slog.WarnContext(ctx, "repositories: directory delete", "workspace_id", ws.ID, "err", err)
		}
		return
	}
	if err := c.directory.Upsert(ctx, ws); err != nil {
		slog.WarnContext(ctx, "repositories: directory upsert", "workspace_id", ws.ID, "err", err)
	}
}
```

Change `eligibilityFor` from:

```go
func (c *Container) eligibilityFor(
	ctx context.Context,
	ws domain.Workspace,
) wsusecase.MergeEligibility {
	if ws.ParentID == "" {
		return wsusecase.MergeEligibility{}
	}
	rows, err := c.Workspace.List(ctx)
	if err != nil {
		return wsusecase.MergeEligibility{}
	}
	return wsusecase.ResolveMergeEligibility(ctx, ws, rows, c.git)
}
```

to:

```go
func (c *Container) eligibilityFor(
	ctx context.Context,
	ws domain.Workspace,
) wsusecase.MergeEligibility {
	if ws.ParentID == "" {
		return wsusecase.MergeEligibility{}
	}
	siblings, err := c.Workspace.ListInRepo(ctx, ws.ProjectID, ws.RepoID)
	if err != nil {
		return wsusecase.MergeEligibility{}
	}
	return wsusecase.ResolveMergeEligibility(ctx, ws, siblings, c.git)
}
```

Add, immediately after `ListWorkspaces` (the method ending at line 187):

```go
// ListWorkspacesInRepo returns every workspace row scoped to one repo, with
// the derived Working overlay applied, from the workspace_directory
// projection — a single indexed query instead of ListWorkspaces' full-install
// per-entity scan. It backs the repo-scoped snapshot-on-subscribe builders.
func (c *Container) ListWorkspacesInRepo(
	ctx context.Context,
	projectID string,
	repoID string,
) ([]domain.Workspace, error) {
	rows, err := c.directory.ListByRepo(ctx, projectID, repoID)
	if err != nil {
		return nil, fmt.Errorf("repositories: list workspaces in repo: %w", err)
	}
	for i := range rows {
		rows[i].Working = c.IsWorking(rows[i].ID)
	}
	return rows, nil
}
```

- [ ] **Step 4: Run the new tests to verify they pass**

Run: `cd api && go test -tags noEmbed -race ./internal/app/repositories/... -run 'TestContainer_ListWorkspacesInRepo|TestContainer_BroadcastWorkspace_DeletedRemovesFromDirectory|TestListWorkspacesInRepo_DirectoryErrorPropagates' -v`
Expected: PASS.

- [ ] **Step 5: Run the full existing repositories suite to confirm nothing regressed**

Run: `cd api && go test -tags noEmbed -race -count=10 ./internal/app/repositories/... -v`
Expected: PASS, including unchanged `TestBroadcastWorkspace_ResolvesMergeEligibility` (now backed by `ListInRepo` instead of `List`, same consistency behavior) and `TestContainer_ListWorkspaces_ListErrorPropagates` (unaffected — still exercises `ListWorkspaces`/`c.Workspace.List` directly). The `-count=10` repeat is deliberate: this task introduces the projection's async-broadcast-driven write, and a single run is not enough to catch a rare ordering flake if one exists.

If `-count=10` reveals a flake in `TestContainer_ListWorkspacesInRepo_ScopesToRepo` or `TestContainer_BroadcastWorkspace_DeletedRemovesFromDirectory` specifically (not in the pre-existing eligibility test): both of those new tests already use `require.Eventually` for exactly this reason (the directory, like the per-entity read model it mirrors in timing, is populated by the same async broadcast callback) — increase the `require.Eventually` timeout from `time.Second` to `3 * time.Second` first; if it still flakes, that indicates the projection write needs to move earlier in the pipeline (into `workspace.go`'s entity-apply path, alongside `entity.store`'s own update) rather than staying in `broadcastWorkspace` — escalate rather than silently loosening the test further.

- [ ] **Step 6: Commit**

```bash
cd api
git add internal/app/repositories/container.go internal/app/repositories/container_test.go internal/app/repositories/directory_internal_test.go
git commit -m "feat(repositories): wire workspace_directory projection into Container"
```

---

### Task 4: Scope the WS snapshot builders to the new repo-scoped list

**Files:**
- Modify: `api/internal/api/v0/snapshots.go`
- Modify: `api/internal/api/v0/snapshots_internal_test.go`

**Interfaces:**
- Consumes: `(*repositories.Container).ListWorkspacesInRepo` (Task 3), existing `(*repositories.Container).ListWorkspaces`, existing `parseRepoScope`.

`workspacesSnapshot`'s scope (from `workspacesDef`'s hierarchical `Namespace`, matched via prefix) is always a `"p"`/`"p/r"`/`"p/r/w"` string — `parseRepoScope` already parses it directly into `projectID`/`repoID`. `gitSnapshot`/`lspSnapshot`'s scope (from `gitDef`/`lspDef`'s `ScopeKey = scopeWsID`) is a single **workspace id**, not a hierarchical string — passing it to `parseRepoScope` would be wrong (it would split a raw id on `/`, which workspace ids don't contain, yielding garbage). Those two resolve the scope's owning workspace first, then list that workspace's repo siblings; a blank scope (a list-level subscribe, not currently used by these two broadcasters but defensively supported) falls back to the existing unscoped `ListWorkspaces`.

- [ ] **Step 1: Write the failing/updated tests**

In `api/internal/api/v0/snapshots_internal_test.go`:

1. Delete `TestWorkspacesSnapshot_ListErrorReturnsNil` (lines 71-75) entirely. Its premise — forcing `a.Repositories.Workspace` to error so the snapshot degrades to nil — no longer applies: `workspacesSnapshot` now reads via `ListWorkspacesInRepo` (the `directory`), not `Workspace.List`. The equivalent "list error degrades to nil" behavior is now covered at the `repositories` package level by Task 3's `TestListWorkspacesInRepo_DirectoryErrorPropagates`, plus `workspacesSnapshot`'s own `if err != nil { return nil }` branch (unchanged code, still present).

2. Add these new tests to the same file:

```go
// TestGitSnapshot_ScopedToWorkspaceRepo proves gitSnapshot resolves the
// subscribing workspace id to its repo and only touches that repo's
// workspaces — not every workspace in the install.
func TestGitSnapshot_ScopedToWorkspaceRepo(t *testing.T) {
	a := newAppForSnapshot(t)
	seedWorkspace(t, a, "w1", "p1", "r1", "", "")
	seedWorkspace(t, a, "w2", "p2", "r2", "", "")

	got := gitSnapshot(a)("w1")

	ids := make([]string, len(got))
	for i, e := range got {
		ids[i] = e.WsID
	}
	assert.Contains(t, ids, "w1")
	assert.NotContains(t, ids, "w2")
}

func TestLSPSnapshot_ScopedToWorkspaceRepo(t *testing.T) {
	a := newAppForSnapshot(t)
	seedWorkspace(t, a, "w1", "p1", "r1", "", "")
	seedWorkspace(t, a, "w2", "p2", "r2", "", "")
	eng, err := engine.New(context.Background())
	require.NoError(t, err)

	assert.NotPanics(t, func() { lspSnapshot(a, eng)("w1") })
}

func TestGitSnapshot_UnknownWorkspaceScope_ReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	assert.Nil(t, gitSnapshot(a)("does-not-exist"))
}
```

- [ ] **Step 2: Run the tests to verify the new ones fail**

Run: `cd api && go test -tags noEmbed ./internal/api/v0/... -run 'TestGitSnapshot_ScopedToWorkspaceRepo|TestLSPSnapshot_ScopedToWorkspaceRepo|TestGitSnapshot_UnknownWorkspaceScope_ReturnsNil' -v`
Expected: FAIL — `gitSnapshot(a)("w1")` still returns status for every workspace in the install (assertion on `w2`'s absence fails), since production code hasn't changed yet.

- [ ] **Step 3: Modify `workspacesSnapshot`**

In `api/internal/api/v0/snapshots.go`, change (lines 23-42) from:

```go
func workspacesSnapshot(
	appContainer *app.Container,
) func(scope string) []dto.WorkspaceDTO {
	return func(scope string) []dto.WorkspaceDTO {
		projectID, repoID := parseRepoScope(scope)
		rows, err := appContainer.Repositories.ListWorkspaces(context.Background())
		if err != nil {
			return nil
		}
		siblings := scopeWorkspacesToRepo(rows, projectID, repoID)
		// Snapshot-on-subscribe has no request to scope to (it's built lazily for
		// a connecting client), so it owns a background context — the same one it
		// already uses for the List above. The detached context is a visible,
		// edge-level choice here, not hidden inside the usecase.
		eligFn := func(w domain.Workspace) workspace.MergeEligibility {
			return appContainer.Usecases.Workspace.MergeEligibilityFor(context.Background(), w, siblings)
		}
		return dto.WorkspaceDTOList(siblings, eligFn)
	}
}
```

to:

```go
func workspacesSnapshot(
	appContainer *app.Container,
) func(scope string) []dto.WorkspaceDTO {
	return func(scope string) []dto.WorkspaceDTO {
		ctx := context.Background()
		projectID, repoID := parseRepoScope(scope)
		siblings, err := appContainer.Repositories.ListWorkspacesInRepo(ctx, projectID, repoID)
		if err != nil {
			return nil
		}
		// Snapshot-on-subscribe has no request to scope to (it's built lazily for
		// a connecting client), so it owns a background context — the same one it
		// already uses for the List above. The detached context is a visible,
		// edge-level choice here, not hidden inside the usecase.
		eligFn := func(w domain.Workspace) workspace.MergeEligibility {
			return appContainer.Usecases.Workspace.MergeEligibilityFor(context.Background(), w, siblings)
		}
		return dto.WorkspaceDTOList(siblings, eligFn)
	}
}
```

Delete `scopeWorkspacesToRepo` (the function at lines 63-84) — it is now unused (its only caller was the code just replaced). Leave `parseRepoScope` and its doc comment untouched (still used, above).

- [ ] **Step 4: Modify `gitSnapshot` and `lspSnapshot`**

In `api/internal/api/v0/snapshots.go`, add this helper immediately before `gitSnapshot` (which starts at line 172):

```go
// scopedWorkspaceRows resolves scope to the workspaces gitSnapshot/lspSnapshot
// should cover. Both broadcasters scope their WS subscriptions by a single
// workspace id (scopeWsID in container.go), not a "p/r" hierarchical prefix —
// unlike workspacesSnapshot's scope. So scope here is resolved to its owning
// workspace's repo, and every workspace in that same repo is returned; the
// broadcaster's own wsId predicate filters delivery down to the connecting
// client's exact workspace afterward, exactly as it already does today. A
// blank scope (a list-level subscribe — not currently used by either
// broadcaster, but handled defensively) falls back to every workspace. An
// unresolvable scope (unknown workspace id) yields no rows rather than an
// error, since a snapshot degrading to empty is safe and a stale/racing
// subscribe for an already-deleted workspace is expected, not exceptional.
func scopedWorkspaceRows(
	ctx context.Context,
	appContainer *app.Container,
	scope string,
) ([]domain.Workspace, error) {
	if scope == "" {
		return appContainer.Repositories.ListWorkspaces(ctx)
	}
	ws, err := appContainer.Repositories.Workspace.Get(ctx, scope)
	if err != nil {
		return nil, nil
	}
	return appContainer.Repositories.ListWorkspacesInRepo(ctx, ws.ProjectID, ws.RepoID)
}
```

Change `gitSnapshot` from:

```go
func gitSnapshot(
	appContainer *app.Container,
) func(scope string) []gitdomain.GitStatusEvent {
	return func(_ string) []gitdomain.GitStatusEvent {
		ctx := context.Background()
		rows, err := appContainer.Repositories.Workspace.List(ctx)
		if err != nil {
			return nil
		}
		events := make([]gitdomain.GitStatusEvent, 0, len(rows))
		for _, row := range rows {
			events = appendGitStatus(ctx, appContainer, events, row.ID)
		}
		return events
	}
}
```

to:

```go
func gitSnapshot(
	appContainer *app.Container,
) func(scope string) []gitdomain.GitStatusEvent {
	return func(scope string) []gitdomain.GitStatusEvent {
		ctx := context.Background()
		rows, err := scopedWorkspaceRows(ctx, appContainer, scope)
		if err != nil {
			return nil
		}
		events := make([]gitdomain.GitStatusEvent, 0, len(rows))
		for _, row := range rows {
			events = appendGitStatus(ctx, appContainer, events, row.ID)
		}
		return events
	}
}
```

Change `lspSnapshot` from:

```go
func lspSnapshot(
	appContainer *app.Container,
	engContainer *engine.Container,
) func(scope string) []lspdomain.DiagnosticsEvent {
	if engContainer == nil || engContainer.LSP == nil {
		return nil
	}
	return func(_ string) []lspdomain.DiagnosticsEvent {
		ctx := context.Background()
		rows, err := appContainer.Repositories.Workspace.List(ctx)
		if err != nil {
			return nil
		}
		events := make([]lspdomain.DiagnosticsEvent, 0, len(rows))
		for _, row := range rows {
			events = appendDiagnostics(engContainer, events, row.ID)
		}
		return events
	}
}
```

to:

```go
func lspSnapshot(
	appContainer *app.Container,
	engContainer *engine.Container,
) func(scope string) []lspdomain.DiagnosticsEvent {
	if engContainer == nil || engContainer.LSP == nil {
		return nil
	}
	return func(scope string) []lspdomain.DiagnosticsEvent {
		ctx := context.Background()
		rows, err := scopedWorkspaceRows(ctx, appContainer, scope)
		if err != nil {
			return nil
		}
		events := make([]lspdomain.DiagnosticsEvent, 0, len(rows))
		for _, row := range rows {
			events = appendDiagnostics(engContainer, events, row.ID)
		}
		return events
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd api && go test -tags noEmbed ./internal/api/v0/... -run 'TestGitSnapshot|TestLSPSnapshot|TestWorkspacesSnapshot' -v`
Expected: PASS, including the pre-existing `TestGitSnapshot_ListErrorReturnsNil`/`TestLSPSnapshot_ListErrorReturnsNil` (scope `""` still routes through `ListWorkspaces`→`Workspace.List`, so `errWorkspaceRepo` still triggers the error path) and `TestGitSnapshot_BadWorktreeSkipsWorkspace`/`TestLSPSnapshot_NoDiagnosticsIsEmpty` (also scope `""`).

- [ ] **Step 6: Run the full v0 unit + integration suites**

Run: `cd api && go build -tags noEmbed ./... && go test -tags noEmbed -race ./internal/api/v0/... -v`
Expected: PASS, and confirm `scopeWorkspacesToRepo` is gone with no remaining references (`grep -rn scopeWorkspacesToRepo api/internal` returns nothing).

Run: `cd api && go test -tags 'integration noEmbed' -race -v -p 1 ./internal/api/v0/...`
Expected: PASS — this exercises the real WS snapshot-on-subscribe path end to end (`snapshots_test.go`), including the LSP diagnostics snapshot test using `seededLSP`.

- [ ] **Step 7: Commit**

```bash
cd api
git add internal/api/v0/snapshots.go internal/api/v0/snapshots_internal_test.go
git commit -m "feat(api): scope git/lsp/workspaces snapshot-on-subscribe to the requesting repo"
```

---

### Task 5: Raise the global view.db connection pool

**Files:**
- Modify: `api/internal/adapter/store/sqlite/sqlite.go`
- Modify: `api/internal/adapter/container.go:128`
- Test: `api/internal/adapter/store/sqlite/sqlite_internal_test.go`

**Interfaces:**
- Produces: `sqlite.OpenDBWithPool(path string, maxOpenConns int) (*gorm.DB, error)` (new); `sqlite.OpenDB` keeps its existing signature and single-connection behavior for per-entity workspace DBs (unchanged callers: `adapter/container.go:230` for `WorkspaceView`, and every existing test using `OpenDB`/`sqlite.New`).

- [ ] **Step 1: Write the failing test**

Add to `api/internal/adapter/store/sqlite/sqlite_internal_test.go`:

```go
func TestOpenDBWithPool_SetsMaxOpenConns(t *testing.T) {
	db, err := OpenDBWithPool(":memory:", 8)
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	assert.Equal(t, 8, sqlDB.Stats().MaxOpenConnections)
}

func TestOpenDB_StillSingleConnection(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	assert.Equal(t, 1, sqlDB.Stats().MaxOpenConnections)
}
```

- [ ] **Step 2: Run the test to verify it fails to compile**

Run: `cd api && go test -tags noEmbed ./internal/adapter/store/sqlite/... -run TestOpenDBWithPool -v`
Expected: FAIL — `undefined: OpenDBWithPool`.

- [ ] **Step 3: Refactor `sqlite.go`**

In `api/internal/adapter/store/sqlite/sqlite.go`, change `OpenDB` (lines 33-57) from:

```go
// OpenDB opens (or creates) a single-connection SQLite database at path.
// WAL journal mode and a 5-second busy timeout are enabled so that a second
// opener (e.g. in crash-recovery tests) does not get SQLITE_BUSY on DDL.
func OpenDB(
	path string,
) (*gorm.DB, error) {
	db, err := gorm.Open(glebarez.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("sqlite: db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		return nil, fmt.Errorf("sqlite: journal_mode: %w", err)
	}
	if err := db.Exec("PRAGMA busy_timeout=5000").Error; err != nil {
		return nil, fmt.Errorf("sqlite: busy_timeout: %w", err)
	}
	return db, nil
}
```

to:

```go
// OpenDB opens (or creates) a single-connection SQLite database at path.
// WAL journal mode and a 5-second busy timeout are enabled so that a second
// opener (e.g. in crash-recovery tests) does not get SQLITE_BUSY on DDL. Used
// for the per-entity workspace databases, which are effectively single-tenant
// (one workspace, one or two open tabs at most).
func OpenDB(
	path string,
) (*gorm.DB, error) {
	return OpenDBWithPool(path, 1)
}

// OpenDBWithPool opens (or creates) a SQLite database at path with WAL journal
// mode, a 5-second busy timeout, and up to maxOpenConns open connections. WAL
// mode allows one writer plus many concurrent readers at the SQLite level;
// maxOpenConns controls how many of those concurrent readers the Go
// connection pool actually allows through at once — use a value greater than
// 1 for a database that serves concurrent read-heavy traffic (the global
// view.db), and 1 (via OpenDB) for a single-tenant per-entity database.
func OpenDBWithPool(
	path string,
	maxOpenConns int,
) (*gorm.DB, error) {
	db, err := gorm.Open(glebarez.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("sqlite: db: %w", err)
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		return nil, fmt.Errorf("sqlite: journal_mode: %w", err)
	}
	if err := db.Exec("PRAGMA busy_timeout=5000").Error; err != nil {
		return nil, fmt.Errorf("sqlite: busy_timeout: %w", err)
	}
	return db, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd api && go test -tags noEmbed ./internal/adapter/store/sqlite/... -run 'TestOpenDBWithPool|TestOpenDB_StillSingleConnection' -v`
Expected: PASS.

- [ ] **Step 5: Raise the pool for the global view.db**

In `api/internal/adapter/container.go`, add the constant near the other per-entity constants (lines 19-23):

```go
// per-entity DB file names (siblings inside a storages/ directory).
const (
	eventStreamDBName = "event_stream.db"
	viewDBName        = "view.db"
)

// globalViewMaxOpenConns bounds the global view.db's connection pool. WAL mode
// (set by storesqlite.OpenDBWithPool) already allows concurrent readers with
// one serialized writer; before this, SetMaxOpenConns(1) forced every reader
// and writer through a single Go-level connection regardless, serializing
// even concurrent reads of the now read-heavy workspace_directory projection.
const globalViewMaxOpenConns = 8
```

Change line 128 from:

```go
	globalView, err := storesqlite.OpenDB(filepath.Join(stateDir, viewDBName))
```

to:

```go
	globalView, err := storesqlite.OpenDBWithPool(filepath.Join(stateDir, viewDBName), globalViewMaxOpenConns)
```

- [ ] **Step 6: Run the full adapter suite**

Run: `cd api && go test -tags noEmbed -race ./internal/adapter/... -v`
Expected: PASS. (No existing test pins the global view.db to exactly 1 connection — `TestGlobalView_HoldsProfilesAndSettings` and friends in `adapter/container_test.go` only assert behavior, not pool size — so this is a behavior-preserving change to verify, not a test to update.)

- [ ] **Step 7: Commit**

```bash
cd api
git add internal/adapter/store/sqlite/sqlite.go internal/adapter/store/sqlite/sqlite_internal_test.go internal/adapter/container.go
git commit -m "perf(adapter): raise the global view.db connection pool to 8"
```

---

### Task 6: Git engine `RWMutex` — reads vs. writes

**Files:**
- Modify: `api/internal/engine/git/engine.go`
- Modify: `api/internal/engine/git/export_test.go`
- Test: new `api/internal/engine/git/rwmutex_internal_test.go`

**Interfaces:**
- Produces (test-only, in `export_test.go`): `git.NewWithExec(exec func(ctx context.Context, dir string, args ...string) gitexec.Result) Engine`.

**Reentrancy hazard found during planning (must be fixed as part of this task, not just the lock type):** three call sites call one locking method from inside another already-locked method, which would deadlock once `Status`/`ComputeStatus`/`ComputeWorkingTreeSummary`/`WorkingTreeSummary` start taking `RLock` (Go's `sync.RWMutex` is not reentrant — `Lock` then `RLock` on the same goroutine deadlocks immediately, and `RLock` then `RLock` again on the same goroutine can deadlock if a writer is queued in between):
1. `Discard` (holds `Lock`) calls `e.Status(...)` at `engine.go:313`.
2. `ComputeStatus` calls `e.Status(...)` at `engine.go:601` — both would take `RLock`.
3. `ComputeWorkingTreeSummary` calls `e.WorkingTreeSummary(...)` at `engine.go:610`, which itself calls `e.computeWorkingTreeSummary(...)` — both `ComputeWorkingTreeSummary` and `WorkingTreeSummary` would take `RLock`.

All three are fixed below by having the inner call go directly to the shared unlocked implementation instead of through the sibling public method.

- [ ] **Step 1: Write the failing tests**

Create `api/internal/engine/git/rwmutex_internal_test.go`:

```go
package git_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// blockingExec returns an execFn-compatible function that answers
// "rev-parse --git-common-dir" immediately (so common-dir resolution, which
// runs before any lock is taken, never blocks) and otherwise signals on
// started before blocking on release — letting a test control exactly when a
// git subprocess "returns" without sleeping.
func blockingExec(
	started chan<- string,
	release <-chan struct{},
) func(ctx context.Context, dir string, args ...string) gitexec.Result {
	return func(_ context.Context, dir string, args ...string) gitexec.Result {
		if len(args) > 0 && args[0] == "rev-parse" {
			return gitexec.Result{ExitCode: 0, Stdout: dir}
		}
		started <- args[0]
		<-release
		return gitexec.Result{ExitCode: 0}
	}
}

func TestRWMutex_ConcurrentReadsDoNotSerialize(t *testing.T) {
	dir := t.TempDir()
	started := make(chan string, 2)
	release := make(chan struct{})
	e := git.NewWithExec(blockingExec(started, release))
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			_, _ = e.WouldMergeConflict(ctx, dir, "a", "b")
		}()
	}

	// Both reads must reach their blocking exec call — proving neither waited
	// for the other to finish (both hold RLock concurrently).
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("both concurrent reads did not start within 1s — a read is blocking on another read's RLock")
		}
	}
	close(release)
	wg.Wait()
}

func TestRWMutex_WriteBlocksConcurrentRead(t *testing.T) {
	dir := t.TempDir()
	started := make(chan string, 1)
	release := make(chan struct{})
	e := git.NewWithExec(blockingExec(started, release))
	ctx := context.Background()

	writeDone := make(chan struct{})
	go func() {
		_ = e.Commit(ctx, dir, "subject", "")
		close(writeDone)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("write never reached its exec call")
	}

	readDone := make(chan struct{})
	go func() {
		_, _ = e.WouldMergeConflict(ctx, dir, "a", "b")
		close(readDone)
	}()

	// The read must NOT complete while the write still holds the lock.
	select {
	case <-readDone:
		t.Fatal("read completed while a write held the lock")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	<-writeDone
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("read did not proceed after the write released the lock")
	}
}

func TestRWMutex_DiscardDoesNotDeadlockOnStatus(t *testing.T) {
	dir := initRepo(t)
	makeCommit(t, dir, "file.txt", "content\n", "init")
	require.NoError(t, osWriteFile(dir, "file.txt", "changed\n"))

	e := git.New()
	done := make(chan error, 1)
	go func() { done <- e.Discard(context.Background(), dir, "file.txt") }()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Discard deadlocked (Status must not re-take the lock Discard already holds)")
	}
}
```

Add this small helper to `engine_test.go` (it already imports `"os"` and `"path/filepath"`) rather than `rwmutex_internal_test.go`, so both files can share it — append at the end of `engine_test.go`:

```go
func osWriteFile(
	dir string,
	name string,
	content string,
) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
}
```

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run: `cd api && go test -tags noEmbed ./internal/engine/git/... -run TestRWMutex -v`
Expected: FAIL — `undefined: git.NewWithExec`.

- [ ] **Step 3: Add the test-only constructor**

In `api/internal/api/v0/../../engine/git/export_test.go` (i.e. `api/internal/engine/git/export_test.go`), add:

```go
// NewWithExec builds an engine over a fake exec function, for white-box
// concurrency tests that need to control command timing without a real git
// subprocess.
func NewWithExec(
	exec func(ctx context.Context, dir string, args ...string) gitexec.Result,
) Engine {
	return &engine{exec: exec}
}
```

- [ ] **Step 4: Change the mutex type and add the read-lock helper**

In `api/internal/engine/git/engine.go`, change the `engine` struct (lines 28-33) from:

```go
type engine struct {
	exec      execFn
	execStdin execStdinFn
	mu        sync.Map
	commonDir sync.Map
}
```

to:

```go
type engine struct {
	exec      execFn
	execStdin execStdinFn
	mu        sync.Map // key: common dir (string) -> *sync.RWMutex
	commonDir sync.Map
}
```

Change `repoMutex` and `lockRepo` (lines 40-44 and 74-78) from:

```go
func (e *engine) repoMutex(ctx context.Context, repoPath string) *sync.Mutex {
	key := e.resolveCommonDir(ctx, repoPath)
	actual, _ := e.mu.LoadOrStore(key, &sync.Mutex{})
	return actual.(*sync.Mutex)
}
```

and

```go
func (e *engine) lockRepo(ctx context.Context, repoPath string) func() {
	mu := e.repoMutex(ctx, repoPath)
	mu.Lock()
	return mu.Unlock
}
```

to:

```go
func (e *engine) repoMutex(ctx context.Context, repoPath string) *sync.RWMutex {
	key := e.resolveCommonDir(ctx, repoPath)
	actual, _ := e.mu.LoadOrStore(key, &sync.RWMutex{})
	return actual.(*sync.RWMutex)
}
```

and

```go
// lockRepo takes the exclusive write lock for repoPath's clone, for any
// operation that mutates the working tree, index, refs, or touches the
// network (07 §3.1).
func (e *engine) lockRepo(ctx context.Context, repoPath string) func() {
	mu := e.repoMutex(ctx, repoPath)
	mu.Lock()
	return mu.Unlock
}

// lockRepoRead takes the shared read lock for repoPath's clone, for any
// read-only inspection (status/diff/log/…). Concurrent reads never block each
// other; a read blocks only while a write (including a background origin-sync
// fetch) holds the exclusive lock, and is guaranteed to observe either the
// fully-pre- or fully-post-mutation state, never a torn one.
func (e *engine) lockRepoRead(ctx context.Context, repoPath string) func() {
	mu := e.repoMutex(ctx, repoPath)
	mu.RLock()
	return mu.RUnlock
}
```

- [ ] **Step 5: Add `RLock` to the read-only methods**

In `api/internal/engine/git/engine.go`, change each of the following from (no lock) to (`defer e.lockRepoRead(ctx, repoPath)()` as the first line of the function body):

```go
func (e *engine) Status(
	ctx context.Context,
	repoPath string,
) (gitdomain.GitStatus, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return status.Parse(ctx, repoPath)
}

func (e *engine) Diff(
	ctx context.Context,
	repoPath string,
	staged bool,
) ([]gitdomain.FileDiff, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return diff.WorkingTree(ctx, repoPath, staged)
}

func (e *engine) CommitDiff(
	ctx context.Context,
	repoPath string,
	sha string,
) (gitdomain.MultiFileDiff, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return diff.Commit(ctx, repoPath, sha)
}

func (e *engine) Log(
	ctx context.Context,
	repoPath string,
	limit int,
	skip int,
) ([]gitdomain.Commit, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return gitlog.List(ctx, repoPath, limit, skip)
}

func (e *engine) Blame(
	ctx context.Context,
	repoPath string,
	filePath string,
) ([]gitdomain.BlameEntry, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return blame.File(ctx, repoPath, filePath)
}

func (e *engine) Branches(
	ctx context.Context,
	repoPath string,
) ([]gitdomain.Branch, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return branches.List(ctx, repoPath)
}

func (e *engine) Stashes(
	ctx context.Context,
	repoPath string,
) ([]gitdomain.Stash, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return stash.List(ctx, repoPath)
}
```

```go
func (e *engine) ConflictedFiles(
	ctx context.Context,
	repoPath string,
) ([]string, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return conflicts.ConflictedFiles(ctx, repoPath)
}

func (e *engine) ConflictHunks(
	ctx context.Context,
	repoPath string,
	filePath string,
) ([]gitdomain.ConflictHunk, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return conflicts.ParseFile(ctx, repoPath, filePath)
}
```

```go
func (e *engine) WorkingTreeSummary(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, bool, bool, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return e.computeWorkingTreeSummary(ctx, repoPath, forkPointSha)
}

// ComputeStatus implements watch.GitStatusProvider.
func (e *engine) ComputeStatus(
	ctx context.Context,
	repoPath string,
) (gitdomain.GitStatus, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return status.Parse(ctx, repoPath)
}

// ComputeWorkingTreeSummary implements watch.GitStatusProvider.
func (e *engine) ComputeWorkingTreeSummary(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, bool, bool, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	return e.computeWorkingTreeSummary(ctx, repoPath, forkPointSha)
}
```

Note the reentrancy fixes baked into the block above: `WorkingTreeSummary` and `ComputeWorkingTreeSummary` now both call `e.computeWorkingTreeSummary` (the unlocked private helper) directly instead of one calling the other's public wrapper, and `ComputeStatus` now calls `status.Parse` directly instead of `e.Status`. Each public method takes its own independent `RLock` and never calls a sibling locking method.

In `api/internal/engine/git/would_merge_conflict.go`, change `WouldMergeConflict` from:

```go
func (e *engine) WouldMergeConflict(
	ctx context.Context,
	repoPath string,
	ours string,
	theirs string,
) (bool, error) {
	r := e.exec(ctx, repoPath, "merge-tree", "--write-tree", ours, theirs)
```

to:

```go
func (e *engine) WouldMergeConflict(
	ctx context.Context,
	repoPath string,
	ours string,
	theirs string,
) (bool, error) {
	defer e.lockRepoRead(ctx, repoPath)()
	r := e.exec(ctx, repoPath, "merge-tree", "--write-tree", ours, theirs)
```

- [ ] **Step 6: Fix the `Discard` reentrancy hazard**

In `api/internal/engine/git/engine.go`, `Discard` (lines 307-327) calls `e.Status(ctx, repoPath)` while already holding the exclusive `Lock` — since `Status` now takes `RLock`, this would deadlock (a goroutine cannot hold `Lock` and also take `RLock` on the same `sync.RWMutex`). Change:

```go
func (e *engine) Discard(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	st, err := e.Status(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("git: discard: status: %w", err)
	}
```

to:

```go
func (e *engine) Discard(
	ctx context.Context,
	repoPath string,
	filePath string,
) error {
	defer e.lockRepo(ctx, repoPath)()
	// Call status.Parse directly, NOT e.Status: e.Status now takes RLock, and
	// this method already holds the exclusive Lock — Go's sync.RWMutex is not
	// reentrant, so going through e.Status here would deadlock.
	st, err := status.Parse(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("git: discard: status: %w", err)
	}
```

(`status` is already imported in `engine.go` — it backs the existing `Status` method.)

- [ ] **Step 7: Run the new tests to verify they pass**

Run: `cd api && go test -tags noEmbed -race ./internal/engine/git/... -run TestRWMutex -v`
Expected: PASS.

- [ ] **Step 8: Run the full git engine suite, repeated, under the race detector**

Run: `cd api && go test -tags noEmbed -race -count=5 ./internal/engine/git/... -v`
Expected: PASS — no deadlocks, no data races, every pre-existing test (`engine_test.go`, `ops_test.go`, `internal_test.go`, `merge_squash_test.go`, etc.) still green.

- [ ] **Step 9: Run the full backend suite**

Run: `cd api && go build -tags noEmbed ./... && go test -tags noEmbed -race ./...`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
cd api
git add internal/engine/git/engine.go internal/engine/git/would_merge_conflict.go internal/engine/git/export_test.go internal/engine/git/engine_test.go internal/engine/git/rwmutex_internal_test.go
git commit -m "fix(git): split the per-repo lock into RWMutex so reads don't block on writes"
```
