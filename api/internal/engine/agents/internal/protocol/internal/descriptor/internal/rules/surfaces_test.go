package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestApply_DeclaringNoSurfacesIsValid(t *testing.T) {
	require.NoError(t, rules.Apply(valid()))
}

func TestApply_AcceptsDeclaredSurfaces(t *testing.T) {
	d := valid()
	d.Surfaces = map[string]spec.SurfaceSpec{
		spec.SurfaceChat:     {Channel: spec.ChannelHooks},
		spec.SurfaceTerminal: {Channel: spec.ChannelHooks},
	}

	assert.NoError(t, rules.Apply(d))
}

func TestApply_RejectsAnUnknownSurfaceName(t *testing.T) {
	d := valid()
	d.Surfaces = map[string]spec.SurfaceSpec{"bogus": {Channel: spec.ChannelHooks}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "unknown surface")
}

func TestApply_RejectsAnUnknownSurfaceChannel(t *testing.T) {
	d := valid()
	d.Surfaces = map[string]spec.SurfaceSpec{spec.SurfaceChat: {Channel: "carrier-pigeon"}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "channel")
}

func TestApply_RejectsAnAPIChannelSurfaceWhenRuntimeDeclaresNoAPITransport(t *testing.T) {
	d := valid()
	d.Surfaces = map[string]spec.SurfaceSpec{spec.SurfaceChat: {Channel: spec.ChannelAPI}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "no api transport")
}

func TestApply_RejectsATerminalSurfaceWhenTheDescriptorStructurallyHasNone(t *testing.T) {
	d := valid()
	d.Runtime.Transport = "api"
	d.Runtime.API = spec.APISpec{Protocol: "jsonrpc2"}
	d.Surfaces = map[string]spec.SurfaceSpec{spec.SurfaceTerminal: {Channel: spec.ChannelAPI}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "no terminal")
}

func TestApply_RejectsTerminalStartHereOnAnAPIChannelWithoutHotswap(t *testing.T) {
	d := valid()
	d.Runtime.Transport = "api"
	d.Runtime.API = spec.APISpec{Protocol: "jsonrpc2", Attach: []string{"probe", "resume", "{session_id}"}}
	d.Runtime.Hotswap = false
	d.Surfaces = map[string]spec.SurfaceSpec{
		spec.SurfaceTerminal: {Channel: spec.ChannelAPI, StartHere: true},
	}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "start_here")
}

func TestApply_AcceptsTerminalStartHereOnAnAPIChannelWithHotswap(t *testing.T) {
	d := valid()
	d.Runtime.Transport = "api"
	d.Runtime.API = spec.APISpec{Protocol: "jsonrpc2", Attach: []string{"probe", "resume", "{session_id}"}}
	d.Runtime.Hotswap = true
	d.Surfaces = map[string]spec.SurfaceSpec{
		spec.SurfaceTerminal: {Channel: spec.ChannelAPI, StartHere: true},
	}

	assert.NoError(t, rules.Apply(d))
}

func TestApply_AcceptsTerminalStartHereOnAHooksChannelWithoutHotswap(t *testing.T) {
	d := valid()
	d.Runtime.Hotswap = false
	d.Surfaces = map[string]spec.SurfaceSpec{
		spec.SurfaceTerminal: {Channel: spec.ChannelHooks, StartHere: true},
	}

	assert.NoError(t, rules.Apply(d))
}

func TestApply_AcceptsTerminalStartHereForAnAPITransportWithAHooksTerminal(t *testing.T) {
	d := valid()
	d.Runtime.Transport = "api"
	d.Runtime.API = spec.APISpec{Protocol: "jsonrpc2", Attach: []string{"probe", "resume", "{session_id}"}}
	d.Runtime.Hotswap = false
	d.Surfaces = map[string]spec.SurfaceSpec{
		spec.SurfaceChat:     {Channel: spec.ChannelAPI, StartHere: true},
		spec.SurfaceTerminal: {Channel: spec.ChannelHooks, StartHere: true},
	}

	assert.NoError(t, rules.Apply(d))
}
