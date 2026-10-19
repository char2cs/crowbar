package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// spyAbandonMessageForRunner records every call, so a test can assert the
// salvage ran (or didn't) without caring what the turn package does with it
// — that behaviour has its own tests one package over (turn/message_internal_test.go).
type spyAbandonMessageForRunner struct {
	noopTurns
	calls []abandonCall
}

type abandonCall struct {
	chatID string
	runner agents.Runner
}

func (s *spyAbandonMessageForRunner) AbandonMessageForRunner(
	_ context.Context, chatID string, runner agents.Runner,
) (bool, error) {
	s.calls = append(s.calls, abandonCall{chatID, runner})
	return true, nil
}

// A dead runner's turn is closed with what it already streamed: its own
// stream is salvaged, never another runner's.
func TestCloseAbandonedTurn_SalvagesTheRunnersOwnStream(t *testing.T) {
	outgoing := agents.Runner{ID: "runner-old"}
	spy := &spyAbandonMessageForRunner{}
	rs := &Runners{
		turns:    spy,
		activity: stubActivityForSpawn{},
		chats:    stubChatsForSpawn{chat: domain.Chat{ID: "chat-1"}},
	}

	rs.closeAbandonedTurn(context.Background(), "chat-1", outgoing)

	require.Len(t, spy.calls, 1, "the outgoing runner's own stream must be salvaged")
	assert.Equal(t, "chat-1", spy.calls[0].chatID)
	assert.Equal(t, outgoing, spy.calls[0].runner)
}

// TestCloseAbandonedTurn_EmptyChatID_SalvagesNothing pins the existing no-op
// guard this fix left untouched: a runner that was never on a chat at all
// (vacated == "") has nothing to salvage.
func TestCloseAbandonedTurn_EmptyChatID_SalvagesNothing(t *testing.T) {
	spy := &spyAbandonMessageForRunner{}
	rs := &Runners{turns: spy}

	rs.closeAbandonedTurn(context.Background(), "", agents.Runner{ID: "runner-old"})

	assert.Empty(t, spy.calls, "an empty chatID names nothing to salvage")
}

// TestRegression_RetireTerminatesTheAttachedNativeViewNotJustTheStaleCompanionPTY:
// the native view SwitchToTerminal forks is tracked only in rs.attached, so a
// retire must end it too, or the visible process runs forever.
func TestRegression_RetireTerminatesTheAttachedNativeViewNotJustTheStaleCompanionPTY(t *testing.T) {
	term := &fakeTermForAttach{}
	rs := retiring(t, &Runners{
		attached: newAttachRegistry(),
		apiConns: newAPIConnRegistry(),
		term:     term,
		agents:   stubAgentsForInterrupt{},
		turns:    noopTurns{},
	})
	rs.live.update("runner-1", func(r *agents.Runner) { r.TerminalSession = "companion-pty-term" })
	rs.attached.set("runner-1", attachedView{termSessID: "native-view-term"})

	rs.RetireChatRunners(context.Background(), "chat-1")

	assert.Contains(t, term.terminated, "companion-pty-term",
		"the original companion PTY is still a real process and must still be torn down")
	assert.Contains(t, term.terminated, "native-view-term",
		"the actually-attached native view PTY — the one the user was looking at — must be torn down too")
	_, stillAttached := rs.attached.get("runner-1")
	assert.False(t, stillAttached, "the attach registry entry must be forgotten, or it answers for a runner nothing will revisit")
	assert.Equal(t, domain.AgentExitDisplaced, retiredAs(t, rs))
}

// An exiting runner can acknowledge nothing more, so what it carried is
// settled with its exit, and only that.
func TestReconcileRunnerExit_SettlesOnlyThePromptsTheExitingRunnerCarried(t *testing.T) {
	rs := retiring(t, &Runners{
		agents: stubAgentsForInterrupt{},
		turns:  &spyAbandonMessageForRunner{},
	})
	appendPrompt(t, rs.ledger, "req-old", "runner-1")
	appendPrompt(t, rs.ledger, "req-new", "runner-new")

	rs.reconcileRunnerExit(t.Context(), "runner-1")

	old := promptReceipt(t, rs.ledger, "req-old")
	assert.Equal(t, ledger.PromptStatusSettled, old.Status)
	assert.Equal(t, ledger.ReasonRunnerExited, old.Reason)
	assert.Equal(t, ledger.PromptStatusPending, promptReceipt(t, rs.ledger, "req-new").Status)
	assert.Equal(t, domain.AgentExitExited, retiredAs(t, rs), "a runner that ends on its own exited")
	_, live := rs.RunnerByID("runner-1")
	assert.False(t, live)
}

