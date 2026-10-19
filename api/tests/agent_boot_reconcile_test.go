//go:build integration

package tests

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/adapter/migrations/chatledger"
	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// bootReconcileFixture boots a daemon at home, spawns a livestub chat, binds a
// conversation to its runner, and opens a turn — the exact pre-restart state both
// restart regressions below start from.
//
// Binding a conversation is load-bearing, not decoration: a chat with no
// conversation has nothing for ResumeChat to bring back (its ledger binds
// none), so a chat that never announced could not distinguish "resume
// is broken" from "there was nothing to resume".
//
// It returns the harness, the project-home mount, the chat, and the id of the
// runner placed on it — the crowbarSegmentID every in-PTY hook carries.
func bootReconcileFixture(
	t *testing.T,
	home string,
) (h *harness, homeBase, chatID, runnerID string) {
	t.Helper()

	h = newHarnessAt(t, home)
	writeLiveStubProviderDescriptor(t, h) // `cat` — holds its PTY open like a real CLI
	imported := importProject(t, h)
	homeBase = "/v0/projects/" + imported.projectID + "/home"

	frames := dialAgentWS(t, h, homeBase+"/chats/ws")

	var created struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	waitForChatFrame(t, frames, created.ID, "created")

	h.Quiesce()
	spawned := getAgentChat(t, h, homeBase, created.ID)
	require.NotEmpty(t, spawned.LiveRunnerID, "precondition: the freshly spawned chat has a runner placed on it")
	require.NotEmpty(t, spawned.TerminalSessionID, "precondition: that runner has a PTY")
	runnerID = spawned.LiveRunnerID

	// Announce a conversation, so the chat has history to be revived from.
	postHomeHook(t, h, homeBase, runnerID, "session_start", `{"session_id":"sess-before-restart"}`)
	waitForConversation(t, frames, created.ID, "sess-before-restart")

	// Open a turn, so the boot reconcile has a live turn to close — not merely a
	// runner row to reap (the crash-mid-turn shape the FE spinner depends on).
	postHomeHook(t, h, homeBase, runnerID, "user_prompt", `{"prompt":"hi"}`)
	waitForChatFrame(t, frames, created.ID, "turn_started")
	h.Quiesce()

	require.True(t, chatBusy(t, h, created.ID), "precondition: the chat is mid-turn before the restart")

	return h, homeBase, created.ID, runnerID
}

// TestRegression_AgentBootReconcile_DeadCLIProcess_EndsSegment is the live-bug
// guard for "the read model lies about a live agent after a daemon restart".
//
// It is the SEGMENT-era test of the same name, ported onto runners: the behaviour it
// pins is unchanged, only the thing that carries liveness moved. Where it used to
// assert the chat's active SEGMENT was ended (a stored status field), it now asserts
// the chat has no LIVE RUNNER — because the live-runner row exists exactly while its
// PTY does, so its absence IS the liveness verdict and there is no status left to
// lie. What must not regress is the reconcile itself.
//
// Scenario (exactly the reproduction): a chat's vendor CLI runs in a PTY, its turn is
// open (Working), the daemon is restarted. The CLI process dies with the daemon — no
// event ever records that, because the only thing that records a death is an exit
// callback living in the process that just went away — so ReconcileRunnersOnBoot must
// Exit the runner and close the turn it abandoned on the next boot.
//
// The trap this pins: a graceful daemon stop SUSPENDS every live PTY (terminal
// Shutdown persists a "suspended" meta row) and the next boot's
// RestorePersistedSessions reloads it as a PTY-LESS PLACEHOLDER under the SAME
// session id. A liveness check that merely asks "is this id in the registry" (the
// engine's SessionExists) therefore reads the dead CLI as alive and skips the
// reconcile — leaving the chat advertising a live agent forever and letting the agent
// pane re-attach to a placeholder the terminal engine resurrects as a BARE SHELL.
// Reconciliation must ask SessionLive ("is this PROCESS alive"), the sole authority.
//
// The daemon DIES rather than stopping gracefully (harness.crash), because only a
// death produces the stale row this reconcile exists for. A graceful stop kills each
// PTY and lets the resulting exit callback RECORD the death on the way down, so boot
// reconciliation would find nothing left to do and the test would pass without ever
// exercising it. A crash — a SIGKILL, the watchdog's SIGQUIT, a panic — records
// nothing at all, which is exactly the state the doc comment on ReconcileRunnersOnBoot
// describes: "a durable sqlite table full of live rows describing CLIs that no longer
// exist".
//
// No timing: every step blocks on a real signal (WS frame / asynx Quiesce), and the
// restart is a real container teardown + rebuild over the same crowbar home.
func TestRegression_AgentBootReconcile_DeadCLIProcess_EndsSegment(t *testing.T) {
	home := t.TempDir()
	h1, homeBase, chatID, runnerID := bootReconcileFixture(t, home)

	// --- the daemon dies; nothing records the CLI's death. Then it boots again. ---
	h1.crash()

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	post := getAgentChat(t, h2, homeBase, chatID)
	assert.Empty(t, post.LiveRunnerID,
		"boot reconcile must leave the chat DORMANT: runner %s's CLI process died with the daemon, so the "+
			"live-runner row naming it must be gone — a chat that still advertises a live runner hands the "+
			"pane a dead PTY (and, worse, a suspended placeholder the engine resurrects as a bare shell)",
		runnerID)
	assert.Empty(t, post.TerminalSessionID,
		"a dormant chat must offer no terminal session to attach to")

	// The conversation history SURVIVES the reconcile: it is append-only and describes
	// no process, so nothing about a dead CLI may erase it. It is also the only thing
	// that makes the chat revivable (see TestRegression_AfterRestart_ResumeStillWorks).
	assert.Equal(t, []string{"sess-before-restart"}, post.sessionIDs(),
		"boot reconcile reaps the RUNNER, never the chat's conversation history")

	assert.False(t, chatBusy(t, h2, chatID),
		"boot reconcile must close the turn interrupted by the restart: the CLI that would have sent the "+
			"turn_stop hook no longer exists, so a chat left working spins forever — and the workspace's "+
			"whole overlay spins with it")
}

