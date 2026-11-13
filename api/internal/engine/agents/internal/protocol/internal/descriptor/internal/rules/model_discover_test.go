package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func withAPIDiscover(d *spec.Descriptor) *spec.Descriptor {
	d = withAPITransport(withModelDiscover(valid()))
	d.Runtime.API = spec.APISpec{
		Protocol: "jsonrpc2", Delivery: spec.APIDeliveryWebsocket, Format: spec.APIFormatJSONRPCText,
		Connect: []string{"probe", "attach"}, ConnectMap: spec.ConnectMap{URL: "url"},
		Handshake: spec.HandshakeSpec{Call: "hello"},
	}
	d.Model.Discover.Command = nil
	d.Model.Discover.API = &spec.ModelDiscoverAPI{Call: "catalog.list"}
	d.Events = map[string]spec.EventSpec{
		"prompt": {Fresh: []spec.CallStep{{Call: "session.new", Send: map[string]any{"pick": "{model}"}}}},
	}
	return d
}

func TestModelDiscover_AcceptsAnAPICallOnADialedProvider(t *testing.T) {
	require.NoError(t, rules.Apply(withAPIDiscover(valid())))
}

func TestModelDiscover_RejectsTheBrokenAPIShapes(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate  func(*spec.Descriptor)
		wantMsg string
	}{
		"no call":       {func(d *spec.Descriptor) { d.Model.Discover.API.Call = "" }, "api.call is required"},
		"and a command": {func(d *spec.Descriptor) { d.Model.Discover.Command = []string{"x"} }, "exactly one of command or api"},
		"stdio delivery": {func(d *spec.Descriptor) {
			d.Runtime.API.Delivery = spec.APIDeliveryStdio
			d.Model.APIApply = []spec.InjectStep{passArg(map[string]any{"arg": "-c", "value": `m="{model}"`})}
		}, "needs websocket delivery"},
		"templated connect": {func(d *spec.Descriptor) { d.Runtime.API.Connect = []string{"probe", "{cwd}"} }, "fixed connect"},
		"selection unsent":  {func(d *spec.Descriptor) { d.Events = nil }, "no step sends {model}"},
	} {
		t.Run(name, func(t *testing.T) {
			d := withAPIDiscover(valid())
			tc.mutate(d)
			err := rules.Apply(d)
			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestModelCatalog_RejectsABrokenIDSplit(t *testing.T) {
	for name, split := range map[string]*spec.ModelIDSplit{
		"no separator": {Names: []string{"a", "b"}},
		"one name":     {Sep: ":", Names: []string{"a"}},
		"repeated":     {Sep: ":", Names: []string{"a", "a"}},
	} {
		t.Run(name, func(t *testing.T) {
			d := withAPIDiscover(valid())
			d.Model.IDSplit = split
			err := rules.Apply(d)
			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), "model.id_split")
		})
	}
	d := withAPIDiscover(valid())
	d.Model.IDSplit = &spec.ModelIDSplit{Sep: ":", Names: []string{"a", "b"}}
	require.NoError(t, rules.Apply(d))
}