// No CLI outlives the daemon that started it: a runner the ledger still holds
// live at boot is ended as daemon_restart, what it carried is settled, and the
// directory its spawn made is reaped.
func TestRegression_BootEndsTheLedgersLiveRunnerAsADaemonRestart(t *testing.T) {
	home := t.TempDir()
	chatsDir := filepath.Join(home, "chats")
	rs := retiring(t, &Runners{
		ws:    stubWorkspaceForSpawn{home: home, worktree: home, chatsDir: chatsDir},
		turns: noopTurns{},
	})
	rs.chats = stubChatsForSpawn{chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}}
	runnerDir := worktreepath.RunnerDir(chatsDir, "runner-1", "claude")
	require.NoError(t, os.MkdirAll(runnerDir, 0o700))
	started, err := ledger.NewRow(ledger.KindRunnerStarted, "", "", ledger.RunnerStarted{RunnerID: "runner-1", Provider: "claude"})
	require.NoError(t, err)
	_, _, err = rs.ledger.Append(t.Context(), "chat-1", started)
	require.NoError(t, err)
	appendPrompt(t, rs.ledger, "req-1", "runner-1")

	require.NoError(t, rs.ReconcileRunnersOnBoot(t.Context()))

	st := chatState(t, rs.ledger)
	_, live := st.LiveRunner()
	assert.False(t, live)
	assert.Equal(t, domain.AgentExitDaemonRestart, retiredAs(t, rs))
	assert.Equal(t, ledger.ReasonDaemonRestart, promptReceipt(t, rs.ledger, "req-1").Reason)
	assert.NoDirExists(t, runnerDir)

	require.NoError(t, rs.ReconcileRunnersOnBoot(t.Context()))
	rows, err := rs.ledger.Read(t.Context(), "chat-1", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, countKind(rows, ledger.KindRunnerExited), "a second boot finds nothing live to end")
}

func appendStarted(t *testing.T, l *ledgerstore.Store, chatID, runnerID string) {
	t.Helper()
	row, err := ledger.NewRow(ledger.KindRunnerStarted, "", "", ledger.RunnerStarted{RunnerID: runnerID, Provider: "claude"})
	require.NoError(t, err)
	_, _, err = l.Append(t.Context(), chatID, row)
	require.NoError(t, err)
}

// appendTurn starts a turn on runnerID.
func appendTurn(t *testing.T, l *ledgerstore.Store, chatID, runnerID string) {
	t.Helper()
	row, err := ledger.NewRow(ledger.KindTurnStarted, "T-"+chatID, "", ledger.TurnStarted{RunnerID: runnerID})
	require.NoError(t, err)
	_, _, err = l.Append(t.Context(), chatID, row)
	require.NoError(t, err)
}