// TestRegression_AfterRestart_ResumeStillWorks pins the OTHER half of boot
// reconciliation, and it is the half that makes the first half matter: after a
// restart, a chat must still be REVIVABLE.
//
// The bug it guards is not cosmetic staleness. A stale live-runner row is
// indistinguishable from a running CLI to every read in the agent usecase, and
// ResumeChat asks LiveRunnerForChat FIRST:
//
//	live, err := u.runners.LiveRunnerForChat(ctx, chatID)
//	if err == nil { return live.ID, nil }   // "already live — nothing to do"
//
// Handed the corpse of a CLI that died with the daemon, Resume concludes the chat is
// already live and returns it as a NO-OP. The Resume button silently does nothing,
// forever, and the pane attaches to a dead terminal session. Without the boot
// reconcile, NO chat could ever be revived after ANY restart — every chat the user
// had open is permanently inert.
//
// So this asserts the specific broken OUTCOME, not merely that resume "works": the
// runner Resume produces must be a DIFFERENT runner on a DIFFERENT PTY than the one
// that died. Returning the pre-restart runner id is exactly what the bug did.
func TestRegression_AfterRestart_ResumeStillWorks(t *testing.T) {
	home := t.TempDir()
	h1, homeBase, chatID, deadRunnerID := bootReconcileFixture(t, home)

	before := getAgentChat(t, h1, homeBase, chatID)
	deadTermSessID := before.TerminalSessionID

	// The daemon DIES (see harness.crash): the runner row survives in sqlite, still
	// claiming a PTY that no longer exists. That stale row is the bug's whole input.
	h1.crash()

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	// Boot reconciliation should already have dropped the corpse (its own guard is the
	// test above). Asserted, NOT required: if the stale row is still there, the whole
	// point is what Resume then does with it — so the test must go on to find out, and
	// report the user-visible outcome rather than stopping at the cause.
	assert.Empty(t, getAgentChat(t, h2, homeBase, chatID).LiveRunnerID,
		"the chat should be dormant after the restart")

	var revived struct {
		ID string `json:"id"`
	}
	h2.post(homeBase+"/chats/"+chatID+"/resume", nil, http.StatusOK, &revived)
	revivedRunnerID := revived.ID

	require.NotEmpty(t, revivedRunnerID, "resume must spawn a runner")
	require.NotEqual(t, deadRunnerID, revivedRunnerID,
		"resume must spawn a NEW CLI, never hand back the runner that died with the daemon: a stale live "+
			"row makes ResumeChat's LiveRunnerForChat guard fire and return the corpse as a silent no-op — "+
			"the Resume button doing nothing forever, and the pane attaching to a dead PTY")

	h2.Quiesce()
	back := getAgentChat(t, h2, homeBase, chatID)
	assert.Equal(t, revivedRunnerID, back.LiveRunnerID,
		"the revived chat must advertise the runner resume just started")
	assert.NotEmpty(t, back.TerminalSessionID, "the revived runner must have a real PTY to attach to")
	assert.NotEqual(t, deadTermSessID, back.TerminalSessionID,
		"a PTY does not survive a daemon restart, so the revived chat must offer a NEW terminal session — "+
			"re-serving the dead one is how the pane ends up attached to a suspended placeholder")
	assert.Equal(t, "livestub", back.ActiveProviderID,
		"resume brings back the provider the chat was last on, read from its conversation history")
}

