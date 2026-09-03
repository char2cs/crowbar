package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/catalog/internal/adapters"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/catalog/internal/normalize"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func mapItems(
	candidates []adapters.Candidate,
	mapping spec.CatalogItemMapping,
	maxItems int,
) ([]models.SlashCatalogItem, []string) {
	items := make([]models.SlashCatalogItem, 0, min(len(candidates), maxItems))
	seen := make(map[string]struct{}, len(candidates))
	truncated := false

	for _, c := range candidates {
		item, ok := renderItem(c, mapping)
		if !ok {
			continue
		}
		key := item.Source + "\x00" + item.InsertText
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		if len(items) == maxItems {
			truncated = true
			continue
		}
		hash := sha256.Sum256([]byte(key))
		item.ID = "skill-" + hex.EncodeToString(hash[:8])
		items = append(items, item)
	}
	if truncated {
		return items, []string{"Catalog results were truncated to the safe item limit."}
	}
	return items, nil
}

func renderItem(c adapters.Candidate, mapping spec.CatalogItemMapping) (models.SlashCatalogItem, bool) {
	vars := map[string]string{
		"name":        normalize.Redact(c.Name),
		"description": normalize.Redact(c.Description),
		"source":      normalize.Source(c.Source),
		"id":          normalize.Source(c.ID),
	}
	label := normalize.TruncateRunes(
		normalize.StripControls(expand(mapping.Label, vars)), normalize.MaxLabelRunes,
	)
	description := normalize.Redact(normalize.TruncateBytes(
		normalize.StripControls(expand(mapping.Description, vars)), normalize.MaxDescriptionByte,
	))
	insertText := normalize.TruncateBytes(
		normalize.StripComposerControls(expand(mapping.InsertText, vars)), normalize.MaxInsertTextByte,
	)
	source := normalize.TruncateRunes(
		normalize.Source(expand(mapping.Source, vars)), normalize.MaxSourceRunes,
	)

	if label == "" || insertText == "" {
		return models.SlashCatalogItem{}, false
	}
	return models.SlashCatalogItem{
		Kind:        models.CatalogItemKindSkill,
		Label:       label,
		Description: description,
		InsertText:  insertText,
		Source:      source,
	}, true
}

func expand(template string, vars map[string]string) string {
	return strings.NewReplacer(
		"{name}", vars["name"],
		"{description}", vars["description"],
		"{source}", vars["source"],
		"{id}", vars["id"],
	).Replace(template)
}
