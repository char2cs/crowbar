package evaluator_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/flow/def"
	"github.com/char2cs/crowbar/api/internal/engine/flow/evaluator"
	"github.com/stretchr/testify/assert"
)

// testFlow returns a simple two-state flow for evaluator tests.
func testFlow() def.FlowDefinition {
	return def.FlowDefinition{
		Name: "eval-test",
		States: []def.StateDefinition{
			{
				Name: "start",
				Transitions: []def.TransitionDef{
					{To: "middle", On: "advance"},
					{To: "start", On: "retry"},
				},
			},
			{
				Name: "middle",
				Transitions: []def.TransitionDef{
					{To: "end", On: "finish"},
				},
			},
			{
				Name:     "end",
				Terminal: true,
			},
		},
	}
}

func TestEvaluate_KnownEventMatchesTransition(t *testing.T) {
	tests := []struct {
		name         string
		currentState string
		event        string
		wantTarget   string
		wantOK       bool
	}{
		{
			name:         "start advance -> middle",
			currentState: "start",
			event:        "advance",
			wantTarget:   "middle",
			wantOK:       true,
		},
		{
			name:         "start retry -> start",
			currentState: "start",
			event:        "retry",
			wantTarget:   "start",
			wantOK:       true,
		},
		{
			name:         "middle finish -> end",
			currentState: "middle",
			event:        "finish",
			wantTarget:   "end",
			wantOK:       true,
		},
	}

	d := testFlow()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := evaluator.Evaluate(d, tc.currentState, tc.event)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantTarget, got)
		})
	}
}

func TestEvaluate_UnknownEvent_ReturnsFalse(t *testing.T) {
	d := testFlow()
	got, ok := evaluator.Evaluate(d, "start", "does_not_exist")
	assert.False(t, ok)
	assert.Equal(t, "", got)
}

func TestEvaluate_UnknownState_ReturnsFalse(t *testing.T) {
	d := testFlow()
	got, ok := evaluator.Evaluate(d, "ghost_state", "advance")
	assert.False(t, ok)
	assert.Equal(t, "", got)
}

func TestEvaluate_TerminalState_NoTransitions_ReturnsFalse(t *testing.T) {
	d := testFlow()
	// "end" is terminal and has no transitions.
	got, ok := evaluator.Evaluate(d, "end", "advance")
	assert.False(t, ok)
	assert.Equal(t, "", got)
}

func TestEvaluate_EmptyFlow_ReturnsFalse(t *testing.T) {
	d := def.FlowDefinition{Name: "empty"}
	got, ok := evaluator.Evaluate(d, "start", "advance")
	assert.False(t, ok)
	assert.Equal(t, "", got)
}
