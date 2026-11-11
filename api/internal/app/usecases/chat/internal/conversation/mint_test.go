package conversation_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// A minted chat is DORMANT: it exists and is readable with no CLI in sight,
// which is the whole reason the record is separable from the runner lifecycle.
func TestMintChat_CreatesAReadableDormantChat(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	chatID, err := f.conversations.MintChat(t.Context(), "", "ws-1", "", "")
	require.NoError(t, err)
	require.NotEmpty(t, chatID)

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "ws-1", chat.WorkspaceID)
	assert.Empty(t, chat.Title)
}

// The caller names the chat it is drawing; a second mint under that id is the
// aggregate's refusal, not a second chat.
func TestMintChat_ATakenIDIsAnIDConflict(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	id := uuid.NewString()

	chatID, err := f.conversations.MintChat(t.Context(), id, "ws-1", "", "")
	require.NoError(t, err)
	assert.Equal(t, id, chatID)

	_, err = f.conversations.MintChat(t.Context(), id, "ws-1", "", "")
	assert.Equal(t, problem.IDConflict, problem.CodeOf(err))
}

// A chat records the provider it is born on at its mint, before any CLI runs:
// a dormant chat with no switch marker has nothing else that names it.
func TestRegression_MintChat_RecordsTheProviderItIsBornOn(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	chatID, err := f.conversations.MintChat(t.Context(), "", "ws-1", "quietvendor", "")
	require.NoError(t, err)

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "quietvendor", chat.ProviderID,
		"no CLI has run yet, and the chat can already say what it runs")
}

// A row minted with no vendor in mind — a placeholder that will never carry a
// CLI — records none, so "" stays the distinguishable answer for a chat that has
// genuinely never had a provider rather than one whose provider was lost.
func TestMintChat_NoProviderAskedForRecordsNone(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	chatID, err := f.conversations.MintChat(t.Context(), "", "ws-1", "", "")
	require.NoError(t, err)

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Empty(t, chat.ProviderID)
}

// The landing surface is durable: every respawn rebuilds the process, and a
// chat born on one surface must come back on it.
func TestMintChat_PersistsTheLandingSurface(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	chatID, err := f.conversations.MintChat(t.Context(), "", "ws-1", "", "terminal")
	require.NoError(t, err)

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "terminal", chat.Surface)
}

// Omitting it stays the provider's own default landing — byte-identical to
// every chat minted before this field existed.
func TestMintChat_NoSurfaceAskedForStaysTheProvidersDefault(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	chatID, err := f.conversations.MintChat(t.Context(), "", "ws-1", "", "")
	require.NoError(t, err)

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Empty(t, chat.Surface)
}

func TestMintChat_SeedsThePermissionLevelFromTheCurrentGlobalDefault(t *testing.T) {
	t.Parallel()
	f := newFixture(t, stubLineage{})
	*f.defaultLevel = "trusted"

	chatID, err := f.conversations.MintChat(t.Context(), "", "ws-1", "", "")
	require.NoError(t, err)
	f.settle()

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "trusted", chat.PermissionLevel)
}

// A chat's stored level is the display snapshot taken at mint; a chat never
// explicitly pinned spawns under the CURRENT global default, which its
// selection resolves at every spawn.
func TestMintChat_TheSeededFieldIsAFrozenSnapshotButChatSelectionTracksTheLiveDefault(t *testing.T) {
	t.Parallel()
	f := newFixture(t, stubLineage{})
	conversations := f.conversations

	chatID, err := conversations.MintChat(t.Context(), "", "ws-1", "", "")
	require.NoError(t, err)
	f.settle()
	chat, err := conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	require.Equal(t, "guarded", chat.PermissionLevel)
	require.False(t, chat.PermissionLevelExplicit, "a plain mint is never an explicit pick")

	*f.defaultLevel = "full-auto" // the global default changes AFTER this chat was minted

	chat, err = conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "guarded", chat.PermissionLevel,
		"the STORED/display field is a snapshot taken at mint and must not drift on its own")

	sel, err := conversations.ChatSelection(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "full-auto", sel.PermissionLevel,
		"but a chat that was never explicitly pinned must SPAWN under the CURRENT global "+
			"default, not the value frozen in the field above — this is the actual fix")
}

// title reads the chat's projected title once its read model has settled.
func title(t *testing.T, f fixture, chatID string) string {
	t.Helper()
	f.settle()
	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	return chat.Title
}
