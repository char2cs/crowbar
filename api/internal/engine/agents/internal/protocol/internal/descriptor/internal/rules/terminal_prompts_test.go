package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestApply_AcceptsDeclaredTerminalPrompts(t *testing.T) {
	d := valid()
	d.TerminalPrompts = []spec.TerminalPromptSpec{
		{Kind: spec.TerminalPromptTrust, Needle: "I trust this folder"},
		{Needle: "Enter to confirm"},
	}

	assert.NoError(t, rules.Apply(d))
}

func TestApply_RejectsAnUnknownTerminalPromptKind(t *testing.T) {
	d := valid()
	d.TerminalPrompts = []spec.TerminalPromptSpec{{Kind: "worksapce_trust", Needle: "x"}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "unknown kind")
}

func TestApply_RejectsAContentFreeNeedle(t *testing.T) {
	d := valid()
	d.TerminalPrompts = []spec.TerminalPromptSpec{{Needle: "· ⏎ ›"}}

	require.ErrorIs(t, rules.Apply(d), rules.ErrInvalidDescriptor)
}

func TestApply_RejectsAnEmptyNeedle(t *testing.T) {
	d := valid()
	d.TerminalPrompts = []spec.TerminalPromptSpec{{Kind: spec.TerminalPromptTrust}}

	require.ErrorIs(t, rules.Apply(d), rules.ErrInvalidDescriptor)
}

func TestApply_DeclaringNoTerminalPromptsIsValid(t *testing.T) {
	assert.NoError(t, rules.Apply(valid()))
}

func TestApply_RejectsAnIncompleteOrDuplicateTerminalPromptAnswer(t *testing.T) {
	for name, answers := range map[string][]spec.TerminalPromptAnswerSpec{
		"missing keys":      {{ID: "accept", Label: "Trust"}},
		"duplicate id":      {{ID: "a", Label: "A", Keys: "\r"}, {ID: "a", Label: "B", Keys: "\r"}},
		"option, no cursor": {{ID: "a", Label: "A", Keys: "\r", Option: "Yes"}},
	} {
		d := valid()
		d.TerminalPrompts = []spec.TerminalPromptSpec{{Needle: "x", Answers: answers}}

		require.ErrorIs(t, rules.Apply(d), rules.ErrInvalidDescriptor, name)
	}
}
