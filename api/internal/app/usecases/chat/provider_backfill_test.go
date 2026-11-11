package chat_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// §7-A target 2: domain.Chat.ProviderID is the ONE owner of "as whom does this
// chat come back". The pre-audit daemon answered it from the runner history
// first, so on upgrade every row is restated once, at boot, to the provider that
// history names newest — after which every reader reads the field.

// legacyChat is a chat row that stores stored as its provider, and records in
// history the provider its legacy runner history names.
func legacyChat(t *testing.T, f testFixture, history map[string]string, stored, named string) string {
	t.Helper()
	chatID := uuid.NewString()
	_, err := f.chats.Create(f.ctx, agentchat.CreateInput{
		ID: chatID, WorkspaceID: "ws1", Type: domain.ChatTypeChat, ProviderID: stored, Now: time.Now(),
	})
	require.NoError(t, err)
	history[chatID] = named
	f.wait()
	require.Equal(t, stored, f.chat(t, chatID).ProviderID, "precondition: what the row stores")
	return chatID
}

func TestBackfill_ALegacyChatLearnsItsProviderAtBoot(t *testing.T) {
	f := newFixture(t)
	history := map[string]string{}
	chatID := legacyChat(t, f, history, "", "codex")

	restate(t, f, history)

	assert.Equal(t, "codex", f.chat(t, chatID).ProviderID)
	got, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, got, live.ID)
	assert.Equal(t, "codex", live.ProviderID, "Resume brings back the backfilled vendor, never a guess")
}

func TestBackfill_AChatThatNeverRanStaysUnknown(t *testing.T) {
	f := newFixture(t)
	chatID := uuid.NewString()
	_, err := f.chats.Create(f.ctx, agentchat.CreateInput{
		ID: chatID, WorkspaceID: "ws1", Type: domain.ChatTypeChat, Now: time.Now(),
	})
	require.NoError(t, err)
	f.wait()

	restate(t, f, map[string]string{})

	assert.Empty(t, f.chat(t, chatID).ProviderID)
	_, err = f.usecase.ResumeChat(f.ctx, chatID)
	require.ErrorIs(t, err, agentusecase.ErrChatProviderUnknown)
}

// A row base wrote may store one vendor while its history — which base read
// first — names another. The history wins, as it did before the upgrade.
func TestBackfill_AStaleStoredProviderIsRestatedToTheHistory(t *testing.T) {
	f := newFixture(t)
	history := map[string]string{}
	chatID := legacyChat(t, f, history, "claude", "codex")

	restate(t, f, history)
	f.wait()
	restate(t, f, history)

	assert.Equal(t, "codex", f.chat(t, chatID).ProviderID)
}

// StaleProviders is the restate's postcondition: what it lists is exactly what
// a restate still has to do.
func TestStaleProviders_ListsWhatTheRestateLeaves(t *testing.T) {
	f := newFixture(t)
	history := map[string]string{}
	stale := legacyChat(t, f, history, "claude", "codex")
	legacyChat(t, f, history, "claude", "claude")

	before, err := f.usecase.StaleProviders(f.ctx, history)
	require.NoError(t, err)
	assert.Equal(t, []string{stale}, before)

	restate(t, f, history)

	after, err := f.usecase.StaleProviders(f.ctx, history)
	require.NoError(t, err)
	assert.Empty(t, after, "the event log answers at once, without waiting on the read model")
}

func restate(t *testing.T, f testFixture, history map[string]string) {
	t.Helper()
	skipped, err := f.usecase.RestateProvidersFromHistory(f.ctx, history)
	require.NoError(t, err)
	require.Zero(t, skipped)
}
