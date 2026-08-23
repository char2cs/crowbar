package selection_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/selection"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func passArg(arg, value string) spec.InjectStep {
	return spec.InjectStep{Verb: "pass_arg", Args: map[string]any{"arg": arg, "value": value}}
}

func declaring() *spec.Descriptor {
	return &spec.Descriptor{
		ID: "probe",
		Model: &spec.ModelSpec{
			Available: []string{"sonnet", "opus"},
			Strategy:  spec.DeliveryRestartTUI,
			Apply:     []spec.InjectStep{passArg("--model", "{model}")},
		},
		Effort: &spec.EffortSpec{
			Available: map[string][]string{spec.EffortFallbackKey: {"low", "high"}},
			Strategy:  spec.DeliveryRestartTUI,
			Apply:     []spec.InjectStep{passArg("--effort", "{effort}")},
		},
	}
}

func TestModels_ListsTheDeclaredCatalogue(t *testing.T) {
	assert.Equal(t, []string{"sonnet", "opus"}, selection.Models(declaring()))
}

func TestModels_AbsentBlockIsAbsentCapability(t *testing.T) {
	assert.Nil(t, selection.Models(&spec.Descriptor{ID: "codex"}))
	assert.Nil(t, selection.Models(nil))
}

func TestModels_CopiesTheDescriptorsSlice(t *testing.T) {
	d := declaring()
	got := selection.Models(d)
	got[0] = "mutated"
	assert.Equal(t, []string{"sonnet", "opus"}, d.Model.Available)
}

func TestEfforts_PerModelListWinsOverTheFallback(t *testing.T) {
	d := declaring()
	d.Effort.Available["opus"] = []string{"max"}

	assert.Equal(t, []string{"max"}, selection.Efforts(d, "opus"))
	assert.Equal(t, []string{"low", "high"}, selection.Efforts(d, "sonnet"))
}

func TestEfforts_TheDefaultModelTakesTheFallback(t *testing.T) {
	assert.Equal(t, []string{"low", "high"}, selection.Efforts(declaring(), ""))
}

func TestEfforts_AbsentBlockIsAbsentCapability(t *testing.T) {
	assert.Nil(t, selection.Efforts(&spec.Descriptor{ID: "codex"}, "sonnet"))
	assert.Nil(t, selection.Efforts(nil, "sonnet"))
}

func TestEfforts_NoFallbackAndNoMatchIsEmpty(t *testing.T) {
	d := declaring()
	d.Effort.Available = map[string][]string{"opus": {"max"}}

	assert.Empty(t, selection.Efforts(d, "sonnet"))
}

func TestSteps_RendersOnlyTheChosenHalves(t *testing.T) {
	testCases := []struct {
		name string
		sel  models.Selection
		want []spec.InjectStep
	}{
		{"nothing chosen", models.Selection{}, nil},
		{
			"model only",
			models.Selection{Model: "opus"},
			[]spec.InjectStep{passArg("--model", "{model}")},
		},
		{
			"effort only",
			models.Selection{Effort: "high"},
			[]spec.InjectStep{passArg("--effort", "{effort}")},
		},
		{
			"both",
			models.Selection{Model: "opus", Effort: "high"},
			[]spec.InjectStep{passArg("--model", "{model}"), passArg("--effort", "{effort}")},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, selection.Steps(declaring(), tc.sel))
		})
	}
}

func TestSteps_UndeclaredBlockRendersNothing(t *testing.T) {
	both := models.Selection{Model: "opus", Effort: "high"}

	assert.Nil(t, selection.Steps(&spec.Descriptor{ID: "codex"}, both))
	assert.Nil(t, selection.Steps(nil, both))
}

func TestSteps_CopiesTheDescriptorsSteps(t *testing.T) {
	d := declaring()
	got := selection.Steps(d, models.Selection{Model: "opus"})
	require.Len(t, got, 1)
	got[0].Args["value"] = "mutated"

	assert.Equal(t, "{model}", d.Model.Apply[0].Args["value"])
}

func TestRestartRequired_AnswersEveryTransition(t *testing.T) {
	testCases := []struct {
		name     string
		launched models.Selection
		desired  models.Selection
		want     bool
	}{
		{"unchanged", models.Selection{Model: "opus"}, models.Selection{Model: "opus"}, false},
		{"nothing either side", models.Selection{}, models.Selection{}, false},
		{"model changed", models.Selection{Model: "opus"}, models.Selection{Model: "sonnet"}, true},
		{"effort changed", models.Selection{Effort: "low"}, models.Selection{Effort: "high"}, true},

		{"first choice", models.Selection{}, models.Selection{Model: "opus"}, true},

		{"cleared to default", models.Selection{Model: "opus"}, models.Selection{}, true},
		{"cleared effort", models.Selection{Effort: "high"}, models.Selection{}, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, selection.RestartRequired(declaring(), tc.launched, tc.desired))
		})
	}
}

func TestRestartRequired_UndeclaredBlockNeverRestarts(t *testing.T) {
	changed := models.Selection{Model: "opus", Effort: "high"}

	assert.False(t, selection.RestartRequired(&spec.Descriptor{ID: "codex"}, models.Selection{}, changed))
	assert.False(t, selection.RestartRequired(nil, models.Selection{}, changed))
}

func TestRestartRequired_ReadsTheBlocksOwnStrategy(t *testing.T) {
	d := declaring()
	d.Model.Strategy = "live_switch"
	d.Effort.Strategy = "live_switch"

	assert.False(t, selection.RestartRequired(d,
		models.Selection{}, models.Selection{Model: "opus", Effort: "high"}))
}

func TestRestartRequired_EachBlockAnswersForItsOwnField(t *testing.T) {
	d := declaring()
	d.Model.Strategy = "live_switch"

	assert.False(t, selection.RestartRequired(d,
		models.Selection{Model: "sonnet"}, models.Selection{Model: "opus"}))
	assert.True(t, selection.RestartRequired(d,
		models.Selection{Effort: "low"}, models.Selection{Effort: "high"}))
}

func TestSelection_EmptyReportsAnUnchosenPair(t *testing.T) {
	assert.True(t, models.Selection{}.Empty())
	assert.False(t, models.Selection{Model: "opus"}.Empty())
	assert.False(t, models.Selection{Effort: "high"}.Empty())
}
