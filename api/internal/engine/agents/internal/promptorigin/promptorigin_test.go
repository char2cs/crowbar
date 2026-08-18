package promptorigin_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/promptorigin"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// taskNotification is the harness-injected prompt VERBATIM as claude 2.1.234 sent
// it, captured from raw hook stdin on a live interactive PTY session (2026-08-18)
// by having claude launch a background subagent and recording the notification it
// fired on completion. The `…` are the capture's own elisions of long ids and
// paths; nothing else has been touched.
//
// It is reproduced rather than synthesised on purpose. A fixture written to look
// like the needle proves only that strings.HasPrefix works.
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

// The two REAL user prompts from the same measurement session, in the two shapes
// that reach Crowbar: its own positional-argument delivery, and a human typing
// into claude's composer and pressing Enter. Neither payload carried a `source`
// key, and both had the same key set as the notification above — so these are the
// messages any structural rule would have got wrong.
const (
	crowbarDelivered = "Launch exactly one general-purpose subagent with the Agent tool. …"
	composerTyped    = "say only the word ACK"
)

// declaring mirrors claude.yaml.
func declaring() *spec.Descriptor {
	return &spec.Descriptor{
		ID: "claude",
		InjectedPrompts: []spec.InjectedPromptSpec{
			{Kind: spec.InjectedPromptTaskNotification, Needle: "<task-notification>"},
		},
	}
}

func TestMatch_RecognisesTheMeasuredTaskNotification(t *testing.T) {
	got, ok := promptorigin.Match(declaring(), taskNotification)

	assert.True(t, ok)
	assert.Equal(t, spec.InjectedPromptTaskNotification, got.Kind)
}

// TestMatch_RealUserPromptsAreTheUsers is the trap the whole design turns on: no
// `source` key exists to key off, so anything that is not a declared injection
// must come back as the user's.
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

// TestMatch_AProviderDeclaringNothingNeverMatches is the degradation story: the
// notification itself, put to a descriptor with no declarations, is the user's.
func TestMatch_AProviderDeclaringNothingNeverMatches(t *testing.T) {
	_, ok := promptorigin.Match(&spec.Descriptor{ID: "codex"}, taskNotification)

	assert.False(t, ok)
	assert.False(t, promptorigin.Declared(&spec.Descriptor{ID: "codex"}))
	assert.True(t, promptorigin.Declared(declaring()))
}

// TestMatch_IsPrefixAnchoredAndNotSqueezed states the comparison rule as three
// cases that a termprompt-style matcher would get wrong.
func TestMatch_IsPrefixAnchoredAndNotSqueezed(t *testing.T) {
	testCases := []struct {
		name   string
		prompt string
		want   bool
	}{
		{
			// The failure a substring search would introduce: a person asking about
			// the machinery is a person, and their message must stay theirs.
			name:   "a human quoting the tag mid-sentence is still the human",
			prompt: "why is <task-notification> showing up in my chat log?",
			want:   false,
		},
		{
			// squeeze() (letters and digits only) reduces both sides to
			// "tasknotification", so this is exactly what the reduction would have
			// swallowed — the angle brackets are the entire difference between a
			// document and an English phrase.
			name:   "the tag without its brackets is prose, not markup",
			prompt: "task notification handling is broken",
			want:   false,
		},
		{
			// Leading whitespace is the one thing a delivery channel can add without
			// changing what was said.
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

// TestMatch_ASpecificDeclarationWinsRegardlessOfOrder: "which injection is this"
// and "is this an injection at all" are different answers, and a descriptor
// declaring both must not have the winner decided by listing order.
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

// TestMatch_EmptyInputsAreTheUsers: a nil descriptor and an empty prompt are both
// "nothing to decide from", and the default when there is nothing to decide from
// is always the human.
func TestMatch_EmptyInputsAreTheUsers(t *testing.T) {
	_, ok := promptorigin.Match(nil, taskNotification)
	assert.False(t, ok)

	_, ok = promptorigin.Match(declaring(), "")
	assert.False(t, ok)

	_, ok = promptorigin.Match(declaring(), "   \n\t ")
	assert.False(t, ok)
}
