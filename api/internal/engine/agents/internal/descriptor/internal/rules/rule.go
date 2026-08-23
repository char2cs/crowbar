package rules

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var ErrInvalidDescriptor = errors.New("agents: invalid descriptor")

type Rule interface {
	Name() string

	Check(d *spec.Descriptor) error
}

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
		terminalPrompts{},
		terminalNotices{},
		injectedPrompts{},
	}
}

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

func invalid(id, format string, args ...any) error {
	return fmt.Errorf("%w %q: %s", ErrInvalidDescriptor, id, fmt.Sprintf(format, args...))
}

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
