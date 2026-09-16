package workspace

import (
	"context"
	"encoding/json"
	"fmt"
)

// SchemaVersion is domain.Workspace's current event schema. Bump this and
// register another WithUpcaster (see container.go's newAsynx[domain.Workspace]
// call) whenever a future change removes or repurposes a field real
// production history still writes patches against — see
// StripRetiredPlacementFields's own doc for why version 1 needed one.
const SchemaVersion = 2

// StripRetiredPlacementFields upcasts a schema-version-1 workspace event to
// version 2 by dropping any patch operation that targets /order or
// /folderId — real production data (2026-09-08 sidebar-placement-unification
// Task 5/8) that predates this migration.
//
// Both fields were real domain.Workspace fields once; a workspace ever
// dragged or reordered before that migration has genuine
// workspace.placement_set events in its history carrying "replace"/"add"
// patches against them. domain.Workspace no longer declares either field —
// sidebar placement is a Node{Kind:workspace} row's job now — so replaying
// one of those events against the current (Order/FolderID-less) struct hits
// jsonpatch's strict RFC 6902 semantics: "replace" requires the target key to
// already exist, and it never will, because the seed JSON is marshaled from a
// struct that no longer has the field to put there at all. Caught live via a
// rehearsal replay against production data: "workspace: get: replace
// operation does not apply: doc is missing key: /order: missing value" —
// silently dropping every chat in the affected workspace from any
// repo-scoped list (repo_scope.go's repoOfWorkspace swallows the error).
//
// Dropping the operation rather than converting it to "add" is deliberate:
// nothing reads either field off domain.Workspace any more, so there is
// nothing worth carrying forward, only a patch that would otherwise still
// need somewhere harmless to land.
func StripRetiredPlacementFields(
	_ context.Context,
	_ string,
	patches []byte,
) ([]byte, error) {
	var ops []map[string]any
	if err := json.Unmarshal(patches, &ops); err != nil {
		return nil, fmt.Errorf("workspace: upcast v1->v2: decode patches: %w", err)
	}
	kept := make([]map[string]any, 0, len(ops))
	for _, op := range ops {
		switch op["path"] {
		case "/order", "/folderId":
			continue
		}
		kept = append(kept, op)
	}
	out, err := json.Marshal(kept)
	if err != nil {
		return nil, fmt.Errorf("workspace: upcast v1->v2: encode patches: %w", err)
	}
	return out, nil
}
