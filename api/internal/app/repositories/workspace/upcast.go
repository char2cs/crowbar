package workspace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/domain"
)

const schemaVersion = 3

func RetiredFields(b *asynx.Builder[domain.Workspace]) {
	b.WithSchemaVersion(schemaVersion).
		WithUpcaster(1, stripRetiredFields).
		WithUpcaster(2, stripRetiredFields)
}

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
