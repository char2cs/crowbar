package chat_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTitle returns chatID's persisted Title for assertions. It waits for
// quiescence first so the read observes the latest SetTitle (and so a
// subsequent RenameChat's own read is against caught-up state).
func getTitle(t *testing.T, f testFixture, chatID string) string {
	t.Helper()
	c := f.chat(t, chatID)
	return c.Title
}

// TestRenameChat_Precedence guards RenameChat's user>agent>derived precedence:
// derived only fills an empty title, agent may upgrade a derived title but
// never a user-locked one, and a user rename always wins and locks the title.
func TestRenameChat_Precedence(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	// derived sets when empty, then does not overwrite
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "First Topic", "derived"))
	assert.Equal(t, "First Topic", getTitle(t, f, chatID))
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "Second Topic", "derived"))
	assert.Equal(t, "First Topic", getTitle(t, f, chatID))

	// agent upgrades a derived title
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "Agent Title", "agent"))
	assert.Equal(t, "Agent Title", getTitle(t, f, chatID))

	// user rename wins and locks; agent can no longer clobber
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "User Title", "user"))
	assert.Equal(t, "User Title", getTitle(t, f, chatID))
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "Agent Again", "agent"))
	assert.Equal(t, "User Title", getTitle(t, f, chatID))

	// empty is always a no-op, regardless of source
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "", "user"))
	assert.Equal(t, "User Title", getTitle(t, f, chatID))
}

// TestRenameChat_BroadcastsTitleSetOnSuccessfulChange guards the title_set
// frame firing only on an actual persisted change, never on a no-op — the hub
// projection emits it off the SetTitle event, so a rename that returns before
// issuing SetTitle produces no frame.
func TestRenameChat_BroadcastsTitleSetOnSuccessfulChange(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()
	f.bc.reset()

	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "A Title", "user"))
	assert.Equal(t, []string{"title_set"}, f.bcKinds(t))

	// An empty-title no-op must not broadcast again.
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "", "user"))
	assert.Equal(t, []string{"title_set"}, f.bcKinds(t))

	// A derived rename against an already-locked (user) title is also a no-op
	// and must not broadcast.
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "Derived Attempt", "derived"))
	assert.Equal(t, []string{"title_set"}, f.bcKinds(t))
}

func TestRenameChat_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	err := f.usecase.RenameChat(ctx, "does-not-exist", "Some Title", "user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rename chat: get")
}

// TestRegression_RenameResolvesChatAtCallTime guards the fix for the stale-title
// bug: the OLD title instruction baked the chat id into the CLI's spawn-time
// system prompt, so an agent that titled itself after a /clear or /resume moved
// the runner to a NEW chat but still carried the OLD chat id — and renamed the
// chat it used to be in, not the one the user was looking at.
//
// RenameByRunner resolves runnerID → runner → CurrentChatID at CALL TIME, the
// same resolution IngestHook already uses for every hook (see its doc comment):
// nothing is baked in, so nothing can go stale. This spawns a runner on chat A,
// drives two conversation announcements — the second an unknown session id,
// which the context-move reducer (engine/agent.Decide) resolves to MoveToNew —
// so the runner ends up on a brand-new chat B while chat A still exists
// untouched, and proves the rename lands on B, never on A.
func TestRegression_RenameResolvesChatAtCallTime(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatA, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sA")
	f.announce(t, runnerID, "sB") // /clear — the runner moves to a NEW chat

	require.NoError(t, f.usecase.RenameByRunner(ctx, runnerID, "New Title", "agent"))

	moved := f.runner(t, runnerID)
	require.NotEqual(t, chatA, moved.CurrentChatID, "precondition: the runner really did move off chat A")

	assert.Equal(t, "New Title", getTitle(t, f, moved.CurrentChatID),
		"the title lands on the chat the runner is on NOW")
	assert.NotEqual(t, "New Title", getTitle(t, f, chatA),
		"and NOT on the chat it used to be on")
}

// TestRenameByRunner_UnknownRunner_ReturnsWrappedError guards the error path: a
// rename call naming a runner Crowbar has never heard of (or whose CLI has
// already exited) must fail loudly here — unlike a hook, which silently drops
// an unknown runner, this is a direct call the handler maps to a 404.
func TestRenameByRunner_UnknownRunner_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	err := f.usecase.RenameByRunner(ctx, "does-not-exist", "Some Title", "agent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rename by runner")
}

// TestRenameByRunner_DisplacedRunner_IsANoop guards the one legitimate case
// where a runner's CurrentChatID is empty: it has been taken off its chat and
// is being retired (a switch, an eviction, a deleted chat) but its process has
// not yet died. There is nowhere to write the title, and — mirroring
// handleTurn's identical guard on the hook path — this must be a silent no-op,
// never an error that would otherwise reach the dying CLI's stderr for no
// actionable reason.
func TestRenameByRunner_DisplacedRunner_IsANoop(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.PurgeChat(ctx, mustChatOf(t, f, runnerID)))

	require.NoError(t, f.usecase.RenameByRunner(ctx, runnerID, "New Title", "agent"))
}

// mustChatOf reads back the chat id a freshly spawned runner is placed on.
func mustChatOf(t *testing.T, f testFixture, runnerID string) string {
	t.Helper()
	r := f.runner(t, runnerID)
	require.NotEmpty(t, r.CurrentChatID)
	return r.CurrentChatID
}

// TestIngestHook_UserPrompt_SetsDerivedTitle guards the first-prompt fallback
// wired into IngestHook's user_prompt case: deriveTitle(prompt) is applied via
// RenameChat(..., "derived"), which only fills an empty title.
func TestIngestHook_UserPrompt_SetsDerivedTitle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "Refactor the auth module to use JWT\nmore detail"})))
	assert.Equal(t, "Refactor the auth module to use JWT", getTitle(t, f, chatID))

	// a later prompt does not overwrite the derived title
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "now do something else"})))
	assert.Equal(t, "Refactor the auth module to use JWT", getTitle(t, f, chatID))
}
