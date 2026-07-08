//go:build integration

// Package crash_test is the crash/recovery slice of the Task 17
// crash/recovery/rebuild/friendly-path integration matrix (spec §5 table). It
// drives the daemon crash primitive (CloseCrashing — SIGKILL semantics, no
// graceful drain) and the delete lifecycle over the real HTTP+WS+SQLite stack,
// then restarts over the same home to assert recovery invariants: committed
// state and provisioned worktrees survive an abrupt kill (WAL durability), a
// provider poll after restart re-syncs the read model (provider drift observed
// while down), and the delete cascade + async reactor converge to the delete
// invariant (no read-model row, no worktree).
//
// SCOPE NOTE (verified against live code): two spec §5 crash rows — "Crash
// mid-provision → reconcile completes/cleans" and "Crash mid-merge (MERGE_HEAD) →
// pr-conflicts" — require the lazy reconcile-on-open path (Task 9) to be
// PRODUCTION-WIRED via workspace.WithReconciler with a real, cancelable,
// timeout-bounded git+provider DeriveFunc that re-derives worktree/MERGE_HEAD and
// PR reality on the first per-id Get after boot. The reconcile machinery
// (reconcile.Reconciler / DeriveFunc / WithReconciler) is fully built and unit
// tested, but the concrete production DeriveFunc + its Get-path wiring is Task 9's
// deliverable and is deliberately NOT introduced by this test task (Task 17):
// bolting an unproven git+provider re-derivation onto every Get is exactly the
// untimed-network-git hot-path risk the refactor exists to kill, and belongs in
// Task 9 under its own TDD + review gate. Those two rows are therefore not asserted
// here. Every OTHER §5 crash/recovery row IS covered end-to-end: durable survival
// of an abrupt kill, provider drift re-synced on restart, the delete invariant via
// the async reactor, AND the crash-mid-cascade boot-sweep reap
// (TestCrash_DeleteMidCascade_BootSweepReaps).
package crash_test

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestMain is the integration entry point for the crash package.
func TestMain(m *testing.M) {
	kit.Main(m)
}

func friendlyWorktree(env *kit.Env, projectID, repoPath, branch string) string {
	return filepath.Join(env.HomeDir(), "projects", projectID, filepath.Base(repoPath), branch)
}

// workspaceStatus returns (status, present) for wsID from the repo-scoped
// read-model list.
func workspaceStatus(t *testing.T, env *kit.Env, projectID, repoID, wsID string) (string, bool) {
	t.Helper()
	resp := env.GET(t, "/v0/projects/"+projectID+"/repos/"+repoID+"/workspaces")
	kit.RequireStatus(t, resp, http.StatusOK)
	var list []map[string]any
	kit.DecodeEnvData(t, resp, &list)
	for _, w := range list {
		if w["id"] == wsID {
			s, _ := w["status"].(string)
			return s, true
		}
	}
	return "", false
}

// TestCrash_CommittedStateSurvivesAbruptKill covers the durability foundation of
// spec §5 row "Crash mid-provision": an abrupt SIGKILL-style crash (no graceful
// drain) must never corrupt or lose already-committed aggregate state or its
// provisioned worktree. After CloseCrashing and a restart over the same home,
// the workspace is still served from the durable read model and its worktree is
// intact on disk (WAL durability, spec §3.8 / decision 12).
//
// (The reconcile-on-open self-heal of a HALF-made worktree — spec §5's
// "reconcile completes/cleans" — is not asserted: that path is built but not
// production-wired; see the package SCOPE NOTE.)
func TestCrash_CommittedStateSurvivesAbruptKill(t *testing.T) {
	home := kit.TempHomeForTest(t)
	env1 := kit.BuildEnvAt(t, home)
	imported := env1.ImportRepo(t, "crash-durable", "")
	const branch = "feature/durable"
	wsID := env1.CreateWorkspace(t, imported.ProjectID, imported.RepoID, branch)
	worktree := friendlyWorktree(env1, imported.ProjectID, imported.RepoPath, branch)
	require.True(t, kit.DirExists(t, worktree), "worktree must be provisioned before the crash")

	// Crash: no server drain, no app.Shutdown — abandon in-flight work mid-flight.
	env1.CloseCrashing(t)

	env2, err := kit.NewEnvWithHome(home)
	require.NoError(t, err, "restart over the same home after a crash")
	defer env2.Close(t)

	status, present := workspaceStatus(t, env2, imported.ProjectID, imported.RepoID, wsID)
	require.True(t, present, "committed workspace must survive an abrupt kill")
	require.NotEqual(t, "deleted", status, "a committed non-deleted workspace must not be reaped by recovery")
	require.True(t, kit.DirExists(t, worktree), "provisioned worktree must survive an abrupt kill")
}

