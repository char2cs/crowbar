//go:build integration

package review_test

import (
	"encoding/json"
	"net/http"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestRegression_ReviewCompositeCarriesNoDiff pins the removal that this whole
// program exists for.
//
// /review used to fold the entire line-level branch diff into its response: on
// a 406-file / 1M-line child review that was 158MB carrying 1,441,452 per-line
// JSON objects, which cost 1,162MB of webview memory and still retained 544MB
// after the review tab was closed. The surface now reads /review/files and
// /review/outline and fetches patches per file, so the composite carries only
// description, merge strategy and threads.
//
// This is a payload-shape assertion on purpose. Nothing in the UI would break
// if the diff came back — it would simply be downloaded, parsed, and retained
// again, silently undoing the phase. A size or memory assertion would be flaky;
// the absence of the key is exact.
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

	// The rest of the composite is still the pane's source and must survive.
	s.Assert().Contains(raw, "mergeStrategy")
	s.Assert().Contains(raw, "threads")
}

// TestRegression_OutlineNeverSendsNullHunks pins the null-slice class of bug
// that once took the whole review pane down.
//
// A FileDiff for a BINARY file carried a nil Lines slice, which marshals to
// JSON null, and the pane read diff.lines.length unguarded — so one binary
// file anywhere in a branch produced "null is not an object" and nothing
// rendered. The composite no longer carries lines at all, but the outline
// carries hunks, and git emits no @@ headers for a binary: the same nil-slice
// shape, one field over. Consumers map it unconditionally.
func (s *ReviewSuite) TestRegression_OutlineNeverSendsNullHunks() {
	worktree := s.env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.wsID)

	// A NUL byte in the first 8000 forces git to classify the file as binary.
	binary := string([]byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0x00, 0x7f, 0x80})
	kit.CommitFile(s.T(), worktree, "assets/blob.bin", binary, "add a binary file")

	resp := s.env.GET(s.T(), s.base()+"/review/outline")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var outline struct {
		Files []struct {
			Path     string `json:"path"`
			IsBinary bool   `json:"isBinary"`
			// A pointer so JSON null is distinguishable from []. With a plain
			// slice both decode to nil and the bug is invisible to the
			// assertion — which is the entire point of the test.
			Hunks *[]any `json:"hunks"`
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
