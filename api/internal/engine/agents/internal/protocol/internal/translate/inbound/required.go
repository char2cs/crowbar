package inbound

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// checkRequired makes a required: field resolving to nothing a named error. It reads
// the same fields/decoded pair build() uses, so the check cannot disagree with it.
func checkRequired(
	d *spec.Descriptor, canonical string, channel spec.Channel, fields spec.FieldMap, decoded map[string]any,
) error {
	for _, req := range d.EventRequired(canonical) {
		if mapping.String(decoded, fields[req]) != "" {
			continue
		}
		// An explicit null text on a delta frame has nothing to append: skip it.
		if req == "text" && isDelta(canonical) && mapping.IsNull(decoded, fields[req]) {
			return fmt.Errorf("%w: %s text is null", ErrVariantMismatch, canonical)
		}
		return &RequiredFieldError{Descriptor: d.ID, Event: canonical, Channel: string(channel), Field: req}
	}
	return nil
}

func isDelta(canonical string) bool {
	switch canonical {
	case spec.HookMessageDelta, spec.HookReasoningDelta, spec.HookToolOutputDelta:
		return true
	}
	return false
}
