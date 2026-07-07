package agent_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTitle returns chatID's persisted Title for assertions.
func getTitle(t *testing.T, f testFixture, chatID string) string {
	t.Helper()
	c, err := f.usecase.GetChat(context.Background(), chatID)
	require.NoError(t, err)
	return c.Title
}

// TestRenameChat_Precedence guards RenameChat's user>agent>derived precedence
// (see agent.go's doc comment): derived only fills an empty title, agent may
// upgrade a derived title but never a user-locked one, and a user rename
// always wins and locks the title against further agent upgrades.
func TestRenameChat_Precedence(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

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

// TestRenameChat_BroadcastsTitledOnSuccessfulChange guards the "titled"
// broadcast firing only on an actual persisted change, never on a no-op.
func TestRenameChat_BroadcastsTitledOnSuccessfulChange(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.bc.calls = nil

	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "A Title", "user"))
	require.Len(t, f.bc.calls, 1)
	assert.Equal(t, "titled", f.bc.calls[0].kind)
	assert.Equal(t, chatID, f.bc.calls[0].chatID)

	// An empty-title no-op must not broadcast again.
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "", "user"))
	assert.Len(t, f.bc.calls, 1)

	// A derived rename against an already-locked (user) title is also a
	// no-op and must not broadcast.
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "Derived Attempt", "derived"))
	assert.Len(t, f.bc.calls, 1)
}

func TestRenameChat_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	err := f.usecase.RenameChat(ctx, "does-not-exist", "Some Title", "user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rename chat: get")
}

// TestIngestHook_UserPrompt_SetsDerivedTitle guards the first-prompt fallback
// wired into IngestHook's user_prompt case: deriveTitle(prompt) is applied
// via RenameChat(..., "derived"), which only fills an empty title.
func TestIngestHook_UserPrompt_SetsDerivedTitle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "Refactor the auth module to use JWT\nmore detail"})))
	assert.Equal(t, "Refactor the auth module to use JWT", getTitle(t, f, chatID))

	// a later prompt does not overwrite the derived title
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "now do something else"})))
	assert.Equal(t, "Refactor the auth module to use JWT", getTitle(t, f, chatID))
}

// TestSpawnChat_InjectsTitleInstruction guards Step 4: a fresh SpawnChat
// injects the configured title instruction (expanded with {crowbar}/{chatid})
// as the system-prompt document via the descriptor's handoff_inject steps —
// claude's is the --append-system-prompt flag.
func TestSpawnChat_InjectsTitleInstruction(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.Len(t, f.term.calls, 1)
	call := f.term.calls[0]
	doc := argAfter(t, call.argv, "--append-system-prompt")
	require.NotEmpty(t, doc)
	assert.Contains(t, doc, "chat rename "+chatID)
}

// TestSwitchProvider_DoesNotInjectTitleInstruction guards the injectTitle=false
// side of Step 4: SwitchProvider must never inject the title instruction (only
// the ledger handoff), even though the target descriptor's handoff_inject
// mechanism is the same one used for the title on a fresh spawn.
func TestSwitchProvider_DoesNotInjectTitleInstruction(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	require.Len(t, f.term.calls, 2)
	for _, a := range f.term.calls[1].argv {
		assert.NotContains(t, a, "chat rename "+chatID)
	}
}
