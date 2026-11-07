package rules

import (
	"path"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type catalogStatic struct{}

func (catalogStatic) Name() string { return "catalog_static" }

// Check keeps the declared commands names and the workspace directories inside
// the workspace: the catalogue reads them without the CLI, so nothing else does.
func (catalogStatic) Check(d *spec.Descriptor) error {
	sc := d.Presentation.SlashCatalog
	if sc == nil {
		return nil
	}
	for i, b := range sc.Builtins {
		if b.Name == "" || strings.ContainsAny(b.Name, "/ \t\r\n") {
			return invalid(d.ID, "presentation.slash_catalog.builtins[%d].name must be a bare command name", i)
		}
	}
	for i, w := range sc.Workspace {
		if w.Dir == "" || path.IsAbs(w.Dir) || path.Clean(w.Dir) != w.Dir || strings.HasPrefix(w.Dir, "..") {
			return invalid(d.ID, "presentation.slash_catalog.workspace[%d].dir must be a clean relative path", i)
		}
		if w.Entry != "" && path.Base(w.Entry) != w.Entry {
			return invalid(d.ID, "presentation.slash_catalog.workspace[%d].entry must be a file name", i)
		}
	}
	return nil
}
