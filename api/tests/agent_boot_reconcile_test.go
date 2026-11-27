//go:build integration

package tests

import (
	"context"
	"net/http"
	"os"
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

func bootReconcileFixture(
	t *testing.T,
	home string,
) (h *harness, homeBase, chatID, runnerID string) {
	t.Helper()

	h = newHarnessAt(t, home)
	writeLiveStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	homeBase = "/v0/projects/" + imported.projectID + "/home"

	feed := recordSidebar(t, h)

	var created struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	feed.awaitListed(created.ID)

	startChat(t, h, homeBase, created.ID)
	spawned := getAgentChat(t, h, homeBase, created.ID)
	require.NotEmpty(t, spawned.LiveRunnerID, "precondition: the freshly spawned chat has a runner placed on it")
	require.NotEmpty(t, spawned.TerminalSessionID, "precondition: that runner has a PTY")
	runnerID = spawned.LiveRunnerID

	postHook(t, h, homeBase, "livestub", runnerID, "session_start", `{"session_id":"sess-before-restart"}`)
	feed.awaitConversation(created.ID, "sess-before-restart")

	postHook(t, h, homeBase, "livestub", runnerID, "user_prompt", `{"prompt":"hi"}`)
	feed.awaitTurnStart(created.ID)
	h.Quiesce()

	require.True(t, chatBusy(t, h, created.ID), "precondition: the chat is mid-turn before the restart")

	return h, homeBase, created.ID, runnerID
}

func TestRegression_AgentBootReconcile_DeadCLIProcess_EndsSegment(t *testing.T) {
	home := t.TempDir()
	h1, homeBase, chatID, runnerID := bootReconcileFixture(t, home)

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

	assert.Equal(t, []string{"sess-before-restart"}, post.sessionIDs(),
		"boot reconcile reaps the RUNNER, never the chat's conversation history")

	assert.False(t, chatBusy(t, h2, chatID),
		"boot reconcile must close the turn interrupted by the restart: the CLI that would have sent the "+
			"turn_stop hook no longer exists, so a chat left working spins forever — and the workspace's "+
			"whole overlay spins with it")
}

func TestRegression_AfterRestart_ResumeStillWorks(t *testing.T) {
	home := t.TempDir()
	h1, homeBase, chatID, deadRunnerID := bootReconcileFixture(t, home)

	before := getAgentChat(t, h1, homeBase, chatID)
	deadTermSessID := before.TerminalSessionID

	h1.crash()

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

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

func exitReason(t *testing.T, h *harness, _ importedRepo, chatID string) string {
	t.Helper()
	exit, _ := chatState(t, h, chatID).LastExit()
	return exit.Reason
}

func TestRegression_ARunnersExitReasonSurvivesARestart(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID, l := createFakeChat(t, h, imported, fake)
	killAndAwaitExit(t, h, recordSidebar(t, h), chatID, l)
	require.Equal(t, "exited", exitReason(t, h, imported, chatID), "precondition: the exit was reported")

	h.shutdown()
	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	assert.Equal(t, "exited", exitReason(t, h2, imported, chatID))
}

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

func TestRegression_AgentBootReconcileEndsTheRunnerACrashedOldDaemonLeft(t *testing.T) {
	assertLegacyRunnerEnded(t, false)
}

func TestRegression_AgentBootReconcileEndsTheRunnerOnAHomeThatAdoptedTheProviderStep(t *testing.T) {
	assertLegacyRunnerEnded(t, true)
}

func assertLegacyRunnerEnded(t *testing.T, providerStepMarked bool) {
	t.Helper()
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
	if providerStepMarked {
		marker := filepath.Join(home, "state", "upgrades", "chat-providers-from-history")
		require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o700))
		require.NoError(t, os.WriteFile(marker, nil, 0o600))
	}
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

func chatBusy(t *testing.T, h *harness, chatID string) bool {
	t.Helper()
	snap, err := h.app.Usecases.AgentChat.ChatSnapshot(context.Background(), chatID)
	require.NoError(t, err)
	return snap.Ledger.Busy
}

func TestRegression_ANewChatAcrossARestartLaunchesOnceOnItsFirstPrompt(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID := createChat(t, h, imported, fake.ID)

	h.shutdown()
	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	assert.Empty(t, readChatSession(t, h2, imported, chatID).LiveRunnerID)
	assert.Empty(t, exitReason(t, h2, imported, chatID), "no runner ever ran, so none ended")
	l := promptLaunch(t, h2, imported, fake, chatID, "after the restart", "")
	requireOneLaunch(t, fake, l)
}
