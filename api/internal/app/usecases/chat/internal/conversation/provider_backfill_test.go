package conversation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The legacy history's newest provider for a chat is restated onto the chat's
// own field; a chat the history agrees with, or does not name, is untouched.
func TestRestateProvidersFromHistory_TheHistorysNewestProviderWins(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	switched, err := f.conversations.MintChat(t.Context(), "ws-1", "vendor-a", "")
	require.NoError(t, err)
	agreed, err := f.conversations.MintChat(t.Context(), "ws-1", "vendor-a", "")
	require.NoError(t, err)
	unnamed, err := f.conversations.MintChat(t.Context(), "ws-1", "vendor-a", "")
	require.NoError(t, err)
	f.settle()
	newest := map[string]string{switched: "vendor-b", agreed: "vendor-a"}

	stale, err := f.conversations.StaleProviders(t.Context(), newest)
	require.NoError(t, err)
	assert.Equal(t, []string{switched}, stale)

	skipped, err := f.conversations.RestateProvidersFromHistory(t.Context(), newest)
	require.NoError(t, err)
	require.Zero(t, skipped)
	f.settle()

	for chatID, want := range map[string]string{switched: "vendor-b", agreed: "vendor-a", unnamed: "vendor-a"} {
		providerID, err := f.conversations.ChatProviderID(t.Context(), chatID)
		require.NoError(t, err)
		assert.Equal(t, want, providerID, chatID)
	}
	stale, err = f.conversations.StaleProviders(t.Context(), newest)
	require.NoError(t, err)
	assert.Empty(t, stale, "nothing is left for the step to restate")
}
