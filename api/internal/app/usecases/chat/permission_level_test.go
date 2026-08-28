package chat_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestSetChatPermissionLevel_RejectsAnUnknownLevel(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	err := f.usecase.SetChatPermissionLevel(f.ctx, chatID, permission.Level("yolo"))

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestSetChatPermissionLevel_RejectsAnUnknownChat(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.SetChatPermissionLevel(f.ctx, "never-created", permission.Guarded)

	require.Error(t, err)
}

func TestSetChatPermissionLevel_Succeeds(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.SetChatPermissionLevel(f.ctx, chatID, permission.FullAuto))
}

// TestRegression_SpawnChatSeedsThePermissionLevelFromTheCurrentGlobalDefault
// proves the REAL production chat-creation path — SpawnChat, not MintChat —
// seeds a freshly spawned chat's trust dial from the global default AT CREATION
// TIME (see recordRunner's seed in internal/runner/spawn.go), not merely from a
// live per-request read of whatever the global default happens to be right now.
//
// The global default is changed to Guarded AFTER the spawn, on purpose: a chat
// that only ever fell back to a LIVE read of the global default (never seeded at
// all) would follow that change and hold the permission below, exactly like an
// unseeded chat under Fix 2's own restart-safe fallback. Only a chat truly
// seeded at creation keeps answering from the value that was current when it
// was minted.
//
// Bash is a standard-tier tool (claude.yaml's risk classification), which
// auto-approves at Trusted and holds at Guarded.
func TestRegression_SpawnChatSeedsThePermissionLevelFromTheCurrentGlobalDefault(t *testing.T) {
	f := newFixtureWithPermissionDefault(t, permission.Trusted)
	chatID, runnerID := f.spawn(t, "claude")

	require.NoError(t, f.usecase.SetDefaultPermissionLevel(f.ctx, permission.Guarded))

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	_ = deliver(t, f, runnerID, "claude", engineagents.HookPermission, permissionPayload())

	assert.Empty(t, pendingChoices(t, f, chatID),
		"a standard-tier tool must still auto-approve at the chat's SEEDED level (Trusted), "+
			"even though the global default has since changed to Guarded")

	all, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, all.Choices, 1)
	assert.True(t, all.Choices[0].AutoApproved,
		"the chat must have been seeded at Trusted, the global default AT CREATION TIME, "+
			"not the fixture's own pinned Guarded default nor the since-changed one")
}

// TestRegression_MoveToNewChatSeedsThePermissionLevelFromTheCurrentGlobalDefault
// proves the THIRD chat-creation path — moveToNewChat, reached whenever a live
// CLI announces a session id Crowbar's history doesn't recognise (an everyday
// mid-conversation /clear or /new, exercised the same way as
// TestRegression_RenameResolvesChatAtCallTime in chat_test.go) — also seeds the
// new chat's trust dial from the global default in effect AT THE MOMENT the
// chat is minted, not from a live per-request read of whatever the global
// default happens to be right now.
//
// The global default is changed to Guarded AFTER the runner has already moved
// to the new chat, on purpose: a chat that only ever fell back to a LIVE read
// of the global default (never seeded at all) would follow that change and
// hold the permission below, exactly like an unseeded chat under Fix 2's own
// restart-safe fallback. Only a chat truly seeded at creation keeps answering
// from the value that was current when it was minted.
//
// Bash is a standard-tier tool (claude.yaml's risk classification), which
// auto-approves at Trusted and holds at Guarded.
func TestRegression_MoveToNewChatSeedsThePermissionLevelFromTheCurrentGlobalDefault(t *testing.T) {
	f := newFixtureWithPermissionDefault(t, permission.Trusted)
	chatA, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sA")
	f.announce(t, runnerID, "sB") // /clear — moveToNewChat mints a brand-new chat

	moved := f.runner(t, runnerID)
	chatB := moved.CurrentChatID
	require.NotEqual(t, chatA, chatB, "precondition: the runner really did move to a new chat")

	require.NoError(t, f.usecase.SetDefaultPermissionLevel(f.ctx, permission.Guarded))

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	_ = deliver(t, f, runnerID, "claude", engineagents.HookPermission, permissionPayload())

	assert.Empty(t, pendingChoices(t, f, chatB),
		"a standard-tier tool must still auto-approve at chat B's SEEDED level (Trusted), "+
			"even though the global default has since changed to Guarded")

	all, err := f.usecase.ReadActivity(f.ctx, chatB, 0, 0)
	require.NoError(t, err)
	require.Len(t, all.Choices, 1)
	assert.True(t, all.Choices[0].AutoApproved,
		"chat B must have been seeded at Trusted, the global default AT moveToNewChat's "+
			"CREATION TIME, not the since-changed Guarded default")
}

