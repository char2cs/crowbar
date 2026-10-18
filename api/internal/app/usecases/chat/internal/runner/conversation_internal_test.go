package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
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

func testAgent(t *testing.T, provider string) engineagents.Agent {
	t.Helper()
	agent, err := engineagents.New().Get(t.Context(), t.TempDir(), provider)
	require.NoError(t, err)
	return agent
}

func TestLaunchSession_ResumesTheBoundSessionByIDAndOnlyThat(t *testing.T) {
	claude, codex := testAgent(t, "claude"), testAgent(t, "codex")

	first, err := launchSession(ledger.State{}, claude)
	require.NoError(t, err)
	require.NotNil(t, first.assign, "an unbound chat mints the id the CLI is launched with")
	assert.False(t, first.lost)
	minted, err := ledger.DecodeBody[ledger.SessionBound](*first.assign)
	require.NoError(t, err)
	assert.Equal(t, ledger.CauseAssigned, minted.Cause)

	bound := ledger.Fold("c1", []ledger.Row{{Seq: 1, Kind: first.assign.Kind, Body: first.assign.Body}})
	again, err := launchSession(bound, claude)
	require.NoError(t, err)
	assert.Equal(t, sessionLaunch{resumeID: minted.SessionID}, again)

	lost := bound.Clone()
	lostRow, err := ledger.Lose(bound, "claude", minted.SessionID, "refused", "")
	require.NoError(t, err)
	lostRow[0].Seq = 2
	lost.Apply(lostRow[0])
	fresh, err := launchSession(lost, claude)
	require.NoError(t, err)
	assert.Empty(t, fresh.resumeID, "a lost session is never resumed")
	assert.True(t, fresh.lost, "and its replacement is handed no transcript")
	require.NotNil(t, fresh.assign)
	replaced, err := ledger.DecodeBody[ledger.SessionBound](*fresh.assign)
	require.NoError(t, err)
	assert.Equal(t, ledger.CauseFresh, replaced.Cause)
	assert.NotEqual(t, minted.SessionID, replaced.SessionID)

	none, err := launchSession(ledger.State{}, codex)
	require.NoError(t, err)
	assert.Equal(t, sessionLaunch{}, none, "a provider that takes no id at launch announces its own")
}

// Only the exit the descriptor recognises as the CLI's refusal loses the
// session; it settles what the runner carried and names resume_failed.
func TestLoseRefusedSession_OnlyTheCLIsRefusalLosesTheSession(t *testing.T) {
	refusal := "No conversation found with session ID: S1"
	for name, tc := range map[string]struct {
		exit engineterminal.Exit
		lost bool
	}{
		"the refusal":            {engineterminal.Exit{Code: 1, Screen: "loading\n" + refusal + "\n"}, true},
		"a kill":                 {engineterminal.Exit{Code: 143, Screen: refusal}, false},
		"a crash without a line": {engineterminal.Exit{Code: 1, Screen: "Killed"}, false},
	} {
		t.Run(name, func(t *testing.T) {
			rs := &Runners{ledger: boundTestLedger(t, "claude", "S1"), sessions: newSessionBook()}
			appendPrompt(t, rs.ledger, "req-1", "runner-1")
			watch := resumeWatch{chatID: "chat-1", providerID: "claude", sessionID: "S1", agent: testAgent(t, "claude")}

			rs.loseRefusedSession(t.Context(), watch, "runner-1", tc.exit)

			st, err := rs.ledger.State(t.Context(), "chat-1")
			require.NoError(t, err)
			c, _ := st.Conversation("claude")
			p := promptReceipt(t, rs.ledger, "req-1")
			if !tc.lost {
				assert.Equal(t, ledger.ConversationBound, c.State)
				assert.Equal(t, ledger.PromptStatusPending, p.Status)
				assert.Equal(t, domain.AgentExitExited, rs.sessions.takeCause("runner-1"))
				return
			}
			assert.Equal(t, ledger.ConversationLost, c.State)
			assert.Equal(t, ledger.PromptStatusSettled, p.Status)
			assert.Equal(t, ledger.ReasonSessionLost, p.Reason)
			assert.Equal(t, domain.AgentExitResumeFailed, rs.sessions.takeCause("runner-1"))
		})
	}
}

// A launch that resumed nothing has nothing to lose, whatever it exits with.
func TestLoseRefusedSession_AFreshLaunchLosesNothing(t *testing.T) {
	rs := &Runners{ledger: boundTestLedger(t, "claude", "S1"), sessions: newSessionBook()}
	rs.loseRefusedSession(t.Context(), resumeWatch{}, "runner-1",
		engineterminal.Exit{Code: 1, Screen: "No conversation found with session ID: S1"})

	st, err := rs.ledger.State(t.Context(), "chat-1")
	require.NoError(t, err)
	c, _ := st.Conversation("claude")
	assert.Equal(t, ledger.ConversationBound, c.State)
}