// exitReason is how chatID's last runner ended, as the chat's session reports it.
func exitReason(t *testing.T, h *harness, imported importedRepo, chatID string) string {
	t.Helper()
	view := readChatSession(t, h, imported, chatID)
	if view.Session == nil {
		return ""
	}
	return view.Session.ExitReason
}

// A chat's last exit reason is a ledger fact, so the next daemon still knows
// why the CLI ended.
func TestRegression_ARunnersExitReasonSurvivesARestart(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID, l := createFakeChat(t, h, imported, fake)
	killAndAwaitExit(t, h, h.dial(repoBase(imported)+"/chats/ws"), l)
	require.Equal(t, "exited", exitReason(t, h, imported, chatID), "precondition: the exit was reported")

	h.shutdown()
	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	assert.Equal(t, "exited", exitReason(t, h2, imported, chatID))
}

// A runner the ledger still holds live at boot died with the old daemon: boot
// records it as daemon_restart, and the next prompt launches a new CLI.
func TestRegression_ABootWithALiveRunnerRecordsDaemonRestart(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID, l1 := createFakeChat(t, h, imported, fake)

	h.crash()
	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	view := readChatSession(t, h2, imported, chatID)
	assert.Empty(t, view.LiveRunnerID, "no CLI outlives the daemon that started it")
	assert.Equal(t, "daemon_restart", exitReason(t, h2, imported, chatID))
	l2 := promptLaunch(t, h2, imported, fake, chatID, "after the restart", "")
	assert.NotEqual(t, l1.RunnerID, l2.RunnerID, "the prompt launched a new CLI")
}

// A daemon from before runners were ledger rows that crashed mid-turn left its
// runner only in its own runner store. The upgrade boot ends it like any other,
// or the chat's turn spins forever.
func TestRegression_AgentBootReconcileEndsTheRunnerACrashedOldDaemonLeft(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	imported := importWritableWorkspace(t, h)
	h.Quiesce()
	h.crash()
	now := time.Now().UTC()
	chat := domain.Chat{
		ID: uuid.NewString(), WorkspaceID: imported.workspaceID, ProviderID: "claude", Title: "mid-turn",
		CreatedAt: now,
	}
	seed := openBaseEra(t, home)
	seed.chat(chat)
	seed.close()
	store := homeAt(t, home).Store()
	legacy := openSQLite(t, filepath.Join(store, chatledger.RunnerDB))
	require.NoError(t, legacy.Exec(chatledger.RunnersDDL).Error)
	require.NoError(t, legacy.Exec(
		"INSERT INTO agent_runners (id, provider_id, current_chat_id, started_at) VALUES (?, ?, ?, ?)",
		"runner-old", "claude", chat.ID, now).Error)
	closeSQLite(t, legacy)

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	assert.False(t, chatBusy(t, h2, chat.ID), "the turn the old daemon died in is closed")
	assert.Equal(t, "daemon_restart", exitReason(t, h2, imported, chat.ID))
	ledgerDB := openSQLite(t, filepath.Join(store, ledgerstore.FileName))
	defer closeSQLite(t, ledgerDB)
	var exits int64
	require.NoError(t, ledgerDB.Raw(
		"SELECT count(*) FROM chat_ledger WHERE chat_id = ? AND kind = 'runner_exited'", chat.ID).Scan(&exits).Error)
	assert.Equal(t, int64(1), exits)
}

func openSQLite(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func closeSQLite(t *testing.T, db *gorm.DB) {
	t.Helper()
	raw, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, raw.Close())
}

// chatBusy is the chat's snapshot saying a turn or its work is open.
func chatBusy(t *testing.T, h *harness, chatID string) bool {
	t.Helper()
	snap, err := h.app.Usecases.AgentChat.ChatSnapshot(context.Background(), chatID)
	require.NoError(t, err)
	return snap.Ledger.Busy
}
