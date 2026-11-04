package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// SchemaVersion is domain.Chat's event schema. Version 1 history patches
// placement (the Tree's now) and turn state (the ledger's now).
const SchemaVersion = 2

var (
	placementPaths = []string{"/parentId", "/order"}
	turnStatePaths = []string{"/working", "/currentTurnStarted", "/lastActivityAt", "/asyncWork", "/ledgerCursor"}
)

// RetiredFields stamps new events with SchemaVersion and drops the patches of
// older ones against fields the chat no longer has: jsonpatch fails replacing
// a field the document does not hold.
func RetiredFields(b *asynx.Builder[domain.Chat]) {
	b.WithSchemaVersion(SchemaVersion).WithUpcaster(1, dropPatches(slices.Concat(placementPaths, turnStatePaths)))
}

// KeepPlacement is RetiredFields for a reader of the placement schema 1
// patched (the placement-tree migration): only the turn state is dropped.
func KeepPlacement[T any](b *asynx.Builder[T]) {
	b.WithSchemaVersion(SchemaVersion).WithUpcaster(1, dropPatches(turnStatePaths))
}

func dropPatches(paths []string) asynxModels.Upcaster {
	return func(_ context.Context, _ string, patches []byte) ([]byte, error) {
		var ops []map[string]any
		if err := json.Unmarshal(patches, &ops); err != nil {
			return nil, fmt.Errorf("agentchat: upcast: decode patches: %w", err)
		}
		kept := slices.DeleteFunc(ops, func(op map[string]any) bool {
			path, _ := op["path"].(string)
			return slices.Contains(paths, path)
		})
		out, err := json.Marshal(kept)
		if err != nil {
			return nil, fmt.Errorf("agentchat: upcast: encode patches: %w", err)
		}
		return out, nil
	}
}
