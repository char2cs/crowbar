package runner

import (
	"os"
	"path/filepath"
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
	assert.Equal(t, ledger.SessionBound{Provider: "claude", SessionID: assigned(t, first).SessionID, Cause: ledger.CauseAssigned},
		assigned(t, first), "an unbound chat mints the id the CLI is launched with")

	resumed, err := launchSession(chatState(t, turnedTestLedger(t, "claude", "S1")), claude)
	require.NoError(t, err)
	assert.Equal(t, sessionLaunch{resumeID: "S1"}, resumed)

	unturned, err := launchSession(chatState(t, boundTestLedger(t, "claude", "S1")), claude)
	require.NoError(t, err)
	assert.Empty(t, unturned.resumeID, "a session that never turned was never written, so it is not resumed")
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
	replaced := assigned(t, fresh)
	assert.Equal(t, ledger.CauseFresh, replaced.Cause)
	assert.Equal(t, "S1", replaced.Previous)

	_, _, err = l.Append(t.Context(), "chat-1", *fresh.assign)
	require.NoError(t, err)
	stillFresh, err := launchSession(chatState(t, l), claude)
	require.NoError(t, err)
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

// pinsRelay is a provider whose session calls name {relay_id}, so it keeps that identity for
// the session's life.
const pinsRelayDescriptor = `
id: pins-relay
spawn:
  cmd: acme
  interactive_required: true
events:
  prompt:
    fresh:
      - call: thread/start
        send: { relay: { id: "{relay_id}" } }
        capture: { session_id: thread.id }
    resume:
      - call: thread/resume
        send: { threadId: "{session_id}", relay: { id: "{relay_id}" } }
    action:
      - call: turn/start
        send: { threadId: "{session_id}", text: "{text}" }
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    delivery: stdio
    format: ndjson
    serve: [acme, serve]
    handshake: { call: initialize, notify: initialized }
`

func pinsRelayAgent(t *testing.T) engineagents.Agent {
	t.Helper()
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "descriptors", "pins-relay.yaml"), []byte(pinsRelayDescriptor), 0o600))
	agent, err := engineagents.New().Get(t.Context(), home, "pins-relay")
	require.NoError(t, err)
	return agent
}

// bindWithRelay is chat-1 bound to the provider's session "S1" with relay id relay, after a
// finished turn.
func bindWithRelay(t *testing.T, provider, relay string) ledger.State {
	t.Helper()
	l := newTestLedger(t)
	bound, err := ledger.NewRow(ledger.KindSessionBound, "", "", ledger.SessionBound{
		Provider: provider, SessionID: "S1", Cause: ledger.CauseAnnounced, RelayID: relay,
	})
	require.NoError(t, err)
	started, err := ledger.NewRow(ledger.KindTurnStarted, "T-turned", "", ledger.TurnStarted{Provider: provider, SessionID: "S1"})
	require.NoError(t, err)
	ended, err := ledger.NewRow(ledger.KindTurnEnded, "T-turned", "", ledger.TurnEnded{Status: string(ledger.TurnStatusDone)})
	require.NoError(t, err)
	_, _, err = l.Append(t.Context(), "chat-1", bound, started, ended)
	require.NoError(t, err)
	return chatState(t, l)
}

// Every launch on one binding names the relay id the session was created with; a session
// created anew is the only one that gets a new id.
func TestLaunchSession_AResumeKeepsTheRelayIDOfItsBindingAndAFreshSessionMintsOne(t *testing.T) {
	agent := pinsRelayAgent(t)
	require.True(t, agent.SessionPinsRelay())
	st := bindWithRelay(t, "pins-relay", "R1")

	first, err := launchSession(st, agent)
	require.NoError(t, err)
	second, err := launchSession(st, agent)
	require.NoError(t, err)
	assert.Equal(t, sessionLaunch{resumeID: "S1", relayID: "R1"}, first)
	assert.Equal(t, first, second, "two runners on one binding are launched the same way")

	fresh, err := launchSession(ledger.State{}, agent)
	require.NoError(t, err)
	assert.NotEmpty(t, fresh.relayID)
	other, err := launchSession(ledger.State{}, agent)
	require.NoError(t, err)
	assert.NotEqual(t, fresh.relayID, other.relayID, "a binding of its own has an identity of its own")
	assert.NotEqual(t, "R1", fresh.relayID)
}

// A session bound before its provider pinned a relay has none to resume with.
func TestLaunchSession_ASessionBoundBeforeTheRelayHasNoRelayID(t *testing.T) {
	launch, err := launchSession(bindWithRelay(t, "pins-relay", ""), pinsRelayAgent(t))
	require.NoError(t, err)
	assert.Equal(t, sessionLaunch{resumeID: "S1"}, launch)
}

// A provider that pins no block is launched exactly as before.
func TestLaunchSession_AProviderThatPinsNoRelayIsGivenNone(t *testing.T) {
	agent := testAgent(t, "codex")
	require.False(t, agent.SessionPinsRelay())
	fresh, err := launchSession(ledger.State{}, agent)
	require.NoError(t, err)
	assert.Empty(t, fresh.relayID)
}

// A launch that displaces a CLI is planned once that CLI has quit: its refusal
// on the way out lost the session the launch would otherwise resume.
func TestSubmitPrompt_ALaunchIsPlannedOnceTheCLIItDisplacesHasQuit(t *testing.T) {
	h := newSpawnHarness(t, "")
	term := &refusingTerm{}
	h.rs.term = term
	h.rs.chats = stubChatsForSpawn{chat: domain.Chat{ID: h.chatID, WorkspaceID: "ws-1", ProviderID: "claude"}}
	h.rs.conversations = reviveConversations{provider: "claude"}
	h.rs.ledger = turnedTestLedger(t, "claude", "S1")
	_, err := h.rs.spawnRunner(t.Context(), h.chatID, "ws-1", "claude", spawnInput{})
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
