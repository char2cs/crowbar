package agents_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// injectedTaskNotification is the prompt claude 2.1.234's harness injected,
// VERBATIM from a live capture of raw hook stdin (2026-08-18): a background
// subagent was launched through the Agent tool and this is the notification it
// fired on completion. The `…` are the capture's own elisions of long ids and
// paths.
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

// TestMatchInjectedPrompt_ReadsTheShippedClaudeDeclaration runs the real embedded
// descriptor, not a hand-built one: the point is that claude.yaml as shipped
// recognises the payload that was actually measured.
func TestMatchInjectedPrompt_ReadsTheShippedClaudeDeclaration(t *testing.T) {
	got, ok := agents.MatchInjectedPrompt(get(t, "claude"), injectedTaskNotification)

	assert.True(t, ok)
	// Spelled out rather than taken from the constant: this pins the value the
	// shipped YAML and the daemon have to agree on, from outside both.
	assert.Equal(t, "task_notification", got.Kind)
	assert.Equal(t, "<task-notification>", got.Needle,
		"the matched needle travels back so a content-based decision can be audited")
}

// TestMatchInjectedPrompt_RealUserPromptsStayTheUsers uses the two measured user
// paths. Neither payload carried a `source` key — see spec.InjectedPromptSpec —
// so a structural rule would have had to guess, and both guesses lose a real
// message or steal one.
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

// TestMatchInjectedPrompt_ADeclaringlessProviderAnswersNo: codex declares none, so
// its every user_prompt is the user's — the behaviour it had before any of this
// existed.
func TestMatchInjectedPrompt_ADeclaringlessProviderAnswersNo(t *testing.T) {
	_, ok := agents.MatchInjectedPrompt(get(t, "codex"), injectedTaskNotification)

	assert.False(t, ok)
}

// TestMatchInjectedPrompt_AnythingThatIsNotADescriptorBackedAgentIsTheUser pins
// the default that makes this a free function: a nil, or a caller's own stub,
// answers "the user's" rather than whatever it felt like answering.
func TestMatchInjectedPrompt_AnythingThatIsNotADescriptorBackedAgentIsTheUser(t *testing.T) {
	_, ok := agents.MatchInjectedPrompt(nil, injectedTaskNotification)

	assert.False(t, ok)
}
