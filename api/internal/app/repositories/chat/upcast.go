package chat

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SchemaVersion is domain.Chat's event schema. Version 1 history patches
// /parentId and /order, which the chat no longer stores (the Tree does).
const SchemaVersion = 2

// StripRetiredPlacementFields stamps new events with SchemaVersion and drops the
// placement patches of older ones: jsonpatch fails replacing a field the
// document no longer has.
func StripRetiredPlacementFields(b *asynx.Builder[domain.Chat]) {
	b.WithSchemaVersion(SchemaVersion).WithUpcaster(1, stripPlacement)
}

func stripPlacement(_ context.Context, _ string, patches []byte) ([]byte, error) {
	var ops []map[string]any
	if err := json.Unmarshal(patches, &ops); err != nil {
		return nil, fmt.Errorf("agentchat: upcast: decode patches: %w", err)
	}
	kept := make([]map[string]any, 0, len(ops))
	for _, op := range ops {
		switch op["path"] {
		case "/parentId", "/order":
			continue
		}
		kept = append(kept, op)
	}
	out, err := json.Marshal(kept)
	if err != nil {
		return nil, fmt.Errorf("agentchat: upcast: encode patches: %w", err)
	}
	return out, nil
}
