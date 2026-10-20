//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// TestReview_FilesSummary_FullBranchPicture proves GET /review/files returns the
// full changed-files picture — committed branch changes, uncommitted tracked
// edits, and untracked files — each with +N/-N counts and the right uncommitted
// flag, all WITHOUT any line-level diff content in the payload.
//
// This also carries what TestReview_BlendedDiffFlagsUncommitted used to: that
// test read the blend off `/review`'s composite `diff.files`, which no longer
// exists (see TestRegression_ReviewCompositeCarriesNoDiff). /review/files is
// where the blend lives now, and it reports strictly more — status and counts
// as well as the flag.
func TestReview_FilesSummary_FullBranchPicture(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	var saved struct {
		ID string `json:"id"`
	}
	save := func(path, content string) {
		h.put(documentsContent, map[string]string{"workspaceId": imported.workspaceID, "path": path, "content": content}, nil)
	}
	// A committed change on the branch.
	save("committed.txt", "one\ntwo\n")
	h.post(base+"/git/stage", map[string]any{"paths": []string{"committed.txt"}}, 200, &saved)
	h.post(base+"/git/commit", map[string]string{"subject": "add committed.txt"}, 200, &saved)

	// An uncommitted working-tree edit of a tracked file.
	save("README.md", "edited line\n")
	// A brand-new untracked file (no diff against the fork point).
	save("scratch.txt", "scratch\n")

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
