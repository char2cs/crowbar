package rules

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type catalogCommand struct{}

func (catalogCommand) Name() string { return "catalog_command" }

// Check requires the inventory command to be FIXED argv: non-empty, no empty
// entries, no templates at all.
//
// The command is run as a real subprocess with no shell, so a template here would
// be the one place a descriptor could smuggle caller-controlled data into argv.
// Detail commands are allowed exactly one {id}, checked by the adapter rule,
// because that value comes from the provider's own inventory output.
func (catalogCommand) Check(d *spec.Descriptor) error {
	sc := d.Presentation.SlashCatalog
	if sc == nil {
		return nil
	}
	cmd := sc.Pipeline.Command
	if len(cmd) == 0 || hasEmptyArg(cmd) {
		return invalid(d.ID,
			"presentation.slash_catalog.pipeline.command must be fixed non-empty argv")
	}
	if strings.ContainsAny(strings.Join(cmd, ""), "{}") {
		return invalid(d.ID, "presentation.slash_catalog.pipeline.command must be fixed argv")
	}
	if flag, found := forbidden(cmd, d.Spawn.ForbidFlags); found {
		return invalid(d.ID, "catalog command contains forbidden flag %q", flag)
	}
	return nil
}
