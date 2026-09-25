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
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// §7-A target 2: domain.Chat.ProviderID is the ONE owner of "as whom does this
// chat come back". The pre-audit daemon answered it from the runner history
// first, so on upgrade every row is restated once, at boot, to what its history
// names — after which every reader (Resume, the DTO, the selection path) reads
// the field and nothing re-derives it.

// legacyChat is a chat row that predates the provider field, with a runner of
// providerID once placed on it and since exited: the placement history is the
// only thing that still names its vendor.
func legacyChat(t *testing.T, f testFixture, stored, providerID string) string {
	t.Helper()
	chatID := uuid.NewString()
	_, err := f.chats.Create(f.ctx, agentchat.CreateInput{
		ID: chatID, WorkspaceID: "ws1", Type: domain.ChatTypeChat, ProviderID: stored, Now: time.Now(),
	})
	require.NoError(t, err)
	runnerID := uuid.NewString()
	_, err = f.runners.Start(f.ctx, agentrunner.StartInput{
		RunnerID: runnerID, WorkspaceID: "ws1", ProviderID: providerID,
		TerminalSession: "term-legacy-" + runnerID, ChatID: chatID, Now: time.Now(),
	})
	require.NoError(t, err)
	_, err = f.runners.Exit(f.ctx, runnerID, time.Now())
	require.NoError(t, err)
	f.wait()
	require.Equal(t, stored, f.chat(t, chatID).ProviderID, "precondition: what the row stores")
	return chatID
}

func TestBackfill_ALegacyChatLearnsItsProviderAtBoot(t *testing.T) {
	f := newFixture(t)
	chatID := legacyChat(t, f, "", "codex")

	require.True(t, f.usecase.RestateProvidersFromHistory(f.ctx))

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

	require.True(t, f.usecase.RestateProvidersFromHistory(f.ctx))

	assert.Empty(t, f.chat(t, chatID).ProviderID)
	_, err = f.usecase.ResumeChat(f.ctx, chatID)
	require.ErrorIs(t, err, agentusecase.ErrChatProviderUnknown)
}

// A row base wrote may store one vendor while its history — which base read
// first — names another. The history wins, as it did before the upgrade.
func TestBackfill_AStaleStoredProviderIsRestatedToTheHistory(t *testing.T) {
	f := newFixture(t)
	chatID := legacyChat(t, f, "claude", "codex")

	require.True(t, f.usecase.RestateProvidersFromHistory(f.ctx))
	f.wait()
	require.True(t, f.usecase.RestateProvidersFromHistory(f.ctx), "a second run changes nothing")

	assert.Equal(t, "codex", f.chat(t, chatID).ProviderID)
}
