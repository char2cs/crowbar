package flow

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/flow/translator"
	"github.com/char2cs/crowbar/api/internal/engine/flow/validator"
)

// defaultLoader is the package-level Loader implementation.
type defaultLoader struct{}

// NewLoader returns a Loader backed by the filesystem.
// When flowPath is "" or "builtin:feature-development" it returns
// FeatureDevelopmentFlow without touching disk.
func NewLoader() Loader {
	return &defaultLoader{}
}

// Load is the package-level loader function. It returns the FlowDefinition for
// the given path. An empty path or "builtin:feature-development" returns the
// hardcoded builtin flow directly. Any other path reads the file from disk,
// parses, and validates it.
func Load(flowPath string) (FlowDefinition, error) {
	if flowPath == "" || flowPath == "builtin:feature-development" {
		return FeatureDevelopmentFlow, nil
	}

	data, err := os.ReadFile(flowPath)
	if err != nil {
		return FlowDefinition{}, fmt.Errorf("flow loader: read %q: %w", flowPath, err)
	}

	def, err := translator.Parse(data)
	if err != nil {
		return FlowDefinition{}, fmt.Errorf("flow loader: parse %q: %w", flowPath, err)
	}

	validationErrs := validator.Validate(def)
	if len(validationErrs) > 0 {
		msgs := make([]string, 0, len(validationErrs))
		for _, ve := range validationErrs {
			msgs = append(msgs, ve.Error())
		}
		return FlowDefinition{}, errors.New("flow loader: validation failed: " + strings.Join(msgs, "; "))
	}

	return def, nil
}

// Load returns the FlowDefinition for the given path.
func (l *defaultLoader) Load(flowPath string) (FlowDefinition, error) {
	return Load(flowPath)
}
