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

// worktreeStatus returns (status, present) for wsID off the repo's chat-list
// read model (GET .../chats) — the replacement for the deleted GET
// .../workspaces list. Every worktree-owning chat carries its git state inline
// (spec §5), so this is the one read that answers what the old workspace list
// used to. It ONLY works while wsID's owning chat still exists: once that chat
// is purged (e.g. by a DELETE .../chats/:id cascade) there is no surviving
// surface to read the workspace's own row through at all, so callers that
// delete the owning chat must not rely on this afterward — see the two delete
// tests below, which check the worktree directory on disk instead.
func worktreeStatus(t *testing.T, env *kit.Env, projectID, repoID, wsID string) (string, bool) {
	t.Helper()
	row, ok := env.WorktreeChats(t, projectID, repoID)[wsID]
	if !ok {
		return "", false
	}
	status, _ := row["status"].(string)
	return status, true
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
	// CreateWorkspaceWithChat, not the bare CreateWorkspace: a workspace with no
	// owning chat never appears on GET .../chats at all (container.go's
	// pushChatWorktree doc — "a workspace with no resolved owning chat pushes
	// nothing" — the read side has the same shape), so worktreeStatus below
	// would find nothing regardless of durability.
	wsID, _ := env1.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, branch, "")
	worktree := friendlyWorktree(env1, imported.ProjectID, imported.RepoPath, branch)
	require.True(t, kit.DirExists(t, worktree), "worktree must be provisioned before the crash")

	// Drain the async store projection so the COMMITTED workspace is durably in the
	// read model (WAL) before the kill — CreateWorkspaceWithChat returns on its own
	// internal Quiesce, but this one is the test's own explicit barrier before the
	// kill. Quiesce only drains projections; it is NOT a graceful shutdown, so the
	// kill below is still abrupt (no server drain, no app.Shutdown).
	env1.Quiesce()

	// Crash: no server drain, no app.Shutdown — abandon in-flight work mid-flight.
	env1.CloseCrashing(t)

	env2, err := kit.NewEnvWithHome(home)
	require.NoError(t, err, "restart over the same home after a crash")
	defer env2.Close(t)

	status, present := worktreeStatus(t, env2, imported.ProjectID, imported.RepoID, wsID)
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
// projection-synchronously and broadcasts the corrected worktree_state frame on
// the chat feed. The restarted daemon's read model must reflect the new
// pr-merged state.
func TestCrash_ProviderDriftWhileDown_ResyncOnRestart(t *testing.T) {
	home := kit.TempHomeForTest(t)
	env1 := kit.BuildEnvAt(t, home)
	imported := env1.ImportRepo(t, "drift", "")
	wsID, _ := env1.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/drift", "")

	// Before the daemon goes down, the PR is open.
	env1.PushProviderState(t, wsID, kit.ProviderState{
		HasPR:    true,
		PRStatus: "open",
		PRUrl:    "https://example.test/pr/7",
		PRTitle:  "feat: drift",
	})
	// PushProviderState applies the sync projection-synchronously (SendWait), so the
	// aggregate is already durable; Quiesce then drains the INDEPENDENT store/list
	// projection that feeds the REST read below. Both are real completions — there is
	// nothing left in flight to poll for.
	env1.Quiesce()
	s, ok := worktreeStatus(t, env1, imported.ProjectID, imported.RepoID, wsID)
	require.True(t, ok, "workspace must be present before the daemon goes down")
	require.Equal(t, "pr-open", s, "workspace must reach pr-open before the daemon goes down")
	env1.Close(t)

	// Restart: the read model reopens pr-open (durable). A provider poll then
	// observes the drift (the PR merged while we were down) and updates the model.
	env2, err := kit.NewEnvWithHome(home)
	require.NoError(t, err, "restart over the same home")
	defer env2.Close(t)

	// Chat ids are durable rows, but the intent here is "what does the REBOOTED
	// daemon say" (kit.Env.OwningChatID's own guidance), so re-resolve against
	// env2 rather than trust a chat id carried over from env1.
	chatID := env2.OwningChatID(t, wsID)
	watcher := env2.DialChat(t, chatID)
	env2.PushProviderState(t, wsID, kit.ProviderState{
		HasPR:    true,
		PRStatus: "merged",
		PRUrl:    "https://example.test/pr/7",
		PRTitle:  "feat: drift",
	})
	merged := kit.WaitForWorkspaceState(t, watcher, wsID, "pr-merged", 5*time.Second)
	require.Equal(t, wsID, merged["id"])

	// The durable read model reflects the re-synced state after the poll.
	status, present := worktreeStatus(t, env2, imported.ProjectID, imported.RepoID, wsID)
	require.True(t, present)
	require.Equal(t, "pr-merged", status, "restarted read model must reflect the provider drift observed on re-poll")
}

