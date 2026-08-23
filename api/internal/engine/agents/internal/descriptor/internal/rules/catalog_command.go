package rules

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type catalogCommand struct{}

func (catalogCommand) Name() string { return "catalog_command" }

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
