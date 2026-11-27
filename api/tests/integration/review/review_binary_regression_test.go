//go:build integration

package review_test

import (
	"encoding/json"
	"net/http"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func (s *ReviewSuite) TestRegression_ReviewCompositeCarriesNoDiff() {
	worktree := s.env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.wsID)
	kit.CommitFile(s.T(), worktree, "src/app.go", "package main\n\nfunc main() {}\n", "add a file")

	resp := s.env.GET(s.T(), s.base()+"/review")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var raw map[string]json.RawMessage
	kit.DecodeEnvData(s.T(), resp, &raw)

	_, hasDiff := raw["diff"]
	s.Assert().False(hasDiff,
		"the review composite must not carry a line-level diff; the surface reads "+
			"/review/files + /review/outline and fetches patches per file")

	s.Assert().Contains(raw, "mergeStrategy")
	s.Assert().Contains(raw, "threads")
}

func (s *ReviewSuite) TestRegression_OutlineNeverSendsNullHunks() {
	worktree := s.env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.wsID)

	binary := string([]byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0x00, 0x7f, 0x80})
	kit.CommitFile(s.T(), worktree, "assets/blob.bin", binary, "add a binary file")

	resp := s.env.GET(s.T(), s.base()+"/review/outline")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var outline struct {
		Files []struct {
			Path     string `json:"path"`
			IsBinary bool   `json:"isBinary"`
			Hunks    *[]any `json:"hunks"`
		} `json:"files"`
	}
	kit.DecodeEnvData(s.T(), resp, &outline)

	s.Require().NotEmpty(outline.Files, "the committed binary file must appear in the outline")

	sawBinary := false
	for _, f := range outline.Files {
		s.Assert().NotNilf(f.Hunks, "%s: hunks must be [], never null", f.Path)
		if f.IsBinary {
			sawBinary = true
		}
	}
	s.Assert().True(sawBinary,
		"expected the committed .bin to be reported as binary; if git stopped "+
			"classifying it that way this test no longer covers the crash")
}
