package v0

import (
	"encoding/json"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// EncodeEntity is the wire payload of an entity frame of kind.
func EncodeEntity(kind stream.EntityKind, v any) (json.RawMessage, error) {
	switch kind {
	case stream.KindOperation:
		op, ok := v.(domain.Operation)
		if !ok {
			return nil, fmt.Errorf("v0: encode %s: got %T", kind, v)
		}
		raw, err := json.Marshal(op)
		if err != nil {
			return nil, fmt.Errorf("v0: encode %s %s: %w", kind, op.ID, err)
		}
		return raw, nil
	default:
		return nil, fmt.Errorf("v0: encode: unknown entity kind %q", kind)
	}
}
