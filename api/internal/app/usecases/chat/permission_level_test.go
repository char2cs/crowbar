package chat_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestSetChatPermissionLevel_RejectsAnUnknownLevel(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	err := f.usecase.SetChatPermissionLevel(f.ctx, chatID, "yolo")

	require.ErrorIs(t, err, problem.ErrInvalidArgument)
}

func TestSetChatPermissionLevel_RejectsAnUnknownChat(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.SetChatPermissionLevel(f.ctx, "never-created", "guarded")

	require.Error(t, err)
}

func TestSetChatPermissionLevel_Succeeds(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.SetChatPermissionLevel(f.ctx, chatID, "full-auto"))
	f.wait()

	chat, err := f.usecase.GetChat(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, "full-auto", chat.PermissionLevel)
}

func TestSetChatPermissionLevel_RejectsALevelTheProviderDoesNotDeclare(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")

	err := f.usecase.SetChatPermissionLevel(f.ctx, chatID, "guarded")

	require.ErrorIs(t, err, problem.ErrInvalidArgument)
}

func TestRegression_SpawnChatSeedsThePermissionLevelFromTheCurrentGlobalDefault(t *testing.T) {
	f := newFixtureWithPermissionDefault(t, "trusted")
	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.SetDefaultPermissionLevel(f.ctx, "guarded"))

	chat, err := f.usecase.GetChat(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, "trusted", chat.PermissionLevel,
		"the chat must have been seeded at trusted, the global default AT CREATION TIME, "+
			"not the fixture's own pinned guarded default nor the since-changed one")
}

func TestRegression_ManyAnsweredPermissionsInOneTurnNeverHitMaxOpenPerTurn(t *testing.T) {
	const gatedCalls = 70
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})

	for i := range gatedCalls {
		payload := permissionPayload()
		payload["prompt_id"] = fmt.Sprintf("p-%02d", i)
		_ = deliver(t, f, runnerID, "claude", engineagents.HookPermission, payload)

		pending := pendingChoices(t, f, chatID)
		require.Len(t, pending, 1, "gated call %d must not be stranded", i)
		require.NoError(t, f.usecase.AnswerChoice(f.ctx, chatID, pending[0].ID, []string{"allow"}, "", nil))
		f.wait()
	}

	assert.Empty(t, pendingChoices(t, f, chatID))
	all, err := f.activity.Choices(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, all, gatedCalls, "every gated call must have opened its own choice")
	for _, i := range all {
		assert.NotEmpty(t, i.Resolution,
			"interruption %s must have closed alongside its answered choice, not been left "+
				"stranded until turn-close", i.ID)
	}
}

func TestRegression_APermissionWithNoPromptIDStillPairsItsChoiceAndInterruption(t *testing.T) {
	const gatedCalls = 70
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})

	for i := range gatedCalls {
		payload := permissionPayload()
		delete(payload, "prompt_id")
		_ = deliver(t, f, runnerID, "claude", engineagents.HookPermission, payload)

		pending := pendingChoices(t, f, chatID)
		require.Len(t, pending, 1, "gated call %d must not be stranded", i)
		require.NoError(t, f.usecase.AnswerChoice(f.ctx, chatID, pending[0].ID, []string{"allow"}, "", nil))
		f.wait()
	}

	assert.Empty(t, pendingChoices(t, f, chatID))
	all, err := f.activity.Choices(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, all, gatedCalls, "every gated call must have opened its own choice")
	for _, i := range all {
		assert.NotEmpty(t, i.Resolution,
			"interruption %s must have closed alongside its answered choice, not been left "+
				"stranded under an id nothing ever resolves", i.ID)
	}
}
