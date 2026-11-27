package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func openingSessions(send map[string]any) *spec.Descriptor {
	d := withWebsocket(valid())
	d.Events["prompt"] = spec.EventSpec{
		Fresh: []spec.CallStep{{Call: "session.create", Send: send, Capture: map[string]string{"session_id": "id"}}},
	}
	return d
}

func TestContextChannel_AcceptsAnyDeclaredWayToHandASessionItsContext(t *testing.T) {
	for name, build := range map[string]func() *spec.Descriptor{
		"the creating call sends it": func() *spec.Descriptor {
			return openingSessions(map[string]any{"instructions": "{context}"})
		},
		"it leads the first prompt": func() *spec.Descriptor {
			d := openingSessions(map[string]any{"cwd": "{cwd}"})
			d.Inject = []spec.InjectSpec{{At: "context", PromptLead: "{context}"}}
			return d
		},
		"a call hands it over": func() *spec.Descriptor {
			d := openingSessions(map[string]any{"cwd": "{cwd}"})
			d.Inject = []spec.InjectSpec{{At: "context", Call: "inject_items"}}
			return d
		},
		"a provider that opens no session": func() *spec.Descriptor { return withWebsocket(valid()) },
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, rules.Apply(build()))
		})
	}
}

func TestContextChannel_RejectsASessionNothingHandsItsContext(t *testing.T) {
	err := rules.Apply(openingSessions(map[string]any{"cwd": "{cwd}"}))

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "{context}")
	assert.Contains(t, err.Error(), "prompt_lead")
}

func TestContextChannel_RejectsAPromptLeadThatCannotLeadAnything(t *testing.T) {
	for name, tc := range map[string]struct {
		step    spec.InjectSpec
		wantMsg string
	}{
		"another moment":    {spec.InjectSpec{At: "mcp", PromptLead: "{context}"}, "only for at: context"},
		"alongside a call":  {spec.InjectSpec{At: "context", Call: "inject_items", PromptLead: "{context}"}, "pick one"},
		"naming no context": {spec.InjectSpec{At: "context", PromptLead: "a fixed line"}, "never names {context}"},
	} {
		t.Run(name, func(t *testing.T) {
			d := openingSessions(map[string]any{"instructions": "{context}"})
			d.Inject = []spec.InjectSpec{tc.step}

			err := rules.Apply(d)

			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}
