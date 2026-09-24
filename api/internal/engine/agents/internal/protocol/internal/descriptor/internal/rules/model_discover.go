package rules

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type modelDiscover struct{}

func (modelDiscover) Name() string { return "model_discover" }

func (modelDiscover) Check(d *spec.Descriptor) error {
	if d.Model == nil || d.Model.Discover == nil {
		return nil
	}
	disc := d.Model.Discover

	if err := checkDiscoverCommand(d, disc); err != nil {
		return err
	}
	if err := checkDiscoverMapping(d, disc); err != nil {
		return err
	}
	return checkDiscoverBounds(d, disc)
}

func checkDiscoverCommand(d *spec.Descriptor, disc *spec.ModelDiscoverSpec) error {
	if len(disc.Command) == 0 || hasEmptyArg(disc.Command) {
		return invalid(d.ID, "model.discover.command must be fixed non-empty argv")
	}
	if strings.ContainsAny(strings.Join(disc.Command, ""), "{}") {
		return invalid(d.ID, "model.discover.command must be fixed argv")
	}
	if flag, found := forbidden(disc.Command, d.Spawn.ForbidFlags); found {
		return invalid(d.ID, "model.discover.command contains forbidden flag %q", flag)
	}
	return nil
}

func checkDiscoverMapping(d *spec.Descriptor, disc *spec.ModelDiscoverSpec) error {
	if disc.Adapter != spec.ModelDiscoverAdapterJSON {
		return invalid(d.ID, "model.discover has unsupported adapter %q", disc.Adapter)
	}
	if disc.ItemsPath == "" {
		return invalid(d.ID, "model.discover.items_path is required")
	}
	if disc.Item.ID == "" || disc.Item.Label == "" {
		return invalid(d.ID, "model.discover.item requires id and label")
	}
	if disc.KeepWhen != nil && disc.KeepWhen.Field == "" {
		return invalid(d.ID, "model.discover.keep_when.field is required")
	}
	if disc.DefaultWhen != nil && disc.DefaultWhen.Field == "" {
		return invalid(d.ID, "model.discover.default_when.field is required")
	}
	return nil
}

func checkDiscoverBounds(d *spec.Descriptor, disc *spec.ModelDiscoverSpec) error {
	bounds := []struct {
		name  string
		value int
		max   int
	}{
		{"timeout_ms", disc.TimeoutMS, spec.MaxModelDiscoverTimeoutMS},
		{"max_stdout_bytes", disc.MaxStdoutBytes, spec.MaxModelDiscoverMaxStdoutBytes},
	}
	for _, b := range bounds {
		if b.value < 0 || b.value > b.max {
			return invalid(d.ID, "model.discover.%s must be between 0 and %d", b.name, b.max)
		}
	}
	return nil
}
