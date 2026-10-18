package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
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
// stream is salvaged, never another runner's, and the chat stops working.
func TestCloseAbandonedTurn_SalvagesTheRunnersOwnStreamAndEndsTheTurn(t *testing.T) {
	outgoing := agents.Runner{ID: "runner-old"}
	spy := &spyAbandonMessageForRunner{}
	rs := &Runners{
		turns:    spy,
		activity: stubActivityForSpawn{},
		chats:    stubChatsForSpawn{chat: domain.Chat{ID: "chat-1"}},
		work:     inflight.NewWork(),
	}
	rs.work.Set("chat-1", true)

	rs.closeAbandonedTurn(context.Background(), "chat-1", outgoing)

	require.Len(t, spy.calls, 1, "the outgoing runner's own stream must be salvaged")
	assert.Equal(t, "chat-1", spy.calls[0].chatID)
	assert.Equal(t, outgoing, spy.calls[0].runner)
	working, known, _ := rs.work.Observe("chat-1")
	assert.True(t, known)
	assert.False(t, working, "a turn whose CLI is gone is not running")
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
// SwitchToTerminal forks a NEW native-view PTY tracked only in rs.attached,
// while runner.TerminalSession still names the companion PTY. Retiring the
// chat ends both and forgets the view, or the visible process runs forever.
func TestRegression_RetireTerminatesTheAttachedNativeViewNotJustTheStaleCompanionPTY(t *testing.T) {
	term := &fakeTermForAttach{}
	rs := retiring(t, &Runners{
		attached:      newAttachRegistry(),
		apiConns:      newAPIConnRegistry(),
		term:          term,
		agents:        stubAgentsForInterrupt{},
		inflightTurns: inflight.NewTurns(),
		turns:         noopTurns{},
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
		agents:        stubAgentsForInterrupt{},
		turns:         &spyAbandonMessageForRunner{},
		inflightTurns: inflight.NewTurns(),
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
	exits := 0
	rows, err := rs.ledger.Read(t.Context(), "chat-1", 0, 0)
	require.NoError(t, err)
	for _, r := range rows {
		if r.Kind == ledger.KindRunnerExited {
			exits++
		}
	}
	assert.Equal(t, 1, exits, "a second boot finds nothing live to end")
}
