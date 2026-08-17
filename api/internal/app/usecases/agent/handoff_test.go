package agent_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/config"
)

func TestAssembleHandoff_WrapsLedgerEntriesInPreamble(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	turn(t, f, runnerID, "claude", "first turn transcript")
	turn(t, f, runnerID, "claude", "second turn transcript")

	got, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)

	// AssembleHandoff wraps the rendered ledger in the CONFIGURED handoff_wrapper
	// (config-driven, not a hardcoded literal): assert against the actual configured
	// template split around {conversation}, so this test tracks config-driven behavior
	// rather than re-hardcoding it.
	wrapper := config.GetPrompts().HandoffWrapper
	pre, post, ok := strings.Cut(wrapper, "{conversation}")
	require.True(t, ok, "handoff_wrapper must contain {conversation}")
	assert.True(t, strings.HasPrefix(got, pre))
	assert.True(t, strings.HasSuffix(got, post))
	assert.Contains(t, got, "first turn transcript")
	assert.Contains(t, got, "second turn transcript")
	// Both entries must appear, in append order.
	assert.Less(t, strings.Index(got, "first turn transcript"), strings.Index(got, "second turn transcript"))
}

func TestAssembleHandoff_EmptyLedger_ReturnsEmptyString(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	got, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestAssembleHandoff_UnknownChat_ReturnsError(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.AssembleHandoff(f.ctx, "does-not-exist")
	require.Error(t, err)
}

// resumeCodexWithGap drives the one path where Crowbar's own context document comes
// back at it as a user prompt: codex switched away and then BACK. A resumed codex
// cannot be reached through any config channel (verified against 0.139.0), so the gap
// is delivered as a positional — which IS codex's first user message, and which its
// user-prompt hook duly reports. Returns the new codex runner and the exact document it
// was spawned with.
func resumeCodexWithGap(t *testing.T, f testFixture) (chatID, codexRunnerID, injected string) {
	t.Helper()

	chatID, codexRunner := f.spawn(t, "codex")
	f.announce(t, codexRunner, "sid-codex")
	turn(t, f, codexRunner, "codex", "codex said this before leaving")

	claudeRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()
	turn(t, f, claudeRunner, "claude", "claude spoke while codex was away")

	codexRunnerID, err = f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	argv := f.term.calls[2].argv
	injected = argv[len(argv)-1]

	// A POINTER, not a transcript: Crowbar already holds the conversation, and
	// pasting it into the chat is a wall of text the user scrolls past on every
	// switch. The pointer names the chat and the tool that reads it, so an agent
	// fetches exactly as much as it needs.
	require.Contains(t, injected, "[Crowbar]", "the resumed codex must be handed the pointer as a positional: %v", argv)
	require.Contains(t, injected, "get_chat_log", "the pointer must name the tool that reads the record: %q", injected)
	require.Contains(t, injected, chatID, "the pointer must name the chat to read: %q", injected)
	require.NotContains(t, injected, "claude spoke while codex was away",
		"the pointer must NOT carry the transcript — that is the wall of text it replaces")
	return chatID, codexRunnerID, injected
}

// TestResumeCodex_InjectedPointer_IsNotRecordedAsAUserTurn: the pointer Crowbar hands a
// resumed codex comes straight back through its user-prompt hook — that is Crowbar's own
// message echoing, not something the user said. Recording it would put Crowbar's
// plumbing into the conversation the next handoff is built from.
func TestResumeCodex_InjectedPointer_IsNotRecordedAsAUserTurn(t *testing.T) {
	f := newFixture(t)

	chatID, codexRunner, injected := resumeCodexWithGap(t, f)

	require.NoError(t, f.usecase.IngestHook(f.ctx, codexRunner, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": injected})))
	f.wait()

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)

	assert.NotContains(t, handoff, "[Crowbar]",
		"Crowbar's injected pointer must never be recorded as a user turn:\n%s", handoff)
	assert.Contains(t, handoff, "claude spoke while codex was away", "the real conversation is still recorded")
	assert.Contains(t, handoff, "codex said this before leaving")
}

// TestResumeCodex_InjectedPointer_StillOpensTheTurn: suppressing the echo from the
// LEDGER must not suppress the turn itself — the CLI really is answering it, so the chat
// must read as Working (the workspace spinner overlay depends on it).
func TestResumeCodex_InjectedPointer_StillOpensTheTurn(t *testing.T) {
	f := newFixture(t)

	chatID, codexRunner, injected := resumeCodexWithGap(t, f)

	require.NoError(t, f.usecase.IngestHook(f.ctx, codexRunner, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": injected})))
	f.wait()

	assert.True(t, f.chat(t, chatID).Working, "the CLI is answering the gap: the chat must read as working")
}

// TestResumeCodex_UserRetypesThePointer_IsRecorded: the suppression is one-shot and
// scoped to the runner the message was injected into, so a user who genuinely sends that
// same text later is still recorded — the guard must never become a permanent content
// filter.
func TestResumeCodex_UserRetypesThePointer_IsRecorded(t *testing.T) {
	f := newFixture(t)

	chatID, codexRunner, injected := resumeCodexWithGap(t, f)

	for range 2 {
		require.NoError(t, f.usecase.IngestHook(f.ctx, codexRunner, "codex", "user_prompt",
			mustJSON(t, map[string]any{"prompt": injected})))
		f.wait()
	}

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(handoff, "[Crowbar]"),
		"the FIRST echo is dropped; a second, genuinely user-sent copy is recorded:\n%s", handoff)
}

// TestRunnerExit_ForgetsTheInjectedContext: the echo guard is per-spawn state about a
// LIVE process. Once the PTY is gone the entry means nothing and must not accumulate —
// it holds a whole handoff document, and a long-lived daemon spawns a lot of CLIs.
func TestRunnerExit_ForgetsTheInjectedContext(t *testing.T) {
	f := newFixture(t)

	_, codexRunner, injected := resumeCodexWithGap(t, f)

	// Precondition: the guard is armed — Crowbar's own document is recognised as an echo.
	require.True(t, f.engine.WasInjected(codexRunner, injected))

	// Re-arm it (the match above consumed it), then kill the PTY.
	f.engine.RecordInjection(codexRunner, injected)
	f.term.exit(t, f.runner(t, codexRunner).TerminalSession)
	f.wait()

	assert.False(t, f.engine.WasInjected(codexRunner, injected),
		"a dead runner's injected context must be forgotten")
}
