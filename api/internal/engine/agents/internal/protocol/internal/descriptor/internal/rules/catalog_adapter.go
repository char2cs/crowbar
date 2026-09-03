package rules

import (
	"regexp"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type catalogAdapter struct{}

func (catalogAdapter) Name() string { return "catalog_adapter" }

func (r catalogAdapter) Check(d *spec.Descriptor) error {
	sc := d.Presentation.SlashCatalog
	if sc == nil {
		return nil
	}
	switch sc.Pipeline.Adapter {
	case spec.CatalogAdapterJSONTextSection:
		return r.checkTextSection(d)
	case spec.CatalogAdapterJSONInventoryDetails:
		return r.checkInventoryDetails(d)
	default:
		return invalid(d.ID,
			"presentation.slash_catalog has unsupported adapter %q", sc.Pipeline.Adapter)
	}
}

func (catalogAdapter) checkTextSection(d *spec.Descriptor) error {
	p := d.Presentation.SlashCatalog.Pipeline
	if p.TextPath == "" || p.StartMarker == "" || p.EndMarker == "" || p.ItemPattern == "" {
		return invalid(d.ID, "json_text_section pipeline is incomplete")
	}
	if strings.Contains(p.TextPath, "..") {
		return invalid(d.ID, "catalog text_path is invalid")
	}
	if err := requireNamedGroup(p.ItemPattern, "name"); err != nil {
		return invalid(d.ID, "catalog item_pattern: %s", err)
	}
	return nil
}

func (r catalogAdapter) checkInventoryDetails(d *spec.Descriptor) error {
	p := d.Presentation.SlashCatalog.Pipeline
	if p.RowsPath == "" || p.EnabledField == "" || p.IDField == "" ||
		len(p.DetailCommand) == 0 || hasEmptyArg(p.DetailCommand) ||
		p.DetailPattern == "" || p.DetailItemsGroup == "" || p.DetailSeparator == "" {
		return invalid(d.ID, "json_inventory_text_detail pipeline is incomplete")
	}
	if err := r.checkDetailCommand(d, p); err != nil {
		return err
	}
	return r.checkDetailPatterns(d, p)
}

func (catalogAdapter) checkDetailCommand(d *spec.Descriptor, p spec.CatalogPipelineSpec) error {
	if countTemplate(p.DetailCommand, "{id}") != 1 {
		return invalid(d.ID, "catalog detail_command must place {id} exactly once")
	}
	if strings.ContainsAny(strings.ReplaceAll(strings.Join(p.DetailCommand, ""), "{id}", ""), "{}") {
		return invalid(d.ID, "catalog detail_command has an unsupported template")
	}
	if flag, found := forbidden(p.DetailCommand, d.Spawn.ForbidFlags); found {
		return invalid(d.ID, "catalog detail_command contains forbidden flag %q", flag)
	}
	return nil
}

func (catalogAdapter) checkDetailPatterns(d *spec.Descriptor, p spec.CatalogPipelineSpec) error {
	if err := requireNamedGroup(p.DetailPattern, p.DetailItemsGroup); err != nil {
		return invalid(d.ID, "catalog detail_pattern: %s", err)
	}
	if p.DetailEmptyPattern != "" {
		if _, err := regexp.Compile(p.DetailEmptyPattern); err != nil {
			return invalid(d.ID, "catalog detail_empty_pattern: invalid regex: %s", err)
		}
	}
	if p.SourcePattern == "" {
		return nil
	}
	if err := requireNamedGroup(p.SourcePattern, "source"); err != nil {
		return invalid(d.ID, "catalog source_pattern: %s", err)
	}
	return nil
}
