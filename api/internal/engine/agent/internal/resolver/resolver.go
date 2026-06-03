package resolver

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

// Resolve maps an intelligence level to the agent binary and args to use.
// It consults cfg to get the model name, then registry to get the binary.
func Resolve(
	intelligence flow.IntelligenceLevel,
	cfg config.IntelligenceConfig,
) (bin string, args []string, err error) {
	modelName := cfg.ModelFor(string(intelligence))
	if modelName == "" {
		return "", nil, fmt.Errorf("resolver: no model configured for intelligence level %q", intelligence)
	}

	entry, ok := registry.Lookup(modelName)
	if !ok {
		return "", nil, fmt.Errorf("resolver: unknown model prefix for %q", modelName)
	}

	// Copy entry.Args so the shared registry slice is never mutated.
	resolved := append([]string(nil), entry.Args...)
	if entry.ModelArg {
		resolved = append(resolved, modelName)
	}

	return entry.Bin, resolved, nil
}
