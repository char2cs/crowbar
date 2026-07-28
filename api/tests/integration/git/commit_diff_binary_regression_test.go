//go:build integration

package git_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestRegression_CommitDiffBinaryFileSendsLinesArray pins the shape of a binary
// file's entry in a commit diff.
//
// A binary file never enters a hunk, so the parser never appends to its Lines —
// and a nil slice marshals to `null`, not `[]`. Every client declares the field
// as a plain array and dereferences it (`diff.lines.length`), so opening ANY
// commit that touched a binary file took the whole diff pane down with "null is
// not an object (evaluating 'diff.lines.length')". The file need not even be
// the one being viewed: the pane reads the first entry to size itself.
//
// The assertion is on the RAW JSON rather than on a decoded struct, because
// decoding is exactly what hides the bug — Go unmarshals both `null` and `[]`
// into a usable nil slice with `len() == 0`, so a struct-level test passes
// against the broken wire format. Only the bytes on the wire distinguish them.
func (s *GitSuite) TestRegression_CommitDiffBinaryFileSendsLinesArray() {
	// ONE commit carrying both: a file git calls binary (a NUL byte is enough)
	// and an ordinary text file. Both in the same commit deliberately — the
	// response then has a normal entry alongside the binary one, so the test can
	// tell "every `lines` is []" from "no `lines` is ever populated", and the
	// binary entry is actually present in the diff under test.
	kit.WriteRepoFile(s.T(), s.worktree, "assets/blob.bin", "\x00\x01\x02binary\x00payload")
	kit.WriteRepoFile(s.T(), s.worktree, "src/plain.ts", "export const x = 1\n")
	kit.GitRun(s.T(), s.worktree, "add", "assets/blob.bin", "src/plain.ts")
	kit.GitRun(s.T(), s.worktree, "commit", "-m", "add a binary asset next to a text file")

	sha := s.headSHA()

	resp := s.Env.GET(s.T(), s.base()+"/git/commit-diff?sha="+sha)
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)

	var env struct {
		Data struct {
			Files []map[string]json.RawMessage `json:"files"`
		} `json:"data"`
	}
	s.Require().NoError(json.Unmarshal(raw, &env))
	s.Require().NotEmpty(env.Data.Files, "the commit changed a file, so the diff must list one")

	for _, file := range env.Data.Files {
		path := jsonString(s.T(), file["file_path"])

		s.Assert().NotEqualf("null", string(file["lines"]),
			"%s: `lines` must be an array on the wire — a client that trusts the declared type crashes on null", path)
		s.Assert().Truef(strings.HasPrefix(string(file["lines"]), "["),
			"%s: `lines` must be a JSON array, got %s", path, truncate(string(file["lines"])))

		// Same failure mode, same fix: a file with no hunks must not send null.
		if hunks, ok := file["hunks"]; ok {
			s.Assert().NotEqualf("null", string(hunks), "%s: `hunks` must be an array on the wire", path)
		}
	}
}

// headSHA returns the full SHA of the workspace worktree's HEAD.
func (s *GitSuite) headSHA() string {
	s.T().Helper()
	return strings.TrimSpace(kit.GitRun(s.T(), s.worktree, "rev-parse", "HEAD"))
}

func jsonString(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return string(raw)
	}
	return s
}

func truncate(s string) string {
	if len(s) <= 60 {
		return s
	}
	return s[:60] + "…"
}
