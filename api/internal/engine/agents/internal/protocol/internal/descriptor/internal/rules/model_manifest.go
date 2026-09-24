package rules

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type modelManifest struct{}

func (modelManifest) Name() string { return "model_manifest" }

func (modelManifest) Check(d *spec.Descriptor) error {
	if d.Model == nil || d.Model.Manifest == nil {
		return nil
	}
	man := d.Model.Manifest

	if !strings.HasPrefix(man.URL, "https://") {
		return invalid(d.ID, "model.manifest.url must be an https URL")
	}
	if man.ItemsPath == "" {
		return invalid(d.ID, "model.manifest.items_path is required")
	}
	if man.Item.ID == "" || man.Item.Label == "" {
		return invalid(d.ID, "model.manifest.item requires id and label")
	}
	if man.KeepWhen != nil && man.KeepWhen.Field == "" {
		return invalid(d.ID, "model.manifest.keep_when.field is required")
	}

	bounds := []struct {
		name  string
		value int
		max   int
	}{
		{"timeout_ms", man.TimeoutMS, spec.MaxModelManifestTimeoutMS},
		{"ttl_ms", man.TTLMS, spec.MaxModelManifestTTLMS},
	}
	for _, b := range bounds {
		if b.value < 0 || b.value > b.max {
			return invalid(d.ID, "model.manifest.%s must be between 0 and %d", b.name, b.max)
		}
	}
	return nil
}
