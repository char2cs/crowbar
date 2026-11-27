package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestApply_DeclaringNoPerEventSurfacesIsValid(t *testing.T) {
	require.NoError(t, rules.Apply(valid()))
}

func TestApply_AcceptsKnownPerEventSurfaces(t *testing.T) {
	d := valid()
	e := d.Events[spec.HookTurnStop]
	e.Surfaces = []string{spec.SurfaceChat, spec.SurfaceTerminal}
	d.Events[spec.HookTurnStop] = e

	assert.NoError(t, rules.Apply(d))
}

func TestApply_RejectsAnUnknownPerEventSurface(t *testing.T) {
	d := valid()
	e := d.Events[spec.HookTurnStop]
	e.Surfaces = []string{"underwater"}
	d.Events[spec.HookTurnStop] = e

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "unknown surface")
}
