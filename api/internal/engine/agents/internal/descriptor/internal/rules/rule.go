// Package rules validates a parsed descriptor.
//
// One rule per file, each answering a single question, because the alternative is
// the function this package replaced: a 39-branch validator where a new provider
// capability meant another arm on something already past the complexity ceiling.
//
// A rule is pure. It reads a descriptor and returns an error or nil; it performs
// no I/O and mutates nothing, so the whole set can run in any order.
package rules

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// ErrInvalidDescriptor is the sentinel every rule failure wraps, so a caller can
// tell "this descriptor is unusable" from "the file could not be read" without
// matching on message text.
var ErrInvalidDescriptor = errors.New("agents: invalid descriptor")

// Rule is one question asked of a descriptor.
type Rule interface {
	// Name identifies the rule in test failures and logs.
	Name() string
	// Check returns nil when the descriptor satisfies this rule.
	Check(d *spec.Descriptor) error
}

// All returns every rule, in the order they are applied. Ordering is for message
// quality only — identity first, so a descriptor missing its id is not reported
// as an anonymous spawn failure.
func All() []Rule {
	return []Rule{
		identity{},
		spawnCommand{},
		hookVocabulary{},
		promptSubmit{},
		catalogBounds{},
		catalogCommand{},
		catalogItemMapping{},
		catalogAdapter{},
		telemetry{},
		selectionStrategy{},
		selectionApply{},
		modelCatalog{},
		effortCatalog{},
	}
}

// Apply runs every rule and returns the first failure.
func Apply(d *spec.Descriptor) error {
	if d == nil {
		return fmt.Errorf("%w: nil descriptor", ErrInvalidDescriptor)
	}
	for _, rule := range All() {
		if err := rule.Check(d); err != nil {
			return err
		}
	}
	return nil
}

// invalid builds a rule failure that always names the descriptor it came from.
// An unattributed validation error is nearly useless once descriptors can be
// overridden on disk.
func invalid(id, format string, args ...any) error {
	return fmt.Errorf("%w %q: %s", ErrInvalidDescriptor, id, fmt.Sprintf(format, args...))
}

// requireNamedGroup checks that a descriptor-supplied regex both compiles and
// defines the capture the engine will read from it.
func requireNamedGroup(pattern, group string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex: %w", err)
	}
	for _, name := range re.SubexpNames() {
		if name == group {
			return nil
		}
	}
	return fmt.Errorf("regex must define named group %q", group)
}

func hasEmptyArg(argv []string) bool {
	for _, arg := range argv {
		if arg == "" {
			return true
		}
	}
	return false
}

func countTemplate(argv []string, token string) int {
	total := 0
	for _, arg := range argv {
		total += strings.Count(arg, token)
	}
	return total
}

// forbidden reports the first argv entry that is one of the descriptor's own
// forbidden flags. A capability probe must never be the hole through which a
// headless invocation reaches a provider.
func forbidden(argv, forbid []string) (string, bool) {
	for _, arg := range argv {
		for _, f := range forbid {
			if arg == f {
				return arg, true
			}
		}
	}
	return "", false
}
