package runner

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// boundTestLedger is an in-memory ledger with chat-1 bound to provider's sessionID.
func boundTestLedger(t *testing.T, provider, sessionID string) *ledgerstore.Store {
	t.Helper()
	l := newTestLedger(t)
	row, err := ledger.Assign(ledger.State{}, provider, sessionID)
	require.NoError(t, err)
	_, _, err = l.Append(t.Context(), "chat-1", row)
	require.NoError(t, err)
	return l
}

// turnedTestLedger is boundTestLedger after a finished turn on the session.
func turnedTestLedger(t *testing.T, provider, sessionID string) *ledgerstore.Store {
	t.Helper()
	l := boundTestLedger(t, provider, sessionID)
	row, err := ledger.NewRow(ledger.KindTurnStarted, "T-turned", "", ledger.TurnStarted{
		Provider: provider, SessionID: sessionID,
	})
	require.NoError(t, err)
	ended, err := ledger.NewRow(ledger.KindTurnEnded, "T-turned", "", ledger.TurnEnded{Status: string(ledger.TurnStatusDone)})
	require.NoError(t, err)
	_, _, err = l.Append(t.Context(), "chat-1", row, ended)
	require.NoError(t, err)
	return l
}

// runningTestLedger is a ledger whose chat-1 is running a turn.
func runningTestLedger(t *testing.T) *ledgerstore.Store {
	t.Helper()
	l := newTestLedger(t)
	row, err := ledger.NewRow(ledger.KindTurnStarted, "T-running", "", ledger.TurnStarted{RunnerID: "runner-1"})
	require.NoError(t, err)
	_, _, err = l.Append(t.Context(), "chat-1", row)
	require.NoError(t, err)
	return l
}

func testAgent(t *testing.T, provider string) engineagents.Agent {
	t.Helper()
	agent, err := engineagents.New().Get(t.Context(), t.TempDir(), provider)
	require.NoError(t, err)
	return agent
}

func chatState(t *testing.T, l *ledgerstore.Store) ledger.State {
	t.Helper()
	st, err := l.State(t.Context(), "chat-1")
	require.NoError(t, err)
	return st
}

func assigned(t *testing.T, launch sessionLaunch) ledger.SessionBound {
	t.Helper()
	require.NotNil(t, launch.assign, "a provider that takes an id is launched on a minted one")
	body, err := ledger.DecodeBody[ledger.SessionBound](*launch.assign)
	require.NoError(t, err)
	return body
}

func TestLaunchSession_ResumesOnlyABoundSessionThatTurned(t *testing.T) {
	claude := testAgent(t, "claude")

	first, err := launchSession(ledger.State{}, claude)
	require.NoError(t, err)
	assert.False(t, first.lost)
	assert.Equal(t, ledger.SessionBound{Provider: "claude", SessionID: assigned(t, first).SessionID, Cause: ledger.CauseAssigned},
		assigned(t, first), "an unbound chat mints the id the CLI is launched with")

	resumed, err := launchSession(chatState(t, turnedTestLedger(t, "claude", "S1")), claude)
	require.NoError(t, err)
	assert.Equal(t, sessionLaunch{resumeID: "S1"}, resumed)

	unturned, err := launchSession(chatState(t, boundTestLedger(t, "claude", "S1")), claude)
	require.NoError(t, err)
	assert.Empty(t, unturned.resumeID, "a session that never turned was never written, so it is not resumed")
	assert.False(t, unturned.lost)
	again := assigned(t, unturned)
	assert.Equal(t, ledger.CauseAssigned, again.Cause)
	assert.Equal(t, "S1", again.Previous)
	assert.NotEqual(t, "S1", again.SessionID, "nor is its id handed to the CLI again")
}

func TestLaunchSession_ALostSessionStartsFresh(t *testing.T) {
	claude := testAgent(t, "claude")
	l := turnedTestLedger(t, "claude", "S1")
	_, _, err := l.Do(t.Context(), "chat-1", func(st ledger.State) ([]ledger.Row, error) {
		return ledger.Lose(st, "claude", "S1", "refused", "")
	})
	require.NoError(t, err)

	fresh, err := launchSession(chatState(t, l), claude)
	require.NoError(t, err)
	assert.Empty(t, fresh.resumeID, "a lost session is never resumed")
	assert.True(t, fresh.lost, "and its replacement is handed no transcript")
	replaced := assigned(t, fresh)
	assert.Equal(t, ledger.CauseFresh, replaced.Cause)
	assert.Equal(t, "S1", replaced.Previous)

	_, _, err = l.Append(t.Context(), "chat-1", *fresh.assign)
	require.NoError(t, err)
	stillFresh, err := launchSession(chatState(t, l), claude)
	require.NoError(t, err)
	assert.True(t, stillFresh.lost, "a fresh session that never turned still carries no conversation")
	assert.Equal(t, ledger.CauseFresh, assigned(t, stillFresh).Cause)
	assert.NotEqual(t, replaced.SessionID, assigned(t, stillFresh).SessionID)
}

func TestLaunchSession_AProviderThatTakesNoIDLaunchesFreshUnlessItTurned(t *testing.T) {
	codex := testAgent(t, "codex")

	none, err := launchSession(ledger.State{}, codex)
	require.NoError(t, err)
	assert.Equal(t, sessionLaunch{}, none, "a provider that takes no id at launch announces its own")

	unturned, err := launchSession(chatState(t, boundTestLedger(t, "codex", "X1")), codex)
	require.NoError(t, err)
	assert.Equal(t, sessionLaunch{}, unturned, "its announcement rebinds the chat")

	resumed, err := launchSession(chatState(t, turnedTestLedger(t, "codex", "X1")), codex)
	require.NoError(t, err)
	assert.Equal(t, sessionLaunch{resumeID: "X1"}, resumed)
}

// A launch that displaces a CLI is planned once that CLI has quit: its refusal
// on the way out lost the session the launch would otherwise resume.
func TestSubmitPrompt_ALaunchIsPlannedOnceTheCLIItDisplacesHasQuit(t *testing.T) {
	h := newSpawnHarness(t, "")
	term := &refusingTerm{}
	h.rs.term = term
	h.rs.chats = stubChatsForSpawn{chat: domain.Chat{ID: h.chatID, WorkspaceID: "ws-1", ProviderID: "claude"}}
	h.rs.conversations, h.rs.activity = reviveConversations{provider: "claude"}, reviveActivity{}
	h.rs.ledger = turnedTestLedger(t, "claude", "S1")
	_, err := h.rs.spawnRunner(t.Context(), h.chatID, "ws-1", "claude", spawnInput{}, false)
	require.NoError(t, err)
	requestID := uuid.NewString()

	_, err = h.rs.SubmitPrompt(t.Context(), h.chatID, "hello", requestID)

	require.NoError(t, err)
	require.Equal(t, 2, term.callCount(), "the send launches one CLI")
	argv := term.lastCall().argv
	assert.NotContains(t, argv, "--resume", "the session the displaced CLI refused is not resumed")
	assert.Contains(t, argv, "hello")
	assert.Equal(t, ledger.PromptStatusPending, promptReceipt(t, h.rs.ledger, requestID).Status)
}
