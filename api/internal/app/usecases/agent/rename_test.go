package agent_test

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
	assert.Contains(t, doc, "chat rename --project=p1 --workspace=ws1 --repo=r1 "+chatID)
}

// TestSpawnChat_Codex_InjectsTitleInstructionViaDeveloperInstructions is codex's
// counterpart: codex ships no --append-system-prompt, so its context_inject step
// carries the title instruction through the documented `developer_instructions`
// config key — a channel the model reads WITHOUT being given a turn. It must
// never arrive as a positional arg, which IS codex's initial user message: that
// would make the CLI answer Crowbar's instruction instead of waiting for the
// user.
func TestSpawnChat_Codex_InjectsTitleInstructionViaDeveloperInstructions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "codex")
	require.NoError(t, err)

	require.Equal(t, 1, f.term.callCount())
	argv := f.term.calls[0].argv

	doc := configValue(t, argv, "developer_instructions=")
	assert.Contains(t, doc, "chat rename --project=p1 --workspace=ws1 --repo=r1 "+chatID)

	// The instruction must not ALSO be a bare positional (codex's user prompt).
	for i, a := range argv {
		if i > 0 && argv[i-1] == "-c" {
			continue
		}
		assert.NotContains(t, a, "chat rename "+chatID, "title must not be injected as a codex positional arg")
	}
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

// TestSpawnChat_ProjectHome_TitleInstructionOmitsRepoFlag pins the rendering at the
// layer where the empty repo id is actually born: a PROJECT-HOME workspace has no
// repo, so WorktreeDir hands SpawnChat an empty RepoID.
//
// The injected command line must then carry NO --repo flag at all. The old
// `--repo {repo_id}` triple rendered `--repo ` here, and because the instruction is
// a flat line that the agent retypes and the shell word-splits, the empty token
// vanished and pflag consumed `--workspace` as --repo's value — so the rename (and
// every other in-PTY callback) died on project-home workspaces. cmd/crowbar's
// scope_roundtrip_test drives that whole chain; this is the unit-level guard.
func TestSpawnChat_ProjectHome_TitleInstructionOmitsRepoFlag(t *testing.T) {
	f := newFixture(t)
	f.ws.repoID = "" // project-home: no owning repo
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	doc := argAfter(t, f.term.calls[0].argv, "--append-system-prompt")
	assert.Contains(t, doc, "chat rename --project=p1 --workspace=ws1 "+chatID)
	assert.NotContains(t, doc, "--repo",
		"a project-home workspace has no repo id — the flag must be omitted, never left empty")
}
