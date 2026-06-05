package diff_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHunkID_Length(t *testing.T) {
	id := diff.HunkID("path/to/file.go", "+added\n-removed\n context")
	assert.Len(t, id, 12)
}

func TestHunkID_Deterministic(t *testing.T) {
	body := "+added line\n-removed line\n context line"
	id1 := diff.HunkID("some/file.go", body)
	id2 := diff.HunkID("some/file.go", body)
	assert.Equal(t, id1, id2)
}

func TestHunkID_DifferentPaths(t *testing.T) {
	body := "+added line\n-removed line"
	id1 := diff.HunkID("path/a.go", body)
	id2 := diff.HunkID("path/b.go", body)
	assert.NotEqual(t, id1, id2)
}

func TestHunkID_DifferentBodies(t *testing.T) {
	id1 := diff.HunkID("file.go", "+line one")
	id2 := diff.HunkID("file.go", "+line two")
	assert.NotEqual(t, id1, id2)
}

func TestHunkID_StableAcrossSiblingHunks(t *testing.T) {
	body1 := "+hunk one added\n-hunk one removed\n context"
	body2 := "+hunk two added\n-hunk two removed\n context"

	id1Before := diff.HunkID("file.go", body1)
	id2 := diff.HunkID("file.go", body2)
	id1After := diff.HunkID("file.go", body1)

	assert.Equal(t, id1Before, id1After, "hunk1 id should not change when hunk2 is computed")
	assert.NotEqual(t, id1Before, id2)
}

func TestHunkID_HexOnly(t *testing.T) {
	id := diff.HunkID("file.go", "+added")
	for _, c := range id {
		require.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"expected hex char, got %c", c)
	}
}
