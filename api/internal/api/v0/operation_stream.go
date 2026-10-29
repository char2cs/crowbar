package v0

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// operationSnapshots is what operationsDef reads a connecting client's snapshot from.
type operationSnapshots interface {
	Snapshot(ctx context.Context) ([]domain.Operation, error)
}

// operationFrame is one operation on the wire; Snapshot marks the rows sent on
// connect, so a client toasts only a failure it meets live.
type operationFrame struct {
	domain.Operation
	Snapshot bool `json:"snapshot,omitempty"`
}

// operationsDef serves GET /v0/operations: every retained operation on
// connect, then each saved change. One global stream, so no scope or filter.
func operationsDef(ops operationSnapshots) ws.StreamDef[operationFrame] {
	return ws.StreamDef[operationFrame]{
		Namespace:     func(f operationFrame) string { return f.ID },
		Serialize:     func(f operationFrame) ([]byte, error) { return json.Marshal(f) },
		FlatNamespace: true,
		Snapshot: func(string) []operationFrame {
			ctx := context.Background()
			snapshot, err := ops.Snapshot(ctx)
			if err != nil {
				// The stream has no error frame; the client keeps what it holds and live frames follow.
				slog.WarnContext(ctx, "v0: operations snapshot", "err", err)
				return nil
			}
			frames := make([]operationFrame, len(snapshot))
			for i, op := range snapshot {
				frames[i] = operationFrame{Operation: op, Snapshot: true}
			}
			return frames
		},
	}
}
