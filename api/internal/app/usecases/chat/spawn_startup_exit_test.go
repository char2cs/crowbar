package chat_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

func exitDuringFork(f testFixture) {
	f.term.duringForkCall = func(c commandCall) { c.onExit() }
}

func TestRegression_ProviderExitingBeforeItsRunnerRowCommitsIsRefused(t *testing.T) {
	f := newFixture(t)
	exitDuringFork(f)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")

	require.ErrorIs(t, err, agentusecase.ErrProviderExitedDuringStartup,
		"a CLI that died before its runner row existed has not started")
	assert.Contains(t, err.Error(), "exited during startup",
		"the refusal has to name what went wrong: the user's own CLI died, and only they can fix it")
}

func TestRegression_ProviderExitingBeforeItsRunnerRowCommitsLeavesNoChat(t *testing.T) {
	f := newFixture(t)
	exitDuringFork(f)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
	require.ErrorIs(t, err, agentusecase.ErrProviderExitedDuringStartup)
	f.wait()

	chats, listErr := f.usecase.ListChatsByWorkspace(f.ctx, "ws1")
	require.NoError(t, listErr)
	assert.Empty(t, chats, "a refused spawn must not leave a chat behind")

	f.term.duringForkCall = nil
	chatID, _ := f.spawn(t, "claude")
	chats, listErr = f.usecase.ListChatsByWorkspace(f.ctx, "ws1")
	require.NoError(t, listErr)
	require.Len(t, chats, 1, "a working provider still creates exactly one chat")
	assert.Equal(t, chatID, chats[0].ID)
}