// TestCrash_DeleteConvergesToInvariant covers the delete lifecycle underlying
// spec §5 row "Deleted + lingering worktree" (spec §3.6/§3.8): a delete drives
// the cascade (git worktree teardown) plus the pure Delete command, whose async
// reactor gates on the persisted "deleted" tombstone, then rm's the worktree
// and Forgets the aggregate (its OnForget drops the
// read-model row). The observable end state is the delete invariant: no
// worktree on disk.
//
// The trigger is DELETE .../chats/:chatId now (spec §8 step 6 deleted the old
// workspace-scoped route): deleting wsID's OWNING chat reaps its worktree
// through the exact same DeleteCascade the old route called
// (chat/internal/tree/chats.go's reapWorktrees → DiscardChildWorkspace →
// hierarchy.DeleteCascade → workspaces.Delete), before the chat row itself is
// hard-purged.
//
// (The CRASH variant of this row — kill mid-cascade so the BOOT ORPHAN-SWEEP,
// rather than the reactor, completes the purge — is asserted separately by
// TestCrash_DeleteMidCascade_BootSweepReaps below.)
func TestCrash_DeleteConvergesToInvariant(t *testing.T) {
	env := kit.BuildEnv(t)
	imported := env.ImportRepo(t, "delete", "")
	const branch = "feature/delete-me"
	wsID, chatID := env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, branch, "")
	worktree := friendlyWorktree(env, imported.ProjectID, imported.RepoPath, branch)
	require.True(t, kit.DirExists(t, worktree), "worktree must exist before delete")

	// The delete is driven through the workspace usecase — the SAME call the
	// deleted DELETE .../workspaces/:wsId route made. This test's subject is the
	// delete CASCADE converging to its invariant.
	//
	// Watching the chat's own lifecycle frame is what proves the delete was
	// actually dispatched before the barrier below runs.
	watcher := env.DialChat(t, chatID)
	env.DeleteWorkspaceCascade(t, wsID)
	kit.WaitForWorkspaceState(t, watcher, wsID, "deleted", 10*time.Second)

	// Now converge to the delete invariant: no row, no worktree. The purge runs in
	// the delete REACTOR — a detached goroutine, so folding the projections is not
	// enough to see its filesystem effect; QuiesceReactors joins the reactor itself
	// (the same drain the daemon's graceful shutdown performs). Once it returns the
	// purge is FINISHED, and the invariant is a plain assertion rather than a race.
	env.QuiesceReactors()
	// The ROW half of the invariant is read in-process (WorkspaceRow), not over
	// the wire. DELETE .../chats/:id purges the owning chat in the same request
	// that tombstones the workspace, so the chat list would report the workspace
	// "absent" the moment the chat went — before the purge had touched the row or
	// the disk. That check would pass whether or not the delete converged, which
	// is precisely the vacuous assertion this one exists instead of.
	_, present := env.WorkspaceRow(t, imported.ProjectID, imported.RepoID, wsID)
	require.False(t, present, "delete must converge to no read-model row")
	require.False(t, kit.DirExists(t, worktree), "delete must converge to no worktree on disk")
}