// The boot pass closes a chat's turn in the commit that ends its runner, and
// one chat it cannot reconcile stops no other.
func TestReconcileRunnersOnBoot_OneChatsFailureLeavesTheOthersReconciled(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	l, err := ledgerstore.Open(db, time.Now)
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TRIGGER refuse_exit BEFORE INSERT ON chat_ledger
		WHEN NEW.kind = 'runner_exited' AND NEW.chat_id = 'chat-a' BEGIN SELECT RAISE(ABORT, 'boom'); END`).Error)
	home := t.TempDir()
	rs := &Runners{
		ledger: l, chats: stubChatsForSpawn{chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}}, activity: stubActivityForSpawn{}, turns: noopTurns{},
		ws: stubWorkspaceForSpawn{home: home, worktree: home, chatsDir: home},
	}
	for _, chatID := range []string{"chat-a", "chat-b", "chat-c"} {
		appendStarted(t, l, chatID, "runner-"+chatID)
		appendTurn(t, l, chatID, "runner-"+chatID)
	}

	require.ErrorContains(t, rs.ReconcileRunnersOnBoot(t.Context()), "chat-a")

	for chatID, ended := range map[string]bool{"chat-a": false, "chat-b": true, "chat-c": true} {
		st, err := l.State(t.Context(), chatID)
		require.NoError(t, err)
		_, live := st.LiveRunner()
		assert.Equal(t, !ended, live, chatID)
		assert.Equal(t, !ended, st.Busy(), "%s's turn closes with its runner", chatID)
	}
}

// purgedChats is a chat store in which gone no longer exists.
type purgedChats struct {
	stubChatsForSpawn
	gone string
}

func (c purgedChats) GetChat(ctx context.Context, chatID string) (domain.Chat, error) {
	if chatID == c.gone {
		return domain.Chat{}, agentchat.ErrNotFound
	}
	return c.stubChatsForSpawn.GetChat(ctx, chatID)
}

// A CLI a crashed pre-ledger daemon left running is adopted as its chat's
// live runner, so this boot ends it. Adopting again changes nothing, and a
// runner on a chat that is gone is skipped.
func TestAdoptLegacyRunners_TheBootEndsWhatACrashedOldDaemonLeftRunning(t *testing.T) {
	home := t.TempDir()
	rs := &Runners{
		ledger: newTestLedger(t), chats: purgedChats{stubChatsForSpawn: stubChatsForSpawn{chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}}, gone: "chat-purged"},
		activity: stubActivityForSpawn{}, turns: noopTurns{},
		ws: stubWorkspaceForSpawn{home: home, worktree: home, chatsDir: home},
	}
	legacy := []agents.Runner{
		{ID: "runner-1", ProviderID: "claude", CurrentChatID: "chat-1"},
		{ID: "runner-p", ProviderID: "claude", CurrentChatID: "chat-purged"},
	}

	require.NoError(t, rs.AdoptLegacyRunners(t.Context(), legacy))
	require.NoError(t, rs.AdoptLegacyRunners(t.Context(), legacy), "a step run again")
	require.NoError(t, rs.ReconcileRunnersOnBoot(t.Context()))
	require.NoError(t, rs.AdoptLegacyRunners(t.Context(), legacy), "a step run again after the boot ended it")

	assert.Equal(t, domain.AgentExitDaemonRestart, retiredAs(t, rs))
	rows, err := rs.ledger.Read(t.Context(), "chat-1", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, countKind(rows, ledger.KindRunnerStarted))
	assert.Equal(t, 1, countKind(rows, ledger.KindRunnerExited))
	purged, err := rs.ledger.Read(t.Context(), "chat-purged", 0, 0)
	require.NoError(t, err)
	assert.Empty(t, purged)
}

// A daemon shutting down ends every runner as daemon_restart: the exit each
// PTY's death fires records that cause once, closes its turn and frees its chat.
func TestShutdown_EveryRunnerEndsAsADaemonRestart(t *testing.T) {
	home := t.TempDir()
	rs := &Runners{
		live: liveWith(t, agents.Runner{ID: "runner-a", CurrentChatID: "chat-a"},
			agents.Runner{ID: "runner-b", CurrentChatID: "chat-b"}),
		ledger: newTestLedger(t), chats: stubChatsForSpawn{chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}}, activity: stubActivityForSpawn{},
		turns: noopTurns{}, agents: stubAgentsForInterrupt{}, surfaces: newSurfaceRegistry(),
		answers:  answerdesk.New(answerdesk.DefaultRetention, nil),
		apiConns: newAPIConnRegistry(), pendingHooks: inflight.NewHooks(), background: newBackgroundWork(),
	}
	chatIDs := []string{"chat-a", "chat-b"}
	for _, chatID := range chatIDs {
		appendStarted(t, rs.ledger, chatID, "runner-"+chatID[len("chat-"):])
		appendTurn(t, rs.ledger, chatID, "runner-"+chatID[len("chat-"):])
	}

	rs.Shutdown()
	for _, id := range []string{"runner-a", "runner-b"} {
		rs.onRunnerExit(home, id, filepath.Join(home, id), resumeWatch{})(engineterminal.Exit{Code: 143})
	}

	all, err := rs.AllLive(t.Context())
	require.NoError(t, err)
	assert.Empty(t, all)
	for _, chatID := range chatIDs {
		rows, err := rs.ledger.Read(t.Context(), chatID, 0, 0)
		require.NoError(t, err)
		st := ledger.Fold(chatID, rows)
		exit, ok := st.LastExit()
		require.True(t, ok, chatID)
		assert.Equal(t, domain.AgentExitDaemonRestart, exit.Reason, chatID)
		assert.Equal(t, 1, countKind(rows, ledger.KindRunnerExited), chatID)
		assert.False(t, st.Busy(), "%s's turn is closed", chatID)
	}
}

func countKind(rows []ledger.Row, kind ledger.Kind) int {
	n := 0
	for _, r := range rows {
		if r.Kind == kind {
			n++
		}
	}
	return n
}
