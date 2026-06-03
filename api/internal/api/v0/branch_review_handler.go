package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/fixtures"
)

// BranchReviewDiff serves a small representative multi-file diff for a workspace.
// The desktop daemon has no real VCS diff source yet; this mock keeps the diff
// surface populated until one exists.
func BranchReviewDiff(c *gin.Context) {
	c.JSON(http.StatusOK, fixtures.MultiFileDiff{
		CommitHash:    "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		CommitMessage: "feat: mock diff served by the Go daemon",
		Files: []fixtures.FileDiff{{
			FilePath:  "src/example.ts",
			IsNew:     true,
			Additions: 3,
			Lines: []fixtures.DiffLine{
				{LineType: "header", Content: "@@ -0,0 +1,3 @@"},
				{LineType: "added", Content: "export function hello() {", NewLineNumber: 1},
				{LineType: "added", Content: "  return 'world'", NewLineNumber: 2},
				{LineType: "added", Content: "}", NewLineNumber: 3},
			},
		}},
		TotalFiles:     1,
		TotalAdditions: 3,
		TotalDeletions: 0,
	})
}

// BranchReviewChats serves the conversations listed in a workspace's review.
func BranchReviewChats(c *gin.Context) {
	c.JSON(http.StatusOK, []fixtures.ReviewConversation{
		{ID: "c1", Title: "Implementation discussion", Age: "3h", IsActive: true},
		{ID: "c2", Title: "Edge cases", Age: "1d", IsActive: false},
	})
}

// BranchReviewThreads serves inline review threads. None by default — the
// frontend seeds and persists threads locally once a reviewer adds them.
func BranchReviewThreads(c *gin.Context) {
	c.JSON(http.StatusOK, []any{})
}

// BranchReviewDescription serves the PR-style description for a workspace.
func BranchReviewDescription(c *gin.Context) {
	c.JSON(http.StatusOK, "Mock branch description served by the Go daemon.")
}
