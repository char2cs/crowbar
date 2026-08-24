package rules

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type catalogItemMapping struct{}

func (catalogItemMapping) Name() string { return "catalog_item_mapping" }

var knownItemPlaceholders = strings.NewReplacer(
	"{name}", "", "{description}", "", "{source}", "", "{id}", "",
)

func (catalogItemMapping) Check(d *spec.Descriptor) error {
	sc := d.Presentation.SlashCatalog
	if sc == nil {
		return nil
	}
	item := sc.Pipeline.Item
	if item.Label == "" || item.InsertText == "" {
		return invalid(d.ID, "catalog item mapping requires label and insert_text")
	}
	fields := []struct{ name, value string }{
		{"label", item.Label},
		{"description", item.Description},
		{"insert_text", item.InsertText},
		{"source", item.Source},
	}
	for _, f := range fields {
		if strings.ContainsAny(knownItemPlaceholders.Replace(f.value), "{}") {
			return invalid(d.ID, "catalog item.%s has an unsupported template", f.name)
		}
	}
	return nil
}
