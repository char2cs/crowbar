package workspace

import (
	"context"
	"encoding/json"
	"fmt"
)

// SchemaVersion is domain.Workspace's current event schema. Bump it and
// register the upcaster for the old version (asynx.go) whenever a field that
// real history still patches is removed.
const SchemaVersion = 3

// StripRetiredFields upcasts a workspace event from schema 1 or 2 by dropping
// every patch against a field domain.Workspace no longer declares (/order,
// /folderId in v1; /lastError in v2). A "replace" on a key the struct cannot
// marshal fails jsonpatch, and with it every read of the workspace.
func StripRetiredFields(
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