// TestRegression_ManyAutoApprovedPermissionsInOneTurnNeverHitMaxOpenPerTurn
// reproduces the gap a live review of a45f7930 found: switching Claude's
// spawn to --permission-mode default means every standard/sensitive-tier
// tool call now fires a real PermissionRequest, where under the old
// (buggy) --permission-mode auto it almost never did. Each one opens both
// a choice AND an interruption (observation.go's HookPermission case), but
// only the choice was ever closed on resolve — the interruption lived until
// turn-close, so a turn with more than domain.MaxOpenPerTurn (64) gated
// tool calls would silently drop every choice past the cap: OpenChoice
// still returns nil (Validate never checks the cap), but the choice is
// never added to the aggregate's map, so the immediately-following
// AnswerChoice fails "no longer pending" and the prompt hangs at the
// provider's hook timeout instead of auto-approving.
//
// 70 exceeds the cap on its own; if resolving a choice also resolves its
// paired interruption (this fix), the count never climbs high enough to
// matter and all 70 auto-approve.
func TestRegression_ManyAutoApprovedPermissionsInOneTurnNeverHitMaxOpenPerTurn(t *testing.T) {
	const gatedCalls = 70
	f := newFixtureWithPermissionDefault(t, permission.Trusted)
	chatID, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})

	for i := range gatedCalls {
		payload := permissionPayload()
		payload["prompt_id"] = fmt.Sprintf("p-%02d", i)
		_ = deliver(t, f, runnerID, "claude", engineagents.HookPermission, payload)
	}

	pending := pendingChoices(t, f, chatID)
	assert.Empty(t, pending,
		"every one of %d gated calls must auto-approve at Trusted — none may be left "+
			"stranded pending because MaxOpenPerTurn's cap silently dropped it", gatedCalls)

	all, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, all.Choices, gatedCalls, "every gated call must have opened its own choice")
	for _, c := range all.Choices {
		assert.True(t, c.AutoApproved, "choice %s must have auto-approved, not been dropped", c.ID)
		assert.Equal(t, domain.ChoiceResolutionAnswered, c.Resolution,
			"choice %s must resolve as answered, not linger unresolved past the turn", c.ID)
	}
}

// TestRegression_APermissionWithNoPromptIDStillPairsItsChoiceAndInterruption
// reproduces the gap a live review of 7fb802c8 found: Codex's own
// permission mapping (descriptors-v3/codex.yaml) never sets prompt_id, so
// choiceID and interruptionID each fell through to their OWN independent
// fallbackID() draw when computed separately — two DIFFERENT ids for what
// should be the same pair, so the auto-approve path's interruption resolve
// named an interruption that was never opened, and the choice/interruption
// pair never actually shrank back to baseline. This payload has no
// prompt_id at all (Claude's own descriptor would too, if asked the same
// way), reproducing the same shape without needing a live Codex session:
// the fix must mint ONE id per gated call and thread it through both the
// interruption and the choice, not derive two independently.
func TestRegression_APermissionWithNoPromptIDStillPairsItsChoiceAndInterruption(t *testing.T) {
	const gatedCalls = 70
	f := newFixtureWithPermissionDefault(t, permission.Trusted)
	chatID, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})

	for range gatedCalls {
		payload := permissionPayload()
		delete(payload, "prompt_id")
		_ = deliver(t, f, runnerID, "claude", engineagents.HookPermission, payload)
	}

	pending := pendingChoices(t, f, chatID)
	assert.Empty(t, pending,
		"every one of %d gated calls must auto-approve at Trusted even with no prompt_id at "+
			"all — none may be left stranded because its choice and interruption were paired "+
			"under two DIFFERENT ids", gatedCalls)

	all, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, all.Choices, gatedCalls, "every gated call must have opened its own choice")
	for _, c := range all.Choices {
		assert.True(t, c.AutoApproved, "choice %s must have auto-approved, not been dropped", c.ID)
		assert.Equal(t, domain.ChoiceResolutionAnswered, c.Resolution,
			"choice %s must resolve as answered, not linger unresolved past the turn", c.ID)
	}
	require.Len(t, all.Interruptions, gatedCalls,
		"every gated call must have opened its own interruption too, not just its choice")
	for _, i := range all.Interruptions {
		assert.NotNil(t, i.ResolvedAt,
			"interruption %s must have closed alongside its choice, not been left stranded "+
				"under an id nothing ever resolves", i.ID)
	}
}
