package rules

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type identity struct{}

func (identity) Name() string { return "identity" }

func (identity) Check(d *spec.Descriptor) error {
	if d.ID == "" {
		return fmt.Errorf("%w: missing id", ErrInvalidDescriptor)
	}
	return nil
}
