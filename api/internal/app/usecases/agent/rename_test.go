package agent_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// TestSpawnChat_InjectsTitleInstruction guards that a fresh SpawnChat injects
// the configured title instruction (expanded with {chatid}) as a true
// system-prompt document via the descriptor's system_prompt_inject steps —
// claude's is the --append-system-prompt flag.
func TestSpawnChat_InjectsTitleInstruction(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.Equal(t, 1, f.term.callCount())
	call := f.term.calls[0]
	doc := argAfter(t, call.argv, "--append-system-prompt")
	require.NotEmpty(t, doc)
	assert.Contains(t, doc, "chat rename "+chatID)
}

// TestSpawnChat_Codex_InjectsTitleInstructionViaAgentsFile is codex's
// counterpart: codex has no per-invocation system-prompt flag, so its
// system_prompt_inject step writes the expanded title instruction to
// $CODEX_HOME/AGENTS.md instead — never as a positional initial prompt.
func TestSpawnChat_Codex_InjectsTitleInstructionViaAgentsFile(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "codex")
	require.NoError(t, err)

	require.Equal(t, 1, f.term.callCount())
	call := f.term.calls[0]

	for _, a := range call.argv {
		assert.NotContains(t, a, "chat rename "+chatID, "title must not be injected as a codex positional arg")
	}

	var codexHome string
	for _, kv := range call.env {
		if v, ok := strings.CutPrefix(kv, "CODEX_HOME="); ok {
			codexHome = v
		}
	}
	require.NotEmpty(t, codexHome, "CODEX_HOME must be set in the spawned env")

	data, err := os.ReadFile(filepath.Join(codexHome, "AGENTS.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "chat rename "+chatID)
}

// TestSwitchProvider_DoesNotInjectTitleInstruction guards the injectTitle=false
// side: SwitchProvider must never inject the title instruction (only the ledger
// handoff, via the descriptor's handoff_inject mechanism).
func TestSwitchProvider_DoesNotInjectTitleInstruction(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	require.Equal(t, 2, f.term.callCount())
	for _, a := range f.term.calls[1].argv {
		assert.NotContains(t, a, "chat rename "+chatID)
	}
}
