package runner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// stubRunnerStoreWithSuccessor answers LiveRunnerForChat with a DIFFERENT,
// already-live runner — simulating a successor (a provider switch, a retry)
// having already taken the chat by the time an outgoing runner's belated
// exit is reconciled. The embedded nil interface panics on anything else.
type stubRunnerStoreWithSuccessor struct {
	agentrunner.EventStore
	successor agents.Runner
}

func (s stubRunnerStoreWithSuccessor) LiveRunnerForChat(
	context.Context, string,
) (agents.Runner, error) {
	return s.successor, nil
}

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

// TestRegression_CloseAbandonedTurn_SalvagesEvenWhenASuccessorAlreadyOwnsTheChat
// pins the exact gap live-reproduced 2026-08-31: an outgoing runner's own
// already-streamed text must be salvaged into the ledger EVEN when a
// successor has already taken the chat by the time this runner's belated
// exit is reconciled (SIGTERM is not synchronous — see closeAbandonedTurn's
// own doc comment). `rs.activity` and `rs.chats` are left nil on purpose: the
// "someone else owns this chat" guard must still stop THOSE — a panic on
// either proves this fix did not also un-gate the turn-state abandon it must
// not touch once a successor is live.
func TestRegression_CloseAbandonedTurn_SalvagesEvenWhenASuccessorAlreadyOwnsTheChat(t *testing.T) {
	successor := agents.Runner{ID: "runner-new"}
	outgoing := agents.Runner{ID: "runner-old"}
	spy := &spyAbandonMessageForRunner{}

	rs := &Runners{
		runnerStore: stubRunnerStoreWithSuccessor{successor: successor},
		turns:       spy,
	}

	rs.closeAbandonedTurn(context.Background(), "chat-1", outgoing)

	require.Len(t, spy.calls, 1, "the outgoing runner's own stream must still be salvaged")
	assert.Equal(t, "chat-1", spy.calls[0].chatID)
	assert.Equal(t, outgoing, spy.calls[0].runner,
		"salvage must be scoped to the OUTGOING runner, never the successor")
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