// TestCrash_DeleteMidCascade_BootSweepReaps covers spec §5 row "Deleted +
// lingering worktree → boot sweep reaps" in its CRASH variant (§3.8): a delete
// persists the "deleted" tombstone, then the daemon is SIGKILLed mid-cascade
// (CloseCrashing abandons the async purge reactor before it rm's the worktree and
// Forgets the aggregate). On restart the boot orphan-sweep runs synchronously in
// app.New — the reactor that would otherwise finish the purge died with the old
// process — reads the durable read model directly, finds the residual
// Status="deleted" row, and re-drives the SAME idempotent purge, converging to
// the delete invariant (no read-model row AND no worktree). The purge is guarded to the
// crowbar home, so the managed worktree is reaped while a user's real checkout
// could never be touched.
func TestCrash_DeleteMidCascade_BootSweepReaps(t *testing.T) {
	// The delete reactor is HELD at the drain gate before the delete, so the
	// crash below lands with the purge deterministically still pending. (The
	// other crash window — between the reactor's Forget and its row delete — is
	// pinned by reactors.TestPurger_AlreadyForgottenAggregate_DropsTheOrphanedRow;
	// racing a real reactor to it made this test pass or fail by timing.)

	home := kit.TempHomeForTest(t)
	env1 := kit.BuildEnvAt(t, home)
	imported := env1.ImportRepo(t, "crash-delete", "")
	const branch = "feature/reap-me"
	wsID, _ := env1.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, branch, "")
	worktree := friendlyWorktree(env1, imported.ProjectID, imported.RepoPath, branch)
	require.True(t, kit.DirExists(t, worktree), "worktree must exist before delete")

	// The tombstone is set through the workspace usecase — the SAME call the
	// deleted DELETE .../workspaces/:wsId route made — rather than through
	// DELETE .../chats/:chatId.
	//
	// This test's subject is the BOOT SWEEP: a workspace tombstoned but not yet
	// purged when the process died.
	env1.HoldReactors()
	env1.DeleteWorkspaceCascade(t, wsID)
	// Quiesce folds the tombstone into the projection the boot sweep reads
	// directly at the next restart (store/workspace.db, no lazy Replay — spec
	// §3.7/§3.8).
	env1.Quiesce()

	// Establish the crash-orphan PRECONDITION before pulling the plug: the
	// "deleted" tombstone must be DURABLE, because that persisted row is the only
	// thing the next boot's sweep can find. Without it there is no orphan to reap
	// and this test asserts on nothing.
	//
	// It is read in-process: no wire read can reach this workspace's row any
	// more. The held reactor guarantees the purge has not run.
	status, present := env1.WorkspaceRow(t, imported.ProjectID, imported.RepoID, wsID)
	require.True(t, present, "precondition: the deleted row must be durable before the crash")
	require.Equal(t, "deleted", status,
		"precondition: the tombstone must be PERSISTED before the crash — it is the only thing "+
			"the next boot's sweep can find")

	// SIGKILL with the purge still pending: the held reactor never ran.
	env1.CloseCrashing(t)

	// Restart over the same home: app.New's boot orphan-sweep
	// (container.go's startBootSweep → reconcile.Sweeper.Sweep) now runs FULLY
	// SYNCHRONOUSLY, purge and all, before app.New returns — it explicitly
	// replaced the old ASYNC recovery sweep a prior version of this test polled
	// for with require.Eventually. There is nothing left to wait for: by the
	// time NewEnvWithHome returns, the sweep has already reaped or not.
	env2, err := kit.NewEnvWithHome(home)
	require.NoError(t, err, "restart over the same home after a crash")
	defer env2.Close(t)

	// BOTH halves: the row and the worktree.
	_, stillThere := env2.WorkspaceRow(t, imported.ProjectID, imported.RepoID, wsID)
	require.False(t, stillThere, "boot sweep must reap the crash-orphaned deleted row")
	require.False(t, kit.DirExists(t, worktree), "boot sweep must reap the lingering worktree")
}
