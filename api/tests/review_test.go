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

type reviewFileSummary struct {
	Path        string `json:"path"`
	OldPath     string `json:"old_path"`
	Status      string `json:"status"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	Uncommitted bool   `json:"uncommitted"`
	Staged      bool   `json:"staged"`
}

type reviewFilesDTO struct {
	Files []reviewFileSummary `json:"files"`
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

// TestReview_FilesSummary_FullBranchPicture proves GET /review/files returns the
// full changed-files picture — committed branch changes, uncommitted tracked
// edits, and untracked files — each with +N/-N counts and the right uncommitted
// flag, all WITHOUT any line-level diff content in the payload.
func TestReview_FilesSummary_FullBranchPicture(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	var saved struct {
		ID string `json:"id"`
	}
	// A committed change on the branch.
	h.put(base+"/files/content", map[string]string{"path": "committed.txt", "content": "one\ntwo\n"}, &saved)
	h.post(base+"/git/stage", map[string]any{"paths": []string{"committed.txt"}}, 200, &saved)
	h.post(base+"/git/commit", map[string]string{"subject": "add committed.txt"}, 200, &saved)

	// An uncommitted working-tree edit of a tracked file.
	h.put(base+"/files/content", map[string]string{"path": "README.md", "content": "edited line\n"}, &saved)
	// A brand-new untracked file (no diff against the fork point).
	h.put(base+"/files/content", map[string]string{"path": "scratch.txt", "content": "scratch\n"}, &saved)

	var summary reviewFilesDTO
	h.get(base+"/review/files", &summary)

	byPath := map[string]reviewFileSummary{}
	for _, f := range summary.Files {
		byPath[f.Path] = f
	}

	require.Contains(t, byPath, "committed.txt")
	assert.False(t, byPath["committed.txt"].Uncommitted, "committed-only file has no uncommitted badge")
	assert.Equal(t, "added", byPath["committed.txt"].Status)
	assert.Equal(t, 2, byPath["committed.txt"].Additions)

	require.Contains(t, byPath, "README.md")
	assert.True(t, byPath["README.md"].Uncommitted, "tracked working-tree edit is uncommitted")

	require.Contains(t, byPath, "scratch.txt")
	assert.Equal(t, "untracked", byPath["scratch.txt"].Status)
	assert.True(t, byPath["scratch.txt"].Uncommitted)
}
