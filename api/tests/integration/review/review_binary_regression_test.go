//go:build integration

package review_test

import (
	"net/http"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestRegression_ReviewCompositeNeverSendsNullLines pins the fix for a crash
// that took down the entire Branch Review pane.
//
// A FileDiff for a BINARY file carries a nil Lines slice, which marshals to
// JSON `null`. The pane reads `diff.lines.length` unguarded, so ONE binary file
// anywhere in a branch produced "null is not an object (evaluating
// 'diff.lines.length')" and the whole review surface refused to render —
// observed live on a 406-file branch whose only binary was a 8KB .bin.
//
// The normaliser existed the whole time (dto.BranchReviewDTOFrom →
// normalizeFileDiff), with a unit test asserting exactly this property. The
// handler simply never called it and wrote the domain object straight out, so
// BranchReviewDTOFrom was reachable only from its own tests. That is why this
// test is black-box against the HTTP response rather than another unit test of
// the mapper: the mapper was never the thing that was broken.
func (s *ReviewSuite) TestRegression_ReviewCompositeNeverSendsNullLines() {
	worktree := s.env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.wsID)

	// Bytes git will classify as binary: a NUL in the first 8000 forces it.
	binary := string([]byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0x00, 0x7f, 0x80})
	kit.CommitFile(s.T(), worktree, "assets/blob.bin", binary, "add a binary file")

	resp := s.env.GET(s.T(), s.base()+"/review")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var panel struct {
		Diff struct {
			Files []struct {
				FilePath string `json:"file_path"`
				IsBinary bool   `json:"is_binary"`
				// A pointer so a JSON `null` is distinguishable from `[]`.
				// With a plain slice both decode to nil and the bug is
				// invisible to the assertion — which is the whole point.
				Lines *[]any `json:"lines"`
				Hunks *[]any `json:"hunks"`
			} `json:"files"`
		} `json:"diff"`
	}
	kit.DecodeEnvData(s.T(), resp, &panel)

	s.Require().NotEmpty(panel.Diff.Files, "the committed binary file must appear in the review diff")

	sawBinary := false
	for _, f := range panel.Diff.Files {
		s.Assert().NotNilf(f.Lines, "%s: lines must be [], never null", f.FilePath)
		s.Assert().NotNilf(f.Hunks, "%s: hunks must be [], never null", f.FilePath)
		if f.IsBinary {
			sawBinary = true
		}
	}
	s.Assert().True(sawBinary,
		"expected the committed .bin to be reported as binary; if git stopped "+
			"classifying it that way this test no longer covers the crash")
}
