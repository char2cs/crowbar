package agent_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
)

// exitDuringFork makes the spawned CLI die in the ONE window that produces
// ErrProviderExitedDuringStartup: after the hook barrier is installed, before
// CreateCommand has a terminal-session id to hand back, and therefore before
// recordRunner can commit the runner row.
//
// The REST tests cannot reach this deterministically. There the stub provider is
// `true`, and whether its death is observed before the row commits is a genuine
// race between the OS reaping a process and a sqlite write — one the CLI wins
// under load, which is honest behaviour (spawnRunner says so at length) but makes
// a status assertion a coin flip. Driving onExit from inside the fake commander
// makes the interleaving exact instead of hoped for, which is the same reason
// duringFork exists at all.
func exitDuringFork(f testFixture) {
	f.term.duringForkCall = func(c commandCall) { c.onExit() }
}

// TestRegression_ProviderExitingBeforeItsRunnerRowCommitsIsRefused pins the
// sentinel. A vendor CLI that dies on startup — an expired login, a broken
// install, a CLI that refuses this workspace — is a DEPENDENCY failure the user
// can act on, so it must travel as ErrProviderExitedDuringStartup (which the REST
// layer answers 424 for) rather than as an opaque wrapped error mapping to 500.
func TestRegression_ProviderExitingBeforeItsRunnerRowCommitsIsRefused(t *testing.T) {
	f := newFixture(t)
	exitDuringFork(f)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")

	require.ErrorIs(t, err, agentusecase.ErrProviderExitedDuringStartup,
		"a CLI that died before its runner row existed has not started")
	assert.Contains(t, err.Error(), "exited during startup",
		"the refusal has to name what went wrong: the user's own CLI died, and only they can fix it")
}

// TestRegression_ProviderExitingBeforeItsRunnerRowCommitsLeavesNoChat is the other
// half, and the half that was missing: the refusal was returned and the chat was
// created anyway.
//
// A refusal that also creates a chat is the record contradicting its own response.
// The caller is told the spawn failed; the state says a chat exists, and the user
// is left holding one they never made — with no CLI behind it and no idea where it
// came from. The chat is written mid-spawn (recordRunner), so only a refusal
// DOWNSTREAM of that point can leave one behind, which is exactly this one.
func TestRegression_ProviderExitingBeforeItsRunnerRowCommitsLeavesNoChat(t *testing.T) {
	f := newFixture(t)
	exitDuringFork(f)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
	require.ErrorIs(t, err, agentusecase.ErrProviderExitedDuringStartup)
	f.wait()

	chats, listErr := f.usecase.ListChatsByWorkspace(f.ctx, "ws1")
	require.NoError(t, listErr)
	assert.Empty(t, chats, "a refused spawn must not leave a chat behind")

	// And the workspace still works: the refusal cost it nothing.
	f.term.duringForkCall = nil
	chatID, _ := f.spawn(t, "claude")
	chats, listErr = f.usecase.ListChatsByWorkspace(f.ctx, "ws1")
	require.NoError(t, listErr)
	require.Len(t, chats, 1, "a working provider still creates exactly one chat")
	assert.Equal(t, chatID, chats[0].ID)
}
