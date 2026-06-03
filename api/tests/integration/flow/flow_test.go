//go:build integration

package flow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/flow"
	"github.com/char2cs/crowbar/api/internal/engine/flow/evaluator"
	"github.com/char2cs/crowbar/api/internal/engine/flow/translator"
	"github.com/char2cs/crowbar/api/internal/engine/flow/validator"
	"github.com/char2cs/crowbar/api/tests/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestFlow(t *testing.T) { suite.Run(t, new(FlowSuite)) }

type FlowSuite struct{ kit.IntegrationSuite }

// TestBuiltinFlowStructure asserts the builtin flow has 7 states and exactly
// one terminal state.
func (s *FlowSuite) TestBuiltinFlowStructure() {
	fd := flow.FeatureDevelopmentFlow
	assert.Equal(s.T(), 7, len(fd.States))

	var terminals int
	for _, st := range fd.States {
		if st.Terminal {
			terminals++
		}
	}
	assert.Equal(s.T(), 1, terminals)
}

// TestMissingNameValidationError asserts that a YAML with no top-level name
// field produces a structural (schema) validation error from the translator.
func (s *FlowSuite) TestMissingNameValidationError() {
	yaml := `version: "1.0"
states:
  - name: done
    terminal: true
`
	_, err := translator.Parse([]byte(yaml))
	// Missing "name" is a structural validation error caught by the JSON schema.
	assert.Error(s.T(), err, "expected error for YAML missing name field")
}

// TestTransitionTargetsExistError asserts the semantic validator catches
// transitions to unknown states.
func (s *FlowSuite) TestTransitionTargetsExistError() {
	yaml := `name: bad-flow
version: "1.0"
states:
  - name: start
    transitions:
      - on: go
        to: nonexistent
  - name: done
    terminal: true
`
	fd, err := translator.Parse([]byte(yaml))
	require.NoError(s.T(), err)

	errs := validator.Validate(fd)
	found := false
	for _, e := range errs {
		if e.Rule == "TransitionTargetsExist" {
			found = true
		}
	}
	assert.True(s.T(), found, "expected TransitionTargetsExist error")
}

// TestNoTerminalStateError asserts the validator catches flows with no
// terminal state.
func (s *FlowSuite) TestNoTerminalStateError() {
	yaml := `name: no-terminal
version: "1.0"
states:
  - name: only
    transitions:
      - on: loop
        to: only
`
	fd, err := translator.Parse([]byte(yaml))
	require.NoError(s.T(), err)

	errs := validator.Validate(fd)
	found := false
	for _, e := range errs {
		if e.Rule == "AtLeastOneTerminal" {
			found = true
		}
	}
	assert.True(s.T(), found, "expected AtLeastOneTerminal error")
}

// TestEvaluateKnownEvent asserts that Evaluate returns the correct next state.
func (s *FlowSuite) TestEvaluateKnownEvent() {
	fd := flow.FeatureDevelopmentFlow
	next, ok := evaluator.Evaluate(fd, "brainstorming", "user_approved")
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "spec", next)
}

// TestEvaluateUnknownEvent asserts that Evaluate returns ("", false) for
// events not declared in the current state.
func (s *FlowSuite) TestEvaluateUnknownEvent() {
	fd := flow.FeatureDevelopmentFlow
	_, ok := evaluator.Evaluate(fd, "brainstorming", "unknown_event")
	assert.False(s.T(), ok)
}

// TestLoadFlowFromDisk asserts that the loader parses and validates a valid
// YAML flow file from disk.
func (s *FlowSuite) TestLoadFlowFromDisk() {
	yaml := `name: disk-flow
version: "1.0"
states:
  - name: start
    transitions:
      - on: done
        to: end
  - name: end
    terminal: true
`
	dir := s.T().TempDir()
	p := filepath.Join(dir, "flow.yaml")
	require.NoError(s.T(), os.WriteFile(p, []byte(yaml), 0o644))

	loader := flow.NewLoader()
	fd, err := loader.Load(p)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "disk-flow", fd.Name)
}
