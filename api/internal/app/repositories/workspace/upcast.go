package workspace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// schemaVersion is domain.Workspace's current event schema; bump it, and
// upcast the old one in RetiredFields, whenever a field history patches goes.
const schemaVersion = 3

// RetiredFields stamps new events with the current schema and upcasts every
// older one, so history that patches a removed field still replays.
func RetiredFields(b *asynx.Builder[domain.Workspace]) {
	b.WithSchemaVersion(schemaVersion).
		WithUpcaster(1, stripRetiredFields).
		WithUpcaster(2, stripRetiredFields)
}

// stripRetiredFields drops patches against fields domain.Workspace no longer
// declares (/order, /folderId in v1; /lastError in v2): jsonpatch fails on them.
func stripRetiredFields(
	_ context.Context,
	_ string,
	patches []byte,
) ([]byte, error) {
	var ops []map[string]any
	if err := json.Unmarshal(patches, &ops); err != nil {
		return nil, fmt.Errorf("workspace: upcast: decode patches: %w", err)
	}
	kept := make([]map[string]any, 0, len(ops))
	for _, op := range ops {
		switch op["path"] {
		case "/order", "/folderId", "/lastError":
			continue
		}
		kept = append(kept, op)
	}
	out, err := json.Marshal(kept)
	if err != nil {
		return nil, fmt.Errorf("workspace: upcast: encode patches: %w", err)
	}
	return out, nil
}
