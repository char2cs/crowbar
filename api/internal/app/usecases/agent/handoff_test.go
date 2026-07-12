package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/config"
)

func TestAssembleHandoff_WrapsLedgerEntriesInPreamble(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	appendAssistantTurn(t, f, segID, "claude", "sid-1", "first turn transcript")
	appendAssistantTurn(t, f, segID, "claude", "sid-1", "second turn transcript")

	got, err := f.usecase.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)

	// AssembleHandoff wraps the rendered ledger in the CONFIGURED
	// handoff_wrapper (config-driven, not a hardcoded literal): assert against
	// the actual configured template split around {conversation}, so this
	// test tracks config-driven behavior rather than re-hardcoding it.
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
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	got, err := f.usecase.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestAssembleHandoff_UnknownChat_ReturnsError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, err := f.usecase.AssembleHandoff(ctx, "does-not-exist")
	require.Error(t, err)
}

// appendAssistantTurn drives a turn_stop hook through IngestHook so a ledger
// entry gets appended for the chat behind segID, via the same path production
// code uses to populate the ledger: the assistant's turn text comes straight
// from the hook payload's last_assistant_message field. It waits for
// quiescence first so IngestHook's chat read observes the chat the spawn/prior
// hooks created (no timing guess — asynx WaitPublish).
func appendAssistantTurn(t *testing.T, f testFixture, segID, provider, sessionID, content string) {
	t.Helper()
	f.wait()
	require.NoError(t, f.usecase.IngestHook(context.Background(), segID, provider, "turn_stop",
		mustJSON(t, map[string]any{
			"session_id":             sessionID,
			"last_assistant_message": content,
		})))
}

// resumeCodexWithGap drives the one path where Crowbar's own context document
// comes back at it as a user prompt: codex switched away and then BACK. A
// resumed codex cannot be reached through any config channel (verified against
// 0.139.0), so the gap is delivered as a positional — which IS codex's first user
// message, and which its user-prompt hook duly reports. Returns the new codex
// segment and the exact document it was spawned with.
func resumeCodexWithGap(t *testing.T, f testFixture) (chatID, codexSegID, injected string) {
	t.Helper()
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "codex")
	require.NoError(t, err)
	f.wait()
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "codex", "session_start",
		mustJSON(t, map[string]any{"session_id": "sid-codex"})))
	appendAssistantTurn(t, f, segID, "codex", "sid-codex", "codex said this before leaving")

	claudeSegID, err := f.usecase.SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()
	appendAssistantTurn(t, f, claudeSegID, "claude", "sid-claude", "claude spoke while codex was away")

	codexSegID, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	argv := f.term.calls[2].argv
	injected = argv[len(argv)-1]

	// A POINTER, not a transcript: the conversation is already on disk, and pasting
	// it into the chat is a wall of text the user has to scroll past on every switch.
	require.Contains(t, injected, "[Crowbar]", "the resumed codex must be handed the pointer as a positional: %v", argv)
	require.Contains(t, injected, "ledger", "the pointer must name the ledger directory: %q", injected)
	require.NotContains(t, injected, "claude spoke while codex was away",
		"the pointer must NOT carry the transcript — that is the wall of text it replaces")
	return chatID, codexSegID, injected
}

// TestResumeCodex_InjectedPointer_IsNotRecordedAsAUserTurn: the pointer Crowbar
// hands a resumed codex comes straight back through its user-prompt hook — that is
// Crowbar's own message echoing, not something the user said. Recording it would
// put Crowbar's plumbing into the conversation the next handoff is built from.
func TestResumeCodex_InjectedPointer_IsNotRecordedAsAUserTurn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, codexSegID, injected := resumeCodexWithGap(t, f)

	require.NoError(t, f.usecase.IngestHook(ctx, codexSegID, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": injected})))
	f.wait()

	handoff, err := f.usecase.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)

	assert.NotContains(t, handoff, "[Crowbar]",
		"Crowbar's injected pointer must never be recorded as a user turn:\n%s", handoff)
	assert.Contains(t, handoff, "claude spoke while codex was away", "the real conversation is still recorded")
	assert.Contains(t, handoff, "codex said this before leaving")
}

// TestResumeCodex_InjectedPointer_StillOpensTheTurn: suppressing the echo from the
// LEDGER must not suppress the turn itself — the CLI really is answering it, so
// the chat must read as Working (the workspace spinner overlay depends on it).
func TestResumeCodex_InjectedPointer_StillOpensTheTurn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, codexSegID, injected := resumeCodexWithGap(t, f)

	require.NoError(t, f.usecase.IngestHook(ctx, codexSegID, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": injected})))
	f.wait()

	assert.True(t, f.chat(t, chatID).Working, "the CLI is answering the gap: the chat must read as working")
}

// TestResumeCodex_UserRetypesThePointer_IsRecorded: the suppression is one-shot and
// scoped to the segment the message was injected into, so a user who genuinely
// sends that same text later is still recorded — the guard must never become a
// permanent content filter.
func TestResumeCodex_UserRetypesThePointer_IsRecorded(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, codexSegID, injected := resumeCodexWithGap(t, f)

	for range 2 {
		require.NoError(t, f.usecase.IngestHook(ctx, codexSegID, "codex", "user_prompt",
			mustJSON(t, map[string]any{"prompt": injected})))
		f.wait()
	}

	handoff, err := f.usecase.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(handoff, "[Crowbar]"),
		"the FIRST echo is dropped; a second, genuinely user-sent copy is recorded:\n%s", handoff)
}
