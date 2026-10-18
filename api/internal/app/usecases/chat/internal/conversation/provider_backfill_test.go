package conversation_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// TestBackfillProviders_ASwitchNewerThanTheLastPlacementWins: vendor-b was
// switched to and left dormant without ever running, so the durable switch
// interruption is its sole trace. The backfill records vendor-b, and
// ChatProviderID (what SubmitPrompt resolves through) reads it.
func TestBackfillProviders_ASwitchNewerThanTheLastPlacementWins(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
	require.NoError(t, err)

	_, err = f.runners.Start(t.Context(), agentrunner.StartInput{
		RunnerID:        chatID + "-runner",
		WorkspaceID:     "ws-1",
		ProviderID:      "vendor-a",
		TerminalSession: "term-1",
		ChatID:          chatID,
		Now:             time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	_, err = f.runners.Exit(t.Context(), chatID+"-runner", time.Unix(3, 0).UTC())
	require.NoError(t, err)

	require.NoError(t, f.activity.Interrupt(t.Context(), chatID, "int-1",
		engineagents.InterruptProviderSwitched, "vendor-b", time.Unix(2, 0).UTC()))
	require.NoError(t, f.activity.ResolveInterruption(t.Context(), chatID, "int-1",
		engineagents.InterruptProviderSwitched, "vendor-b", time.Unix(2, 0).UTC()))
	f.settle()
	skipped, err := f.conversations.RestateProvidersFromHistory(t.Context())
	require.NoError(t, err)
	require.Zero(t, skipped)
	f.settle()

	providerID, err := f.conversations.ChatProviderID(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "vendor-b", providerID,
		"the switch interruption is the only trace vendor-b left, and must win over vendor-a's older placement")
}
