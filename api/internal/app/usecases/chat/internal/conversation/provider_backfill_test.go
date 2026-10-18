package conversation_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// TestBackfillProviders_ASwitchNewerThanTheLastConversationWins migrates the old
// three-projection scan's hardest case into the one-time backfill: vendor-b was
// switched to and left dormant WITHOUT ever binding a session, so only vendor-a's
// OLDER conversation exists and the durable switch interruption is the sole trace
// of vendor-b. The backfill must record vendor-b, and ChatProviderID (the path
// SetChatSelection, SwitchProvider and SubmitPrompt resolve through) reads it.
func TestBackfillProviders_ASwitchNewerThanTheLastConversationWins(t *testing.T) {
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
	_, err = f.runners.BindSession(t.Context(), chatID+"-runner", "sess-1", false, time.Unix(1, 0).UTC(), "", "")
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
		"the switch interruption is the only trace vendor-b left, and must win over vendor-a's older conversation")
}
