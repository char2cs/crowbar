package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func withType(d *spec.Descriptor) *spec.Descriptor {
	d = withPromptSubmit(d, spec.DeliveryRestartTUI)
	d.Presentation.PromptSubmit.Type = &spec.PromptTypeSpec{
		Paste:        spec.PasteBracketed,
		Submit:       "\r",
		SkipPrefixes: []string{"!"},
		Composer:     spec.PromptComposerSpec{RowPrefix: "❯ "},
	}
	return d
}

func TestPromptSubmit_AcceptsADeclaredType(t *testing.T) {
	require.NoError(t, rules.Apply(withType(valid())))
}

func TestPromptSubmit_AcceptsATypeWithNoSkipPrefixes(t *testing.T) {
	d := withType(valid())
	d.Presentation.PromptSubmit.Type.SkipPrefixes = nil
	require.NoError(t, rules.Apply(d))
}

func TestPromptSubmit_AWhitespaceOnlyRowPrefixIsStillAPrefix(t *testing.T) {
	d := withType(valid())
	d.Presentation.PromptSubmit.Type.Composer.RowPrefix = " "
	require.NoError(t, rules.Apply(d))
}

func TestPromptSubmit_RejectsTheBrokenTypeShapes(t *testing.T) {
	testCases := []struct {
		name    string
		mutate  func(*spec.Descriptor)
		wantMsg string
	}{
		{"unknown paste", func(d *spec.Descriptor) { d.Presentation.PromptSubmit.Type.Paste = "raw" }, "type.paste"},
		{"no paste", func(d *spec.Descriptor) { d.Presentation.PromptSubmit.Type.Paste = "" }, "type.paste"},
		{"no submit key", func(d *spec.Descriptor) { d.Presentation.PromptSubmit.Type.Submit = "" }, "type.submit"},
		{"no composer row prefix", func(d *spec.Descriptor) { d.Presentation.PromptSubmit.Type.Composer.RowPrefix = "" }, "type.composer.row_prefix"},
		{"an unknown ghost", func(d *spec.Descriptor) { d.Presentation.PromptSubmit.Type.Composer.Ghost = "grey" }, "type.composer.ghost"},
		{"an empty skip prefix", func(d *spec.Descriptor) { d.Presentation.PromptSubmit.Type.SkipPrefixes = []string{""} }, "type.skip_prefixes"},
		{"no restart fallback", func(d *spec.Descriptor) { d.Presentation.PromptSubmit.Strategy = "" }, "unsupported strategy"},
		{"no resume fallback", func(d *spec.Descriptor) { d.Session.Resume = nil }, "requires session.resume"},
		{"no fresh fallback", func(d *spec.Descriptor) { d.Presentation.PromptSubmit.Fresh = nil }, "fresh is empty"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := withType(valid())
			tc.mutate(d)
			err := rules.Apply(d)
			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}
