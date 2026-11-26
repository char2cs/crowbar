package agents_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

const injectedTaskNotification = `<task-notification>
<task-id>aa3b60603214670cc</task-id>
<tool-use-id>toolu_01CZ…</tool-use-id>
<output-file>…</output-file>
<status>completed</status>
<summary>Agent "Reply with PONG" finished</summary>
<note>A task-notification fires each time this agent stops with no live background children of its own. …</note>
<result>PONG</result>
<usage><subagent_tokens>18471</subagent_tokens><tool_uses>0</tool_uses><duration_ms>1337</duration_ms></usage>
</task-notification>`

func TestMatchInjectedPrompt_ReadsTheShippedClaudeDeclaration(t *testing.T) {
	got, ok := agents.MatchInjectedPrompt(get(t, "claude"), injectedTaskNotification)

	assert.True(t, ok)

	assert.Equal(t, "task_notification", got.Kind)
	assert.Equal(t, "<task-notification>", got.Needle,
		"the matched needle travels back so a content-based decision can be audited")
}

func TestMatchInjectedPrompt_ClaudeHandBackIsHarnessText(t *testing.T) {
	got, ok := agents.MatchInjectedPrompt(get(t, "claude"),
		"[Subagent hand-back]The text below is the final report of a subagent this session delegated to.")
	assert.True(t, ok)
	assert.Equal(t, "[Subagent hand-back]", got.Needle)
}

func TestMatchInjectedPrompt_RealUserPromptsStayTheUsers(t *testing.T) {
	claude := get(t, "claude")

	for name, prompt := range map[string]string{
		"crowbar positional delivery": "Launch exactly one general-purpose subagent with the Agent tool. …",
		"typed into the composer":     "say only the word ACK",
	} {
		t.Run(name, func(t *testing.T) {
			_, ok := agents.MatchInjectedPrompt(claude, prompt)
			assert.False(t, ok)
		})
	}
}

func TestMatchInjectedPrompt_ADeclaringlessProviderAnswersNo(t *testing.T) {
	_, ok := agents.MatchInjectedPrompt(get(t, "codex"), injectedTaskNotification)

	assert.False(t, ok)
}

func TestMatchInjectedPrompt_AsyncDelegationBatchIsHarnessText(t *testing.T) {
	hermes := get(t, "hermes")

	got, ok := agents.MatchInjectedPrompt(hermes,
		"[ASYNC DELEGATION BATCH COMPLETE — deleg_03104a1c]\nA background fan-out unit you dispatched earlier has finished.")
	assert.True(t, ok)
	assert.Equal(t, "[ASYNC DELEGATION BATCH COMPLETE", got.Needle)

	for name, prompt := range map[string]string{
		"typed into the composer": "say only the word ACK",
		"quoting the marker":      "What does [ASYNC DELEGATION BATCH COMPLETE — deleg_1] mean?",
		"lowercase marker":        "[async delegation batch complete — deleg_1]",
	} {
		t.Run(name, func(t *testing.T) {
			_, ok := agents.MatchInjectedPrompt(hermes, prompt)
			assert.False(t, ok)
		})
	}
}

func TestMatchInjectedPrompt_ABackgroundProcessBatchIsHarnessText(t *testing.T) {
	hermes := get(t, "hermes")

	got, ok := agents.MatchInjectedPrompt(hermes,
		"[IMPORTANT: 3 background processes completed for this session.\n\n- proc_a: exit_code=0, reason=exited\n]")
	assert.True(t, ok)
	assert.Equal(t, "task_notification", got.Kind)
	assert.Empty(t, got.Needle, "an anchored match has no literal prefix to report")

	_, ok = agents.MatchInjectedPrompt(hermes, "please review [IMPORTANT: 3 background processes completed")
	assert.False(t, ok)
}

func TestMatchInjectedPrompt_AnythingThatIsNotADescriptorBackedAgentIsTheUser(t *testing.T) {
	_, ok := agents.MatchInjectedPrompt(nil, injectedTaskNotification)

	assert.False(t, ok)
}
