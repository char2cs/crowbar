package promptorigin_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/promptorigin"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

const taskNotification = `<task-notification>
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
	crowbarDelivered = "Launch exactly one general-purpose subagent with the Agent tool. …"
	composerTyped    = "say only the word ACK"
)

func declaring() *spec.Descriptor {
	return &spec.Descriptor{
		ID: "claude",
		InjectedPrompts: []spec.InjectedPromptSpec{
			{Kind: spec.InjectedPromptTaskNotification, Needle: "<task-notification>"},
			{Needle: "[Subagent hand-back]"},
		},
	}
}

func TestMatch_RecognisesTheMeasuredTaskNotification(t *testing.T) {
	got, ok := promptorigin.Match(declaring(), taskNotification)

	assert.True(t, ok)
	assert.Equal(t, spec.InjectedPromptTaskNotification, got.Kind)
}

func TestMatch_RealUserPromptsAreTheUsers(t *testing.T) {
	for name, prompt := range map[string]string{
		"crowbar positional delivery": crowbarDelivered,
		"typed into the composer":     composerTyped,
	} {
		t.Run(name, func(t *testing.T) {
			_, ok := promptorigin.Match(declaring(), prompt)
			assert.False(t, ok)
		})
	}
}

func TestMatch_AProviderDeclaringNothingNeverMatches(t *testing.T) {
	_, ok := promptorigin.Match(&spec.Descriptor{ID: "codex"}, taskNotification)

	assert.False(t, ok)
}

func TestMatch_IsPrefixAnchoredAndNotSqueezed(t *testing.T) {
	testCases := []struct {
		name   string
		prompt string
		want   bool
	}{
		{
			name:   "a human quoting the tag mid-sentence is still the human",
			prompt: "why is <task-notification> showing up in my chat log?",
			want:   false,
		},
		{
			name:   "the tag without its brackets is prose, not markup",
			prompt: "task notification handling is broken",
			want:   false,
		},
		{
			name:   "the frame under one wrapping tag is still the harness",
			prompt: "<wrapper from=\"x\">\n[Subagent hand-back] The report follows:\n  done\n</wrapper>",
			want:   true,
		},
		{
			name:   "markup the human typed is not a frame",
			prompt: "<div>why is the hand-back missing?</div>",
			want:   false,
		},
		{
			name:   "leading whitespace does not hide the document",
			prompt: "\n  " + taskNotification,
			want:   true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := promptorigin.Match(declaring(), tc.prompt)
			assert.Equal(t, tc.want, ok)
		})
	}
}

func TestMatch_ASpecificDeclarationWinsRegardlessOfOrder(t *testing.T) {
	d := &spec.Descriptor{
		ID: "claude",
		InjectedPrompts: []spec.InjectedPromptSpec{
			{Needle: "<task-"},
			{Kind: spec.InjectedPromptTaskNotification, Needle: "<task-notification>"},
		},
	}

	got, ok := promptorigin.Match(d, taskNotification)

	assert.True(t, ok)
	assert.Equal(t, spec.InjectedPromptTaskNotification, got.Kind)
}

func TestMatch_AnAnchoredMatchOpensWhatNoNeedleCanSpell(t *testing.T) {
	var d spec.Descriptor
	require.NoError(t, yaml.Unmarshal([]byte(`
id: probe
injected_prompts:
  - kind: task_notification
    match: '^\[IMPORTANT: \d+ background processes completed'
`), &d))
	batch := "[IMPORTANT: 2 background processes completed for this session.\n\n- proc_a: exit_code=3\n]"

	got, ok := promptorigin.Match(&d, batch)
	assert.True(t, ok)
	assert.Equal(t, spec.InjectedPromptTaskNotification, got.Kind)
	_, ok = promptorigin.Match(&d, "\n  [IMPORTANT: 12 background processes completed")
	assert.True(t, ok, "leading whitespace does not hide the notice")
	_, ok = promptorigin.Match(&d, "<wrapper>\n"+batch+"\n</wrapper>")
	assert.True(t, ok, "the notice under one wrapping tag is still the harness")

	for _, prompt := range []string{
		"please review [IMPORTANT: 2 background processes completed",
		"[IMPORTANT: two background processes completed",
		"say only the word ACK",
	} {
		_, ok := promptorigin.Match(&d, prompt)
		assert.False(t, ok, prompt)
	}
}

func TestMatch_EmptyInputsAreTheUsers(t *testing.T) {
	_, ok := promptorigin.Match(nil, taskNotification)
	assert.False(t, ok)

	_, ok = promptorigin.Match(declaring(), "")
	assert.False(t, ok)

	_, ok = promptorigin.Match(declaring(), "   \n\t ")
	assert.False(t, ok)
}
