package agent_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

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

const (
	crowbarDeliveredPrompt = "Launch exactly one general-purpose subagent with the Agent tool. …"
	composerTypedPrompt    = "say only the word ACK"
)

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

func TestIngestHook_UserPrompt_HarnessInjectionNeverBecomesTheChatTitle(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.bc.reset()

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": taskNotificationPrompt})))

	assert.NotContains(t, f.chat(t, chatID).Title, "task-notification")

	assert.Equal(t, []string{"turn_started"}, f.bcKinds(t))

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": composerTypedPrompt})))
	assert.Equal(t, "say only the word ACK", f.chat(t, chatID).Title)
}

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

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Contains(t, handoff, "claude harness (injected, NOT the user):")
}