// TestCrash_ProviderDriftWhileDown_ResyncOnRestart covers spec §5 row "Provider
// drift while down → reconcile re-fetches → read model updated". While the
// daemon is down the remote PR transitions (open → merged); on restart a
// provider poll observes the drift and re-syncs the aggregate. The poll is
// injected through the mock-provider seam (PushProviderState, spec §11 — the
// same deterministic seam the provider suite uses), which applies the sync
// projection-synchronously and broadcasts the corrected WorkspaceDTO. The
// restarted daemon's read model must reflect the new pr-merged state.
func TestCrash_ProviderDriftWhileDown_ResyncOnRestart(t *testing.T) {
	home := kit.TempHomeForTest(t)
	env1 := kit.BuildEnvAt(t, home)
	imported := env1.ImportRepo(t, "drift", "")
	wsID := env1.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/drift")

	// Before the daemon goes down, the PR is open.
	env1.PushProviderState(t, wsID, kit.ProviderState{
		HasPR:    true,
		PRStatus: "open",
		PRUrl:    "https://example.test/pr/7",
		PRTitle:  "feat: drift",
	})
	require.Eventuallyf(t, func() bool {
		s, ok := workspaceStatus(t, env1, imported.ProjectID, imported.RepoID, wsID)
		return ok && s == "pr-open"
	}, 5*time.Second, 50*time.Millisecond, "workspace must reach pr-open before the daemon goes down")
	env1.Close(t)

	// Restart: the read model reopens pr-open (durable). A provider poll then
	// observes the drift (the PR merged while we were down) and updates the model.
	env2, err := kit.NewEnvWithHome(home)
	require.NoError(t, err, "restart over the same home")
	defer env2.Close(t)

	watcher := env2.DialWorkspace(t, imported.ProjectID, imported.RepoID, wsID)
	env2.PushProviderState(t, wsID, kit.ProviderState{
		HasPR:    true,
		PRStatus: "merged",
		PRUrl:    "https://example.test/pr/7",
		PRTitle:  "feat: drift",
	})
	merged := kit.WaitForWorkspaceState(t, watcher, wsID, "pr-merged", 5*time.Second)
	require.Equal(t, wsID, merged["id"])

	// The durable read model reflects the re-synced state after the poll.
	status, present := workspaceStatus(t, env2, imported.ProjectID, imported.RepoID, wsID)
	require.True(t, present)
	require.Equal(t, "pr-merged", status, "restarted read model must reflect the provider drift observed on re-poll")
}

// TestCrash_DeleteConvergesToInvariant covers the delete lifecycle underlying
// spec §5 row "Deleted + lingering worktree" (spec §3.6/§3.8): a delete drives
// the cascade (git worktree teardown) plus the pure Delete command, whose async
// reactor gates on the persisted "deleted" tombstone, then rm's the worktree,
// drops the id↔path row, and Forgets the aggregate (its OnForget drops the
// read-model row). The observable end state is the delete invariant: no
// read-model row AND no worktree.
//
// (The CRASH variant of this row — kill mid-cascade so the BOOT ORPHAN-SWEEP,
// rather than the reactor, completes the purge — is asserted separately by
// TestCrash_DeleteMidCascade_BootSweepReaps below.)
func TestCrash_DeleteConvergesToInvariant(t *testing.T) {
	env := kit.BuildEnv(t)
	imported := env.ImportRepo(t, "delete", "")
	const branch = "feature/delete-me"
	wsID := env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, branch)
	worktree := friendlyWorktree(env, imported.ProjectID, imported.RepoPath, branch)
	require.True(t, kit.DirExists(t, worktree), "worktree must exist before delete")

	resp := env.DELETE(t, "/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/workspaces/"+wsID)
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// Converge to the delete invariant: no row, no worktree.
	require.Eventuallyf(t, func() bool {
		_, present := workspaceStatus(t, env, imported.ProjectID, imported.RepoID, wsID)
		return !present && !kit.DirExists(t, worktree)
	}, 10*time.Second, 50*time.Millisecond,
		"delete must converge to no read-model row AND no worktree")
}

// TestCrash_DeleteMidCascade_BootSweepReaps covers spec §5 row "Deleted +
// lingering worktree → boot sweep reaps" in its CRASH variant (§3.8): a delete
// persists the "deleted" tombstone, then the daemon is SIGKILLed mid-cascade
// (CloseCrashing abandons the async purge reactor before it rm's the worktree and
// Forgets the aggregate). On restart the boot orphan-sweep runs synchronously in
// app.New — the reactor that would otherwise finish the purge died with the old
// process — reads the durable read model directly, finds the residual
// Status="deleted" row, and re-drives the SAME idempotent purge, converging to the
// delete invariant (no read-model row AND no worktree). The purge is guarded to
// the crowbar home, so the managed worktree is reaped while a user's real checkout
// could never be touched.
func TestCrash_DeleteMidCascade_BootSweepReaps(t *testing.T) {
	home := kit.TempHomeForTest(t)
	env1 := kit.BuildEnvAt(t, home)
	imported := env1.ImportRepo(t, "crash-delete", "")
	const branch = "feature/reap-me"
	wsID := env1.CreateWorkspace(t, imported.ProjectID, imported.RepoID, branch)
	worktree := friendlyWorktree(env1, imported.ProjectID, imported.RepoPath, branch)
	require.True(t, kit.DirExists(t, worktree), "worktree must exist before delete")

	// Delete, then wait for the persisted "deleted" tombstone (the store projection
	// folded the terminal state and broadcast it) BEFORE crashing — so the residual
	// deleted row + lingering worktree are exactly the crash-orphan state the boot
	// sweep must reap on the next boot.
	watcher := env1.DialWorkspace(t, imported.ProjectID, imported.RepoID, wsID)
	resp := env1.DELETE(t, "/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/workspaces/"+wsID)
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	kit.WaitForWorkspaceState(t, watcher, wsID, "deleted", 5*time.Second)

	// SIGKILL mid-cascade: abandon the async purge reactor (no graceful drain), so
	// on restart the boot sweep — not the reactor — is what must complete the purge.
	env1.CloseCrashing(t)

	// Restart over the same home: app.New runs the boot orphan-sweep before serving.
	env2, err := kit.NewEnvWithHome(home)
	require.NoError(t, err, "restart over the same home after a crash")
	defer env2.Close(t)

	require.Eventuallyf(t, func() bool {
		_, present := workspaceStatus(t, env2, imported.ProjectID, imported.RepoID, wsID)
		return !present && !kit.DirExists(t, worktree)
	}, 10*time.Second, 50*time.Millisecond,
		"boot sweep must reap the crash-orphaned deleted row AND its lingering worktree")
}
