package chat_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
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
