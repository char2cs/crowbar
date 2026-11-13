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

func TestRichMappings_AnEventMayOnlyMapOntoCrowbarsOwnVocabulary(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event spec.EventSpec
		want  string
	}{
		{"known kind", spec.EventSpec{KindMap: map[string]string{"sh": "execute"}}, ""},
		{"unknown kind", spec.EventSpec{KindMap: map[string]string{"sh": "teleport"}}, "unknown canonical kind"},
		{"unknown shell kind", spec.EventSpec{ShellKindMap: map[string]string{"x": "daemon"}}, "unknown shell kind"},
		{"unknown status", spec.EventSpec{StatusMap: map[string]string{"x": "dormant"}}, "unknown canonical status"},
		{"locations need items and path", spec.EventSpec{Locations: &spec.LocationsSpec{Items: "a"}}, "items and path are required"},
		{"diff files need items path and diff", spec.EventSpec{DiffFiles: &spec.DiffFilesSpec{Items: "a"}}, "items, path and diff are required"},
		{"whole needs a kind", spec.EventSpec{DiffFiles: &spec.DiffFilesSpec{
			Items: "a", Path: "p", Diff: "d", Whole: map[string]string{"new": spec.DiffAdded},
		}}, "whole needs kind"},
		{"whole names a known whole-file kind", spec.EventSpec{DiffFiles: &spec.DiffFilesSpec{
			Items: "a", Path: "p", Diff: "d", Kind: "k", Whole: map[string]string{"new": "renamed"},
		}}, "neither"},
		{"complete diff files", spec.EventSpec{DiffFiles: &spec.DiffFilesSpec{
			Items: "a", Path: "p", Diff: "d", Kind: "k", Whole: map[string]string{"new": spec.DiffAdded},
		}}, ""},
		{"hunks need every part", spec.EventSpec{Patch: &spec.PatchSpec{Items: "h"}}, "are required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := valid()
			d.Events[spec.HookToolPost] = tc.event

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

func TestRichMappings_AFailedTurnMapsOnlyOntoFailureKinds(t *testing.T) {
	d := valid()
	d.Events[spec.HookTurnFailed] = spec.EventSpec{StatusMap: map[string]string{"x": "completed"}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "unknown failure kind")
}
