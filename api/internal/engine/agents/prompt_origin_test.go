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

func TestMatchInjectedPrompt_AnythingThatIsNotADescriptorBackedAgentIsTheUser(t *testing.T) {
	_, ok := agents.MatchInjectedPrompt(nil, injectedTaskNotification)

	assert.False(t, ok)
}
