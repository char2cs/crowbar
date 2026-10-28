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

// operationsDef serves GET /v0/operations: every retained operation on
// connect, then each saved change. One global stream, so no scope or filter.
func operationsDef(ops operationSnapshots) ws.StreamDef[domain.Operation] {
	return ws.StreamDef[domain.Operation]{
		Namespace:     func(op domain.Operation) string { return op.ID },
		Serialize:     func(op domain.Operation) ([]byte, error) { return json.Marshal(op) },
		FlatNamespace: true,
		Snapshot: func(string) []domain.Operation {
			ctx := context.Background()
			snapshot, err := ops.Snapshot(ctx)
			if err != nil {
				// The stream has no error frame; the client keeps what it holds and live frames follow.
				slog.WarnContext(ctx, "v0: operations snapshot", "err", err)
				return nil
			}
			return snapshot
		},
	}
}
