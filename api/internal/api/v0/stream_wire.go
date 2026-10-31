package v0

import (
	"encoding/json"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
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
	case stream.KindReviewThread:
		return encodeReviewThread(v)
	default:
		return nil, fmt.Errorf("v0: encode: unknown entity kind %q", kind)
	}
}

// encodeReviewThread is a thread's ThreadDTO: a REST write hands the DTO it
// answered with, a snapshot or an agent the thread and the ids its scope names.
func encodeReviewThread(v any) (json.RawMessage, error) {
	d, ok := v.(dto.ThreadDTO)
	if th, isThread := v.(appstream.ReviewThread); isThread {
		d, ok = dto.ThreadDTOFrom(th.Thread.NormalizedMessages(), th.ProjectID, th.RepoID), true
	}
	if !ok {
		return nil, fmt.Errorf("v0: encode %s: got %T", stream.KindReviewThread, v)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("v0: encode %s %s: %w", stream.KindReviewThread, d.ID, err)
	}
	return raw, nil
}
