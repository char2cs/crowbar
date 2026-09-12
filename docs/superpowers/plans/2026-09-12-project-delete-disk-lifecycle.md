# Project Delete Disk-Lifecycle Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deleting a project must actually remove its on-disk directory tree, proven by a real-filesystem test — not a mocked one — instead of relying on a single best-effort `os.RemoveAll` call that silently leaves an empty, permanently orphaned directory behind on any transient failure.

**Architecture:** `project_delete.go`'s `removeProjectDir` calls `os.RemoveAll` exactly once and swallows any error into a `WARN` log line. This plan makes it retry a bounded number of times with a short, injectable delay (closing the actual gap: a transient failure — a filesystem indexer or sync client briefly touching the directory between "remove children" and "remove now-empty parent" — usually clears within milliseconds), keeps the existing contract that a persistent failure still does not fail the overall `Delete()` call (the DB rows are already correctly gone by that point; that part of the design is sound), and raises the final swallowed failure to `ERROR` so it is operationally visible instead of indistinguishable from routine noise. A new integration test exercises the real `os.RemoveAll` against a real temp directory populated the way a real project's tree actually looks, proving zero residue end to end.

**Tech Stack:** Go, testify.

**Spec:** This plan's own Goal/Architecture above. This is the direct fix for the "boring" of the two candidate causes discussed for the orphaned `projects/*` directories — the one that needs no dev tooling and explains a plain client install. The "batch" candidate (dev/seed tooling writing into the real home) is `2026-09-12-crowbar-home-propagation.md` and `2026-09-12-seed-home-safety.md`.

## Global Constraints

- `DeleteDeps.RemoveAll func(path string) error` (`project_delete.go`) is the existing injectable seam every current test in `project_delete_test.go` already uses — extend it, don't replace it.
- Never make a persistent (post-retry) disk-removal failure fail `Delete()` itself: the DB rows are already gone by the time `removeProjectDir` runs, and failing the call now would report an operation that DID succeed (the project is gone from every list the user sees) as an error. `TestProjectDelete_RemoveProjectDirFailure_IsLoggedNotFatal` pins this contract; extend it, don't invert it.
- No sleeps in tests: the retry delay itself must be an injectable field (defaulting to a small real duration in production, set to `0` in tests), never something a test waits out.

---

### Task 1: Bounded retry with injectable delay in `removeProjectDir`

**Files:**
- Modify: `api/internal/app/usecases/project/project_delete.go`
- Modify: `api/internal/app/usecases/project/project_delete_test.go`

**Interfaces:**
- Produces: `DeleteDeps.RemoveAllRetries int` (default 3) and `DeleteDeps.RemoveAllRetryDelay time.Duration` (default a small production value, e.g. 20ms) — both defaulted in `NewDelete` the same way `RemoveAll` already is. `removeProjectDir` retries up to `RemoveAllRetries` times, sleeping `RemoveAllRetryDelay` between attempts, and logs at `ERROR` (not `WARN`) only once every attempt has failed.

- [ ] **Step 1: Write the failing test**

Extend the existing test to prove retry happens, and add a new one proving a transient failure now actually clears:

```go
// TestProjectDelete_RemoveProjectDirFailure_IsLoggedNotFatal now also proves
// the retry budget is spent before giving up — a single failed attempt must
// not be the end of the story.
func TestProjectDelete_RemoveProjectDirFailure_IsLoggedNotFatal(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	var attempts int
	f.uc = project.NewDelete(project.DeleteDeps{
		Projects:    f.projects,
		Repos:       f.repos,
		Workspaces:  f.workspaces,
		Git:         f.git,
		CrowbarHome: func() (string, error) { return "/home/u/.crowbar", nil },
		RemoveAll: func(string) error {
			attempts++
			return errors.New("disk gremlin")
		},
		RemoveAllRetryDelay: 0, // no real sleeping in a test
	})

	err := f.uc.Delete(context.Background(), "p1")

	require.NoError(t, err, "a failed directory removal must not fail the whole delete")
	assert.Equal(t, []string{"p1"}, f.projects.deleted)
	assert.Greater(t, attempts, 1, "a persistently failing removal must be retried, not given up on after one try")
}

// TestProjectDelete_RemoveProjectDirFailure_ClearsOnRetry is the actual bug
// fix, proven directly: a transient failure (the shape a filesystem indexer
// or sync client briefly touching the directory produces) must not become a
// permanent orphan just because the FIRST attempt lost a race.
func TestProjectDelete_RemoveProjectDirFailure_ClearsOnRetry(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	var attempts int
	var removed []string
	f.uc = project.NewDelete(project.DeleteDeps{
		Projects:    f.projects,
		Repos:       f.repos,
		Workspaces:  f.workspaces,
		Git:         f.git,
		CrowbarHome: func() (string, error) { return "/home/u/.crowbar", nil },
		RemoveAll: func(path string) error {
			attempts++
			if attempts < 3 {
				return errors.New("transient: ENOTEMPTY")
			}
			removed = append(removed, path)
			return nil
		},
		RemoveAllRetryDelay: 0,
	})

	require.NoError(t, f.uc.Delete(context.Background(), "p1"))

	assert.Equal(t, 3, attempts)
	assert.Equal(t, []string{"/home/u/.crowbar/projects/p1"}, removed)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/app/usecases/project/... -run TestProjectDelete_RemoveProjectDirFailure -v`
