package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestPatch_AUnifiedSourceStandsAloneOrNotAtAll(t *testing.T) {
	for _, tc := range []struct {
		name  string
		patch spec.PatchSpec
		want  string
	}{
		{"alone", spec.PatchSpec{Unified: "result.diff"}, ""},
		{"beside hunks", spec.PatchSpec{Unified: "result.diff", Items: "result.hunks"}, "unified excludes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := valid()
			d.Events[spec.HookToolPost] = spec.EventSpec{Patch: &tc.patch}
			err := rules.Apply(d)
			if tc.want == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
