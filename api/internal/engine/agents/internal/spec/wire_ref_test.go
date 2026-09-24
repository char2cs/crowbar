package spec_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestWireRef_SingleNameIsTheNameItAlwaysWas(t *testing.T) {
	ref := spec.WireRef{"turn/completed"}

	assert.Equal(t, []string{"turn/completed"}, ref.Names())
	assert.Equal(t, "turn/completed", ref.Name())
	assert.True(t, ref.Has("turn/completed"))
	assert.False(t, ref.Has("turn/started"))
	assert.False(t, ref.Empty())
}

func TestWireRef_MultipleNamesAnswersToEveryOne(t *testing.T) {
	ref := spec.WireRef{
		"item/commandExecution/requestApproval",
		"item/fileChange/requestApproval",
	}

	assert.Equal(t, []string{
		"item/commandExecution/requestApproval",
		"item/fileChange/requestApproval",
	}, ref.Names())
	assert.True(t, ref.Has("item/commandExecution/requestApproval"))
	assert.True(t, ref.Has("item/fileChange/requestApproval"))
	assert.False(t, ref.Has("item/permissions/requestApproval"))
}

// Name is what an outbound call and a display label take. Handing either the whole
// list would put every alternative on the wire as one method name.
func TestWireRef_NameIsTheFirstAlternativeNotTheWholeList(t *testing.T) {
	ref := spec.WireRef{"first/call", "second/call"}

	assert.Equal(t, "first/call", ref.Name())
}

func TestWireRef_EmptyDeclaresNothing(t *testing.T) {
	assert.True(t, spec.WireRef(nil).Empty())
	assert.True(t, spec.WireRef{}.Empty())
	assert.True(t, spec.WireRef{""}.Empty())
	assert.True(t, spec.WireRef{"  "}.Empty())
	assert.Empty(t, spec.WireRef(nil).Name())
	assert.False(t, spec.WireRef(nil).Has(""))
}

// --- YAML decoding -----------------------------------------------------------

func TestWireRef_DecodesAPlainScalar(t *testing.T) {
	var ref spec.WireRef
	require.NoError(t, yaml.Unmarshal([]byte(`turn/completed`), &ref))
	assert.Equal(t, spec.WireRef{"turn/completed"}, ref)
}

func TestWireRef_DecodesAListAsMultipleWireNames(t *testing.T) {
	var ref spec.WireRef
	require.NoError(t, yaml.Unmarshal([]byte(`
- item/commandExecution/requestApproval
- item/fileChange/requestApproval
`), &ref))

	assert.Equal(t, []string{
		"item/commandExecution/requestApproval",
		"item/fileChange/requestApproval",
	}, ref.Names())
}

func TestWireRef_RejectsAnEmptyList(t *testing.T) {
	var ref spec.WireRef
	assert.Error(t, yaml.Unmarshal([]byte(`[]`), &ref))
}

// 5.0: `||` is a hard parse error, not a silent pass-through into a literal
// wire name that will never match anything real.
func TestWireRef_RejectsAScalarContainingTheGlyph(t *testing.T) {
	var ref spec.WireRef
	err := yaml.Unmarshal(
		[]byte(`item/commandExecution/requestApproval || item/fileChange/requestApproval`), &ref)
	require.Error(t, err)
	var glyph *spec.AlternationGlyphError
	require.ErrorAs(t, err, &glyph)
}

func TestWireRef_RejectsAListElementContainingTheGlyph(t *testing.T) {
	var ref spec.WireRef
	err := yaml.Unmarshal([]byte(`
- item/commandExecution/requestApproval || item/fileChange/requestApproval
- item/other
`), &ref)
	require.Error(t, err)
	var glyph *spec.AlternationGlyphError
	require.ErrorAs(t, err, &glyph)
}