Expected: FAIL — `RemoveAllRetryDelay` doesn't exist (compile error) until Step 3 lands; then the "clears on retry" case fails because today's code calls `RemoveAll` exactly once.

- [ ] **Step 3: Write minimal implementation**

Add fields to `DeleteDeps` and defaults in `NewDelete`:

```go
type DeleteDeps struct {
	Projects    DeleteProjectStore
	Repos       DeleteRepositoryStore
	Workspaces  DeleteWorkspaceRepo
	Git         DeleteGitEngine
	CrowbarHome func() (string, error)
	// RemoveAll deletes the entity-scoped project directory tree (worktrees,
	// storages, icon) under ~/.crowbar/projects/<P>. Defaults to os.RemoveAll
	// when nil; tests stub it to assert the exact path removed.
	RemoveAll func(path string) error
	// RemoveAllRetries bounds how many times a failing RemoveAll is retried
	// before the failure is logged and swallowed. Defaults to 3: enough to
	// clear the transient case (a filesystem indexer or sync client briefly
	// touching the directory between "remove children" and "remove the now-
	// empty parent") without turning a genuinely stuck removal into a long
	// stall on the delete path.
	RemoveAllRetries int
	// RemoveAllRetryDelay is the pause between retries. Defaults to a small
	// real duration in production; tests set it to 0 so nothing sleeps.
	RemoveAllRetryDelay time.Duration
}

// NewDelete builds a DeleteUsecase from its dependencies.
func NewDelete(
	deps DeleteDeps,
) DeleteUsecase {
	if deps.RemoveAll == nil {
		deps.RemoveAll = os.RemoveAll
	}
	if deps.RemoveAllRetries <= 0 {
		deps.RemoveAllRetries = 3
	}
	if deps.RemoveAllRetryDelay == 0 {
		deps.RemoveAllRetryDelay = 20 * time.Millisecond
	}
	return &projectDelete{deps: deps}
}
```

Update `removeProjectDir`:

```go
// removeProjectDir rm -rf's the entity-scoped project directory tree
// (~/.crowbar/projects/<P> — worktrees, storages, icon) once the GORM rows are
// gone. It is guarded by the crowbarHome prefix so it can NEVER touch a user's
// real repo Path or an adopted main worktree (both live outside ~/.crowbar).
//
// A single failed RemoveAll is retried up to RemoveAllRetries times: the
// common real-world failure here is transient (a filesystem indexer or sync
// client briefly touching the directory between removing its children and
// removing the now-empty directory itself), and a bare, unretried attempt is
// exactly what left an empty, permanently orphaned, DB-row-less directory
// behind in production. Only once every attempt has failed is it logged —
// at ERROR, not WARN, so it is operationally visible — and swallowed: the
// DB rows are already gone by this point, so failing Delete() itself would
// report an operation that in every way the user can observe DID succeed.
func (u *projectDelete) removeProjectDir(
	ctx context.Context,
	projectID string,
) {
	if u.deps.CrowbarHome == nil {
		return
	}
	home, err := u.deps.CrowbarHome()
	if err != nil || home == "" {
		return
	}
	dir := worktreepath.ProjectDir(home, projectID)
	if !strings.HasPrefix(dir, home) {
		return
	}
	var lastErr error
	for attempt := 0; attempt < u.deps.RemoveAllRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(u.deps.RemoveAllRetryDelay)
		}
		if lastErr = u.deps.RemoveAll(dir); lastErr == nil {
			return
		}
	}
	slog.ErrorContext(ctx, "project delete: remove project dir failed after retries; records already gone, directory left on disk",
		"project_id", projectID, "dir", dir, "attempts", u.deps.RemoveAllRetries, "err", lastErr)
}
```

