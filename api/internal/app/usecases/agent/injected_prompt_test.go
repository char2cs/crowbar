package agent_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// taskNotificationPrompt is the prompt claude 2.1.234's own harness submitted
// through UserPromptSubmit, VERBATIM from a live capture of raw hook stdin
// (2026-08-18): a background subagent was launched with the Agent tool and this is
// the notification the harness fired when it completed. The `…` are the capture's
// own elisions of long ids and paths; nothing else is altered.
//
// It is the real payload rather than a synthetic one on purpose. A fixture shaped
// to fit the needle would prove that the needle matches itself, and the sequence
// this test is named for — a live chat's ledger row recorded as `role: user` —
// began with exactly these bytes.
const taskNotificationPrompt = `<task-notification>
<task-id>aa3b60603214670cc</task-id>
<tool-use-id>toolu_01CZ…</tool-use-id>
<output-file>…</output-file>
<status>completed</status>
<summary>Agent "Reply with PONG" finished</summary>
<note>A task-notification fires each time this agent stops with no live background children of its own. …</note>
<result>PONG</result>
<usage><subagent_tokens>18471</subagent_tokens><tool_uses>0</tool_uses><duration_ms>1337</duration_ms></usage>
</task-notification>`

// The two REAL user prompts from the same measurement session, in the two shapes
// that reach Crowbar: its own delivery (the prompt as a positional argument after
// `--`) and a human typing into claude's composer and pressing Enter.
//
// Neither payload carried claude's documented `source` key, and both had the same
// key set as the notification above. They are the trap: a rule of
// `source == "user"` would have dropped both of these, and its inverse would have
// filed both as the harness's.
const (
	crowbarDeliveredPrompt = "Launch exactly one general-purpose subagent with the Agent tool. …"
	composerTypedPrompt    = "say only the word ACK"
)

// TestRegression_HarnessInjectedPromptIsRecordedAsHarnessNotUser: sequence 26 of a
// live chat was `role: user` carrying a `<task-notification>` document the user
// never wrote — and get_chat_log then served it to sibling agents as something the
// human had said.
//
// It is recorded, NOT dropped: the agent genuinely received this text and its next
// answer refers to it, so a ledger without it leaves a reply with no antecedent.
func TestRegression_HarnessInjectedPromptIsRecordedAsHarnessNotUser(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": taskNotificationPrompt})))
	f.wait()

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, page.Items, 1, "the injected prompt is recorded, never dropped")
	assert.Equal(t, domain.TurnRoleHarness, page.Items[0].Role)
	assert.Equal(t, taskNotificationPrompt, page.Items[0].Text,
		"recorded verbatim: it is the context the next reply answers")
}

// TestRegression_HarnessInjectedPromptStillOpensTheTurn: the CLI really is working
// on it, so the workspace's working overlay must say so — the same reason
// Crowbar's own injected handoff opens a turn without being recorded.
func TestRegression_HarnessInjectedPromptStillOpensTheTurn(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.False(t, f.chat(t, chatID).Working, "a fresh chat is not Working")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": taskNotificationPrompt})))

	working := f.chat(t, chatID)
	assert.True(t, working.Working, "an injected prompt still opens the turn")
	require.NotNil(t, working.CurrentTurnStarted)
}

// TestRegression_RealUserPromptsWithNoSourceKeyStayTheUsers is the other half, and
// the half a naive fix breaks. claude 2.1.234 documents an optional
// `source: user|sdk|system|…` on UserPromptSubmit and emits it on NONE of the three
// paths measured; every payload carries the identical key set
// (session_id, transcript_path, cwd, prompt_id, permission_mode, hook_event_name,
// prompt). So "record only when source == user" would silently drop every message
// a human ever sent.
//
// Absent means the user. Both measured user paths are pinned here.
func TestRegression_RealUserPromptsWithNoSourceKeyStayTheUsers(t *testing.T) {
	for name, prompt := range map[string]string{
		"crowbar positional delivery": crowbarDeliveredPrompt,
		"typed into the composer":     composerTypedPrompt,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)

			chatID, runnerID := f.spawn(t, "claude")

			require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
				mustJSON(t, map[string]any{"prompt": prompt})))
			f.wait()

			page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
			require.NoError(t, err)
			require.Len(t, page.Items, 1)
			assert.Equal(t, domain.TurnRoleUser, page.Items[0].Role)
			assert.Equal(t, prompt, page.Items[0].Text)
		})
	}
}

// TestIngestHook_UserPrompt_ProviderDeclaringNoInjectedPromptsRecordsEverythingAsUser:
// codex declares none, so even the exact claude payload is the user's there. This is
// the degradation story — a provider nobody has measured behaves exactly as it did
// before any of this existed.
func TestIngestHook_UserPrompt_ProviderDeclaringNoInjectedPromptsRecordsEverythingAsUser(
	t *testing.T,
) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": taskNotificationPrompt})))
	f.wait()

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, domain.TurnRoleUser, page.Items[0].Role)
}

// TestIngestHook_UserPrompt_HarnessInjectionNeverBecomesTheChatTitle: a chat named
// after a subagent's completion report is named after nothing its user did. The
// title stays where the user's next real prompt can still claim it.
func TestIngestHook_UserPrompt_HarnessInjectionNeverBecomesTheChatTitle(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.bc.reset()

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": taskNotificationPrompt})))

	assert.NotContains(t, f.chat(t, chatID).Title, "task-notification")
	// Only the turn opens. A derived title would show up here as a `title_set`
	// frame, which is what the user-prompt path emits FIRST when it derives one.
	assert.Equal(t, []string{"turn_started"}, f.bcKinds(t))

	// And the user's own next prompt still gets to name the chat.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": composerTypedPrompt})))
	assert.Equal(t, "say only the word ACK", f.chat(t, chatID).Title)
}

// TestRegression_ChatLogDoesNotServeAHarnessTurnAsTheUsers: get_chat_log is a
// CROSS-AGENT read. A sibling agent asking what this chat's human said was being
// handed the provider harness's own words under the bare word "user", and acting
// on them.
func TestRegression_ChatLogDoesNotServeAHarnessTurnAsTheUsers(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": taskNotificationPrompt})))
	f.wait()

	turns, err := f.usecase.ReadChatLog(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, turns, 1)
	assert.NotEqual(t, "user", turns[0].Speaker)
	assert.Contains(t, turns[0].Speaker, "harness")
	assert.Contains(t, turns[0].Speaker, "NOT the user")

	// The handoff document a joining CLI reads is the same rendering, so it cannot
	// disagree with what get_chat_log said.
	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Contains(t, handoff, "claude harness (injected, NOT the user):")
}
