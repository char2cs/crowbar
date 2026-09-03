package spec_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestInjectStep_DecodesTheSingleKeyForm(t *testing.T) {
	var steps []spec.InjectStep

	require.NoError(t, yaml.Unmarshal([]byte(`
- pass_arg: { arg: "--settings", value: "x" }
- write_file: { path: "/tmp/a", content: "b" }
`), &steps))

	require.Len(t, steps, 2)
	assert.Equal(t, "pass_arg", steps[0].Verb)
	assert.Equal(t, "--settings", steps[0].Args["arg"])
	assert.Equal(t, "write_file", steps[1].Verb)
}

func TestInjectStep_RejectsAnythingButExactlyOneVerb(t *testing.T) {
	testCases := []struct{ name, doc string }{
		{"two verbs", `- {pass_arg: {arg: a}, set_env: {name: b}}`},
		{"no verb", `- {}`},
		{"not a mapping", `- "just a string"`},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var steps []spec.InjectStep
			assert.Error(t, yaml.Unmarshal([]byte(tc.doc), &steps))
		})
	}
}

func TestCloneSteps_IsADeepCopy(t *testing.T) {
	src := []spec.InjectStep{{Verb: "pass_arg", Args: map[string]any{"arg": "a"}}}

	got := spec.CloneSteps(src)
	got[0].Args["arg"] = "MUTATED"

	assert.Equal(t, "a", src[0].Args["arg"])
	assert.Empty(t, spec.CloneSteps(nil))
}

func TestArgString_RendersScalarsWithoutForcingTheCallerToTypeSwitch(t *testing.T) {
	testCases := []struct {
		name string
		in   any
		want string
	}{
		{"string", "a", "a"},
		{"nil", nil, ""},
		{"int", 7, "7"},
		{"bool", true, "true"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, spec.ArgString(tc.in))
		})
	}
}

func TestSlashCatalogSpec_UnsetBoundsFallBackToTheDefaults(t *testing.T) {
	var s spec.SlashCatalogSpec

	assert.Equal(t, spec.DefaultCatalogTimeoutMS, s.EffectiveTimeoutMS())
	assert.Equal(t, spec.DefaultCatalogMaxStdoutBytes, s.EffectiveMaxStdoutBytes())
	assert.Equal(t, spec.DefaultCatalogMaxStderrBytes, s.EffectiveMaxStderrBytes())
	assert.Equal(t, spec.DefaultCatalogMaxItems, s.EffectiveMaxItems())
	assert.Equal(t, spec.DefaultCatalogDetailConcurrency, s.Pipeline.EffectiveDetailConcurrency())
}

func TestSlashCatalogSpec_DeclaredBoundsWin(t *testing.T) {
	s := spec.SlashCatalogSpec{
		TimeoutMS: 1, MaxStdoutBytes: 2, MaxStderrBytes: 3, MaxItems: 4,
		Pipeline: spec.CatalogPipelineSpec{DetailConcurrency: 1},
	}

	assert.Equal(t, 1, s.EffectiveTimeoutMS())
	assert.Equal(t, 2, s.EffectiveMaxStdoutBytes())
	assert.Equal(t, 3, s.EffectiveMaxStderrBytes())
	assert.Equal(t, 4, s.EffectiveMaxItems())
	assert.Equal(t, 1, s.Pipeline.EffectiveDetailConcurrency())
}

func TestTelemetryProbeSpec_UnsetBoundsFallBackToTheDefaults(t *testing.T) {
	var p spec.TelemetryProbeSpec

	assert.Equal(t, spec.DefaultCatalogTimeoutMS, p.EffectiveTimeoutMS())
	assert.Equal(t, spec.DefaultCatalogMaxStdoutBytes, p.EffectiveMaxStdoutBytes())
	assert.Equal(t, spec.DefaultCatalogMaxStderrBytes, p.EffectiveMaxStderrBytes())

	declared := spec.TelemetryProbeSpec{TimeoutMS: 5, MaxStdoutBytes: 6, MaxStderrBytes: 7}
	assert.Equal(t, 5, declared.EffectiveTimeoutMS())
	assert.Equal(t, 6, declared.EffectiveMaxStdoutBytes())
	assert.Equal(t, 7, declared.EffectiveMaxStderrBytes())
}

func TestTerminalNoticeSpec_DecodesTheDeclaredShape(t *testing.T) {
	var d spec.Descriptor
	require.NoError(t, yaml.Unmarshal([]byte(`
terminal_notices:
  - kind: usage_limit
    needle: "You've hit your usage limit"
    ends_turn: true
  - kind: usage_limit
    needle: "informational"
`), &d))

	require.Len(t, d.TerminalNotices, 2)
	assert.Equal(t, spec.TerminalNoticeUsageLimit, d.TerminalNotices[0].Kind)
	assert.Equal(t, "You've hit your usage limit", d.TerminalNotices[0].Needle)
	assert.True(t, d.TerminalNotices[0].EndsTurn)

	assert.False(t, d.TerminalNotices[1].EndsTurn)
}
