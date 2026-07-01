//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reviewDiffFile struct {
	FilePath    string `json:"file_path"`
	Uncommitted bool   `json:"uncommitted"`
}

type reviewDTO struct {
	Diff struct {
		Files []reviewDiffFile `json:"files"`
	} `json:"diff"`
}

// TestReview_BlendedDiffFlagsUncommitted proves the review diff is blended:
// a committed branch change appears with uncommitted=false, while a working-tree
// edit appears with uncommitted=true.
func TestReview_BlendedDiffFlagsUncommitted(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	// committed change on the branch
	var saved struct {
		ID string `json:"id"`
	}
	h.put(base+"/files/content", map[string]string{"path": "committed.txt", "content": "one\n"}, &saved)
	h.post(base+"/git/stage", map[string]any{"paths": []string{"committed.txt"}}, 200, &saved)
	h.post(base+"/git/commit", map[string]string{"subject": "add committed.txt"}, 200, &saved)

	// uncommitted working-tree edit of a tracked file
	h.put(base+"/files/content", map[string]string{"path": "README.md", "content": "edited\n"}, &saved)

	var review reviewDTO
	h.get(base+"/review", &review)

	byPath := map[string]bool{}
	for _, f := range review.Diff.Files {
		byPath[f.FilePath] = f.Uncommitted
	}
	require.Contains(t, byPath, "committed.txt", "committed change must be in the review diff")
	require.Contains(t, byPath, "README.md", "uncommitted edit must be in the review diff")
	assert.False(t, byPath["committed.txt"], "committed file must be flagged uncommitted=false")
	assert.True(t, byPath["README.md"], "edited file must be flagged uncommitted=true")
}
