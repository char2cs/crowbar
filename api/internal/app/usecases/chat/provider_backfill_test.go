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
// chat come back". Rows minted before the field existed are backfilled once,
// at boot, from the runner history — after which every reader (Resume, the DTO,
// the selection path) reads the field and nothing re-derives it.

// legacyChat is a chat row that predates the provider field, with a runner of
// providerID once placed on it and since exited: the placement history is the
// only thing that still names its vendor.
func legacyChat(t *testing.T, f testFixture, providerID string) string {
	t.Helper()
	chatID := uuid.NewString()
	_, err := f.chats.Create(f.ctx, agentchat.CreateInput{
		ID: chatID, WorkspaceID: "ws1", Type: domain.ChatTypeChat, Now: time.Now(),
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
	require.Empty(t, f.chat(t, chatID).ProviderID, "precondition: a row that predates the field")
	return chatID
}

func TestBackfill_ALegacyChatLearnsItsProviderAtBoot(t *testing.T) {
	f := newFixture(t)
	chatID := legacyChat(t, f, "codex")

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))

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

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))

	assert.Empty(t, f.chat(t, chatID).ProviderID)
	_, err = f.usecase.ResumeChat(f.ctx, chatID)
	require.ErrorIs(t, err, agentusecase.ErrChatProviderUnknown)
}