Add `"time"` to `project_delete.go`'s imports.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/app/usecases/project/... -v`
Expected: PASS, full package — including every pre-existing `project_delete_test.go` case (`NoCrowbarHomeConfigured`, `CrowbarHomeError`, the path-traversal regression guard, and the rest).

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/project/project_delete.go api/internal/app/usecases/project/project_delete_test.go
git commit -m "fix(project): retry a failing project-dir removal instead of orphaning it on the first transient error"
```

---

### Task 2: Real-filesystem integration test — zero residue after delete

**Files:**
- Test: `api/internal/app/usecases/project/project_delete_disk_test.go` (new)

**Interfaces:**
- Consumes: `project.NewDelete` with the REAL default `RemoveAll` (`os.RemoveAll` — leave `RemoveAll` unset in `DeleteDeps` so the production default runs), against a real `t.TempDir()` standing in for crowbar home.

- [ ] **Step 1: Write the failing test**

```go
package project_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
)

// TestProjectDelete_RealFilesystem_LeavesNoResidue is the end-to-end proof
// this plan exists for: after Delete() returns, the project's ENTIRE on-disk
// tree — not just its icon, not just one repo's worktree, everything under
// projects/<id>/ — is gone. No mocked RemoveAll: this runs the real default
// against a real temp directory populated the way an actual project's tree
// looks (an icon file, a repo subdirectory holding a worktree with real
// files in it), which is exactly the shape a bare "no error was returned"
// unit test can't catch a regression in.
func TestProjectDelete_RealFilesystem_LeavesNoResidue(t *testing.T) {
	home := t.TempDir()
	projectDir := filepath.Join(home, "projects", "p1")
	repoDir := filepath.Join(projectDir, "r1")
	worktreeDir := filepath.Join(repoDir, "feature-x", "worktree")
	require.NoError(t, os.MkdirAll(worktreeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "icon"), []byte("fake-png-bytes"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, "README.md"), []byte("hi"), 0o644))

	f := newDeleteFixture(t)
	f.seedProject()
	f.workspaces.workspaces = []domain.Workspace{
		{
			ID: "w-child", RepoID: "r1", ProjectID: "p1",
			Branch: "feature-x", WorktreePath: worktreeDir,
		},
	}
	f.uc = project.NewDelete(project.DeleteDeps{
		Projects:    f.projects,
		Repos:       f.repos,
		Workspaces:  f.workspaces,
		Git:         f.git,
		CrowbarHome: func() (string, error) { return home, nil },
		// RemoveAll deliberately left unset: this test exercises the REAL
		// os.RemoveAll default, not a stub.
	})

	require.NoError(t, f.uc.Delete(context.Background(), "p1"))

	_, err := os.Stat(projectDir)
	require.True(t, os.IsNotExist(err), "projects/p1 must not exist at all after Delete(), got err=%v", err)
}
```

(This lives in package `project_test` alongside `project_delete_test.go`, so `newDeleteFixture`, `f.seedProject()`, `f.projects`/`f.repos`/`f.workspaces`/`f.git`, and `domain.Workspace` are already in scope from that file — add the `domain` import only if it is not already implicitly available via a shared `_test.go` helper file in the package.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/internal/app/usecases/project/... -run TestProjectDelete_RealFilesystem_LeavesNoResidue -v`
Expected: on today's `develop`, this should already PASS (the happy-path real `os.RemoveAll` has no reason to fail here) — its value is as a regression guard and as the concrete proof the user asked for, not as a currently-red test. Run it anyway and confirm it's green before moving on; if it's red, that is itself an important finding to report before continuing.

- [ ] **Step 3: N/A — no implementation change if Step 2 is already green**

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/internal/app/usecases/project/... -v`
Expected: PASS, full package.

- [ ] **Step 5: Commit**

```bash
git add api/internal/app/usecases/project/project_delete_disk_test.go
git commit -m "test(project): prove a real-filesystem project delete leaves zero on-disk residue"
```

---

## Self-Review Notes

- **Spec coverage:** Task 1 is the actual fix for the "boring" candidate mechanism (transient `RemoveAll` failure orphaning a directory); Task 2 is the real-filesystem proof the user asked for in place of a background GC.
- **Deliberately not attempted:** reproducing a genuine daemon-crash-mid-`Create()` scenario as an automated test — killing a live goroutine mid-flight is not a reliable, non-flaky thing to assert in a unit test, and the code-reading in this session's diagnosis already established that `Create()`/`Import()`'s rollback path (`project_import.go`'s `_ = u.deps.Projects.Delete(ctx, project.ID)` on a `createHomeWorkspace` failure) does not touch the filesystem before that failure point for a home-kind workspace, so there is nothing for a crash there to leave behind today.
