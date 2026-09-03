package chat_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestSetChatPermissionLevel_RejectsAnUnknownLevel(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	err := f.usecase.SetChatPermissionLevel(f.ctx, chatID, "yolo")

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
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

// TestSetChatPermissionLevel_RejectsALevelTheProviderDoesNotDeclare proves
// the "if a provider can't reach a level, it isn't offered" rule holds for
// an EXPLICIT user choice, not just what the switcher's own options list
// shows. Codex has no guarded level at all — its CLI's --ask-for-approval
// accepts only on-request/never (see codex.yaml's own permission_levels
// block and its comment on why guarded isn't there).
func TestSetChatPermissionLevel_RejectsALevelTheProviderDoesNotDeclare(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")

	err := f.usecase.SetChatPermissionLevel(f.ctx, chatID, "guarded")

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

// TestRegression_SpawnChatSeedsThePermissionLevelFromTheCurrentGlobalDefault
// proves the REAL production chat-creation path — SpawnChat, not MintChat —
// durably seeds a freshly spawned chat's trust dial from the global default
// AT CREATION TIME (see recordRunner's seed in internal/runner/spawn.go),
// not merely from a live per-request read of whatever the global default
// happens to be right now.
//
// The global default is changed to guarded AFTER the spawn, on purpose: a
// chat that only ever fell back to a LIVE read of the global default (never
// seeded at all) would follow that change. Only a chat truly seeded at
// creation keeps answering from the value that was current when it was
// minted.
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

// TestRegression_MoveToNewChatSeedsThePermissionLevelFromTheCurrentGlobalDefault
// proves the THIRD chat-creation path — moveToNewChat, reached whenever a
// live CLI announces a session id Crowbar's history doesn't recognise (an
// everyday mid-conversation /clear or /new) — also seeds the new chat's
// trust dial from the global default in effect AT THE MOMENT the chat is
// minted, not from a live per-request read of whatever it is right now.
func TestRegression_MoveToNewChatSeedsThePermissionLevelFromTheCurrentGlobalDefault(t *testing.T) {
	f := newFixtureWithPermissionDefault(t, "trusted")
	chatA, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sA")
	f.announce(t, runnerID, "sB") // /clear — moveToNewChat mints a brand-new chat

	moved := f.runner(t, runnerID)
	chatB := moved.CurrentChatID
	require.NotEqual(t, chatA, chatB, "precondition: the runner really did move to a new chat")

	require.NoError(t, f.usecase.SetDefaultPermissionLevel(f.ctx, "guarded"))

	chat, err := f.usecase.GetChat(f.ctx, chatB)
	require.NoError(t, err)
	assert.Equal(t, "trusted", chat.PermissionLevel,
		"chat B must have been seeded at trusted, the global default AT moveToNewChat's "+
			"CREATION TIME, not the since-changed guarded default")
}

// TestRegression_ManyAnsweredPermissionsInOneTurnNeverHitMaxOpenPerTurn
// proves a turn with more than domain.MaxOpenPerTurn (64) gated tool calls
// never strands one past the cap. Each permission opens BOTH a choice AND
// an interruption (observation.go's HookPermission case), and answering the
// choice resolves both — see answers.go's own resolvePermissionInterruption
// — so the running total never climbs high enough to hit the cap. 70
// exceeds the cap on its own if either half of a decided pair is left open.
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
	all, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, all.Choices, gatedCalls, "every gated call must have opened its own choice")
	require.Len(t, all.Interruptions, gatedCalls,
		"every gated call must have opened its own interruption too, not just its choice")
	for _, i := range all.Interruptions {
		assert.NotNil(t, i.ResolvedAt,
			"interruption %s must have closed alongside its answered choice, not been left "+
				"stranded until turn-close", i.ID)
	}
}

// TestRegression_APermissionWithNoPromptIDStillPairsItsChoiceAndInterruption
// reproduces Codex's own shape without a live session: its permission
// mapping never sets prompt_id, so choiceID and interruptionID must be
// minted ONCE per gated call and threaded through to both, not each
// independently drawing its own fallbackID() — two DIFFERENT ids for what
// should be the same pair.
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
	all, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, all.Choices, gatedCalls, "every gated call must have opened its own choice")
	require.Len(t, all.Interruptions, gatedCalls,
		"every gated call must have opened its own interruption too, not just its choice")
	for _, i := range all.Interruptions {
		assert.NotNil(t, i.ResolvedAt,
			"interruption %s must have closed alongside its answered choice, not been left "+
				"stranded under an id nothing ever resolves", i.ID)
	}
}
