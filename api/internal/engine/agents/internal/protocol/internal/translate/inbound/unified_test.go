package inbound_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func unifiedPatch(t *testing.T, payload string) []models.PatchHunk {
	t.Helper()
	d := descriptor(map[string]map[string]string{spec.HookToolPost: {"tool_id": "id"}})
	event := d.Events[spec.HookToolPost]
	event.Patch = &spec.PatchSpec{Unified: "res.diff"}
	d.Events[spec.HookToolPost] = event

	ev, err := parse(d, spec.HookToolPost, []byte(payload))

	require.NoError(t, err)
	return ev.Tool.Patch
}

func TestParse_AUnifiedDiffStringBecomesHunksWithTheirRealLineNumbers(t *testing.T) {
	got := unifiedPatch(t, `{"id":"t1","res":{"diff":"diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -8,3 +9,3 @@ func x()\n a\n-b\n+c\n@@ -30 +31,2 @@\n-d\n+e\n+f\n\\ No newline at end of file\n"}}`)

	assert.Equal(t, []models.PatchHunk{
		{OldStart: 8, NewStart: 9, Lines: []string{" a", "-b", "+c"}},
		{OldStart: 30, NewStart: 31, Lines: []string{"-d", "+e", "+f"}},
	}, got)
}

func TestParse_AUnifiedDiffWithNoHunkIsNoPatch(t *testing.T) {
	assert.Empty(t, unifiedPatch(t, `{"id":"t1","res":{"diff":"--- a/f\n+++ b/f\n"}}`))
	assert.Empty(t, unifiedPatch(t, `{"id":"t1","res":{}}`))
}
