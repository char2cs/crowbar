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
	d := valid() // no runtime.api section at all
	d.Surfaces = map[string]spec.SurfaceSpec{spec.SurfaceChat: {Channel: spec.ChannelAPI}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "no api transport")
}

func TestApply_RejectsATerminalSurfaceWhenTheDescriptorStructurallyHasNone(t *testing.T) {
	d := valid()
	d.Runtime.Transport = "api"
	d.Runtime.API = spec.APISpec{Protocol: "jsonrpc2"} // no Attach: no terminal at all
	d.Surfaces = map[string]spec.SurfaceSpec{spec.SurfaceTerminal: {Channel: spec.ChannelAPI}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "no terminal")
}

// An api-channel terminal's only argv (runtime.api.attach) names a {session_id} that does
// not exist until a session is established, and without hotswap nothing renders it at spawn,
// so that one combination cannot serve a brand-new chat.
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

// A hooks-CHANNEL terminal is the descriptor's own spawn.cmd PTY, live from
// the instant it forks and naming no session at all — so it is spawnable into
// from birth whether or not the descriptor hotswaps.
func TestApply_AcceptsTerminalStartHereOnAHooksChannelWithoutHotswap(t *testing.T) {
	d := valid()
	d.Runtime.Hotswap = false
	d.Surfaces = map[string]spec.SurfaceSpec{
		spec.SurfaceTerminal: {Channel: spec.ChannelHooks, StartHere: true},
	}

	assert.NoError(t, rules.Apply(d))
}

// codex's own shape: api transport, attach declared WITHOUT hotswap, and a
// terminal surface fed by the hooks channel. Its native TUI at birth is the
// ordinary `codex` PTY every spawn already forks, not the session-scoped
// `codex resume {id}` SwitchToTerminal needs — so start_here is legal here,
// and the idle-only attach restriction stays where it belongs, on the SWITCH.
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
