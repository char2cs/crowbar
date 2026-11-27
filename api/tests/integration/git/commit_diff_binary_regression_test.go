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

func (s *GitSuite) TestRegression_CommitDiffBinaryFileSendsLinesArray() {
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

		if hunks, ok := file["hunks"]; ok {
			s.Assert().NotEqualf("null", string(hunks), "%s: `hunks` must be an array on the wire", path)
		}
	}
}

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
