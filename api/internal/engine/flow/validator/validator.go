package validator

import (
	"fmt"
	"sync"

	"github.com/char2cs/crowbar/api/internal/engine/flow/def"
)

// ValidationError records a failed validation rule.
type ValidationError struct {
	Rule    string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Rule, e.Message)
}

// Validate runs all semantic validation rules concurrently against def.
// All rules run regardless of earlier failures. Returns the full set of errors.
func Validate(d def.FlowDefinition) []ValidationError {
	var (
		mu   sync.Mutex
		errs []ValidationError
		wg   sync.WaitGroup
	)

	collect := func(ruleErrs []ValidationError) {
		if len(ruleErrs) == 0 {
			return
		}
		mu.Lock()
		errs = append(errs, ruleErrs...)
		mu.Unlock()
	}

	rules := []func(def.FlowDefinition) []ValidationError{
		ruleUniqueStateNames,
		ruleAtLeastOneTerminal,
		ruleTransitionTargetsExist,
		ruleNonTerminalHasTransitions,
	}

	wg.Add(len(rules))
	for _, r := range rules {
		r := r
		go func() {
			defer wg.Done()
			collect(r(d))
		}()
	}
	wg.Wait()

	return errs
}

func ruleUniqueStateNames(d def.FlowDefinition) []ValidationError {
	seen := make(map[string]bool, len(d.States))
	var errs []ValidationError
	for _, s := range d.States {
		if seen[s.Name] {
			errs = append(errs, ValidationError{
				Rule:    "UniqueStateNames",
				Message: fmt.Sprintf("duplicate state name %q", s.Name),
			})
		}
		seen[s.Name] = true
	}
	return errs
}

func ruleAtLeastOneTerminal(d def.FlowDefinition) []ValidationError {
	for _, s := range d.States {
		if s.Terminal {
			return nil
		}
	}
	return []ValidationError{{
		Rule:    "AtLeastOneTerminal",
		Message: "flow must have at least one terminal state",
	}}
}

func ruleTransitionTargetsExist(d def.FlowDefinition) []ValidationError {
	stateNames := make(map[string]bool, len(d.States))
	for _, s := range d.States {
		stateNames[s.Name] = true
	}
	var errs []ValidationError
	for _, s := range d.States {
		for _, t := range s.Transitions {
			if !stateNames[t.To] {
				errs = append(errs, ValidationError{
					Rule:    "TransitionTargetsExist",
					Message: fmt.Sprintf("state %q transitions to unknown state %q (on event %q)", s.Name, t.To, t.On),
				})
			}
		}
	}
	return errs
}

func ruleNonTerminalHasTransitions(d def.FlowDefinition) []ValidationError {
	var errs []ValidationError
	for _, s := range d.States {
		if !s.Terminal && len(s.Transitions) == 0 {
			errs = append(errs, ValidationError{
				Rule:    "NonTerminalHasTransitions",
				Message: fmt.Sprintf("non-terminal state %q has no transitions — dead end", s.Name),
			})
		}
	}
	return errs
}
