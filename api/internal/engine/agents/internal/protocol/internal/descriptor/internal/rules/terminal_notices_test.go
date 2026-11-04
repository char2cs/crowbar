package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestApply_AcceptsADeclaredTerminalNotice(t *testing.T) {
	d := valid()
	d.TerminalNotices = []spec.TerminalNoticeSpec{
		{Kind: spec.TerminalNoticeUsageLimit, Needle: "You've hit your usage limit", EndsTurn: true},
	}

	assert.NoError(t, rules.Apply(d))
}

func TestApply_RejectsAKindlessTerminalNotice(t *testing.T) {
	d := valid()
	d.TerminalNotices = []spec.TerminalNoticeSpec{{Needle: "something went wrong", EndsTurn: true}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "kind is required")
}

func TestApply_RejectsAnUnknownTerminalNoticeKind(t *testing.T) {
	d := valid()
	d.TerminalNotices = []spec.TerminalNoticeSpec{{Kind: "usage_limits", Needle: "x"}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "unknown kind")
}

func TestApply_RejectsAContentFreeNoticeNeedle(t *testing.T) {
	d := valid()
	d.TerminalNotices = []spec.TerminalNoticeSpec{{Kind: spec.TerminalNoticeUsageLimit, Needle: "· ⏎ ›"}}

	require.ErrorIs(t, rules.Apply(d), rules.ErrInvalidDescriptor)
}
