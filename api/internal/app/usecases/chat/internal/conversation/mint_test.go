package conversation_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

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

func TestMintChat_NoProviderAskedForRecordsNone(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	chatID, err := f.conversations.MintChat(t.Context(), "", "ws-1", "", "")
	require.NoError(t, err)

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Empty(t, chat.ProviderID)
}

func TestMintChat_PersistsTheLandingSurface(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	chatID, err := f.conversations.MintChat(t.Context(), "", "ws-1", "", "terminal")
	require.NoError(t, err)

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "terminal", chat.Surface)
}

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

	*f.defaultLevel = "full-auto"

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

func title(t *testing.T, f fixture, chatID string) string {
	t.Helper()
	f.settle()
	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	return chat.Title
}
