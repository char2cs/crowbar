package workspace_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
)

// History written before a field was retired still patches it; replayed
// against a struct without the key, a "replace" would fail the whole read.
func TestStripRetiredFields_DropsLastErrorPatches(t *testing.T) {
	in := `[
		{"op":"replace","path":"/lastError","value":"boom"},
		{"op":"replace","path":"/order","value":3},
		{"op":"add","path":"/folderId","value":"f"},
		{"op":"replace","path":"/branch","value":"feat/a"}
	]`

	out, err := workspace.StripRetiredFields(context.Background(), "ws-1", []byte(in))

	require.NoError(t, err)
	var ops []map[string]any
	require.NoError(t, json.Unmarshal(out, &ops))
	require.Len(t, ops, 1)
	assert.Equal(t, "/branch", ops[0]["path"])
}
