package spec_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestWireRef_SingleNameIsTheNameItAlwaysWas(t *testing.T) {
	ref := spec.WireRef("turn/completed")

	assert.Equal(t, []string{"turn/completed"}, ref.Names())
	assert.Equal(t, "turn/completed", ref.Name())
	assert.True(t, ref.Has("turn/completed"))
	assert.False(t, ref.Has("turn/started"))
	assert.False(t, ref.Empty())
}

func TestWireRef_AlternationAnswersToEveryName(t *testing.T) {
	ref := spec.WireRef("item/commandExecution/requestApproval || item/fileChange/requestApproval")

	assert.Equal(t, []string{
		"item/commandExecution/requestApproval",
		"item/fileChange/requestApproval",
	}, ref.Names())
	assert.True(t, ref.Has("item/commandExecution/requestApproval"))
	assert.True(t, ref.Has("item/fileChange/requestApproval"))
	assert.False(t, ref.Has("item/permissions/requestApproval"))
}

// Name is what an outbound call and a display label take. Handing either the whole
// alternation would put "a || b" on the wire as a method name.
func TestWireRef_NameIsTheFirstAlternativeNotTheWholeExpression(t *testing.T) {
	ref := spec.WireRef("first/call || second/call")

	assert.Equal(t, "first/call", ref.Name())
}

func TestWireRef_EmptyDeclaresNothing(t *testing.T) {
	assert.True(t, spec.WireRef("").Empty())
	assert.True(t, spec.WireRef("  ").Empty())
	assert.True(t, spec.WireRef("||").Empty())
	assert.Empty(t, spec.WireRef("").Name())
	assert.False(t, spec.WireRef("").Has(""))
}
