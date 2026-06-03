package validator_test

import (
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/flow/def"
	"github.com/char2cs/crowbar/api/internal/engine/flow/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalValid returns a three-state flow that passes all rules:
// - unique state names
// - at least one terminal
// - all transition targets exist
// - no non-terminal transitions INTO the terminal state (a and b cycle; end is unreachable but valid)
// - all non-terminal states have at least one transition
func minimalValid() def.FlowDefinition {
	return def.FlowDefinition{
		Name:    "test",
		Version: "1.0",
		States: []def.StateDefinition{
			{
				Name: "a",
				Transitions: []def.TransitionDef{
					{To: "b", On: "next"},
				},
			},
			{
				Name: "b",
				Transitions: []def.TransitionDef{
					{To: "a", On: "back"},
				},
			},
			{
				Name:     "end",
				Terminal: true,
			},
		},
	}
}

func TestValidate_ValidFlow_NoErrors(t *testing.T) {
	errs := validator.Validate(minimalValid())
	assert.Empty(t, errs)
}

func TestValidationError_Error(t *testing.T) {
	e := validator.ValidationError{Rule: "SomeRule", Message: "something went wrong"}
	assert.Equal(t, "[SomeRule] something went wrong", e.Error())
}

// --- ruleUniqueStateNames ---

func TestRule_UniqueStateNames_Pass(t *testing.T) {
	d := minimalValid()
	errs := validator.Validate(d)
	hasUnique := false
	for _, e := range errs {
		if e.Rule == "UniqueStateNames" {
			hasUnique = true
		}
	}
	assert.False(t, hasUnique)
}

func TestRule_UniqueStateNames_Fail(t *testing.T) {
	d := def.FlowDefinition{
		Name: "dup",
		States: []def.StateDefinition{
			{Name: "start", Transitions: []def.TransitionDef{{To: "start", On: "loop"}}},
			{Name: "start", Terminal: true},
		},
	}
	errs := validator.Validate(d)
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Rule == "UniqueStateNames" && strings.Contains(e.Message, `"start"`) {
			found = true
		}
	}
	assert.True(t, found, "expected UniqueStateNames error for duplicate 'start'")
}

// --- ruleAtLeastOneTerminal ---

func TestRule_AtLeastOneTerminal_Pass(t *testing.T) {
	errs := validator.Validate(minimalValid())
	for _, e := range errs {
		assert.NotEqual(t, "AtLeastOneTerminal", e.Rule)
	}
}

func TestRule_AtLeastOneTerminal_Fail(t *testing.T) {
	d := def.FlowDefinition{
		Name: "no-terminal",
		States: []def.StateDefinition{
			{Name: "a", Transitions: []def.TransitionDef{{To: "b", On: "go"}}},
			{Name: "b", Transitions: []def.TransitionDef{{To: "a", On: "back"}}},
		},
	}
	errs := validator.Validate(d)
	found := false
	for _, e := range errs {
		if e.Rule == "AtLeastOneTerminal" {
			found = true
		}
	}
	assert.True(t, found)
}

// --- ruleTransitionTargetsExist ---

func TestRule_TransitionTargetsExist_Pass(t *testing.T) {
	errs := validator.Validate(minimalValid())
	for _, e := range errs {
		assert.NotEqual(t, "TransitionTargetsExist", e.Rule)
	}
}

func TestRule_TransitionTargetsExist_Fail(t *testing.T) {
	d := def.FlowDefinition{
		Name: "bad-target",
		States: []def.StateDefinition{
			{
				Name: "start",
				Transitions: []def.TransitionDef{
					{To: "ghost", On: "vanish"},
				},
			},
			{Name: "done", Terminal: true},
		},
	}
	errs := validator.Validate(d)
	found := false
	for _, e := range errs {
		if e.Rule == "TransitionTargetsExist" && strings.Contains(e.Message, `"ghost"`) {
			found = true
		}
	}
	assert.True(t, found)
}

// --- ruleNonTerminalHasTransitions ---

func TestRule_NonTerminalHasTransitions_Pass(t *testing.T) {
	// Build a flow where non-terminal states have transitions and no direct-to-terminal transitions.
	d := def.FlowDefinition{
		Name: "valid-chain",
		States: []def.StateDefinition{
			{Name: "a", Transitions: []def.TransitionDef{{To: "b", On: "go"}}},
			{Name: "b", Transitions: []def.TransitionDef{{To: "a", On: "back"}}},
			// terminal added just to satisfy AtLeastOneTerminal
			{Name: "end", Terminal: true},
		},
	}
	errs := validator.Validate(d)
	for _, e := range errs {
		assert.NotEqual(t, "NonTerminalHasTransitions", e.Rule)
	}
}

func TestRule_NonTerminalHasTransitions_Fail(t *testing.T) {
	d := def.FlowDefinition{
		Name: "dead-end",
		States: []def.StateDefinition{
			{Name: "start"},                 // non-terminal, no transitions
			{Name: "done", Terminal: true},
		},
	}
	errs := validator.Validate(d)
	found := false
	for _, e := range errs {
		if e.Rule == "NonTerminalHasTransitions" && strings.Contains(e.Message, `"start"`) {
			found = true
		}
	}
	assert.True(t, found)
}

// --- full multi-error run ---

func TestValidate_MultipleRulesFail(t *testing.T) {
	// Flow with no terminal AND a non-terminal with no transitions.
	d := def.FlowDefinition{
		Name: "broken",
		States: []def.StateDefinition{
			{Name: "orphan"}, // no transitions, no terminal
		},
	}
	errs := validator.Validate(d)
	rules := make(map[string]bool)
	for _, e := range errs {
		rules[e.Rule] = true
	}
	assert.True(t, rules["AtLeastOneTerminal"])
	assert.True(t, rules["NonTerminalHasTransitions"])
}

func TestValidate_EmptyFlow(t *testing.T) {
	d := def.FlowDefinition{Name: "empty"}
	errs := validator.Validate(d)
	rules := make(map[string]bool)
	for _, e := range errs {
		rules[e.Rule] = true
	}
	assert.True(t, rules["AtLeastOneTerminal"])
}
