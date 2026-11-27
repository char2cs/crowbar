//go:build integration

package review_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func (s *ReviewSuite) commitScopeFixture() (first, second string) {
	worktree := s.env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.wsID)
	kit.CommitFile(s.T(), worktree, "src/first.ts", "export const first = 1\n", "first change")
	first = strings.TrimSpace(kit.GitRun(s.T(), worktree, "rev-parse", "HEAD"))
	kit.CommitFile(s.T(), worktree, "src/second.ts", "export const second = 2\n", "second change")
	second = strings.TrimSpace(kit.GitRun(s.T(), worktree, "rev-parse", "HEAD"))
	return first, second
}

func (s *ReviewSuite) outlinePaths(query string) []string {
	s.T().Helper()
	resp := s.env.GET(s.T(), s.base()+"/review/outline"+query)
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	defer func() { _ = resp.Body.Close() }()

	var body struct {
		Data struct {
			Files []struct {
				Path string `json:"path"`
			} `json:"files"`
		} `json:"data"`
	}
	raw, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)
	s.Require().NoError(json.Unmarshal(raw, &body))

	paths := make([]string, 0, len(body.Data.Files))
	for _, f := range body.Data.Files {
		paths = append(paths, f.Path)
	}
	return paths
}

func (s *ReviewSuite) TestRegression_OutlineScopedToACommitExcludesTheRestOfTheBranch() {
	first, second := s.commitScopeFixture()

	branch := s.outlinePaths("")
	s.Require().Contains(branch, "src/first.ts", "the branch diff carries both commits")
	s.Require().Contains(branch, "src/second.ts")

	scoped := s.outlinePaths("?sha=" + second)
	s.Assert().Contains(scoped, "src/second.ts", "the scoped outline must carry its own commit")
	s.Assert().NotContains(scoped, "src/first.ts",
		"a commit-scoped outline must not carry the commit before it — that is the whole point of the range")

	scopedFirst := s.outlinePaths("?sha=" + first)
	s.Assert().Contains(scopedFirst, "src/first.ts")
	s.Assert().NotContains(scopedFirst, "src/second.ts")
}

func (s *ReviewSuite) TestRegression_PatchScopedToACommitCarriesThatCommitsContent() {
	_, second := s.commitScopeFixture()

	resp := s.env.GET(s.T(), s.base()+"/review/patch?sha="+second+"&path=src/second.ts")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)
	patch := string(raw)

	s.Assert().Contains(patch, "diff --git")
	s.Assert().Contains(patch, "+export const second = 2")
}

func (s *ReviewSuite) TestRegression_FilesScopedToACommitExcludesTheRestOfTheBranch() {
	first, _ := s.commitScopeFixture()

	paths := func(query string) []string {
		resp := s.env.GET(s.T(), s.base()+"/review/files"+query)
		kit.RequireStatus(s.T(), resp, http.StatusOK)
		defer func() { _ = resp.Body.Close() }()
		var body struct {
			Data struct {
				Files []struct {
					Path string `json:"path"`
				} `json:"files"`
			} `json:"data"`
		}
		raw, err := io.ReadAll(resp.Body)
		s.Require().NoError(err)
		s.Require().NoError(json.Unmarshal(raw, &body))
		out := make([]string, 0, len(body.Data.Files))
		for _, f := range body.Data.Files {
			out = append(out, f.Path)
		}
		return out
	}

	s.Assert().Contains(paths("?sha="+first), "src/first.ts")
	s.Assert().NotContains(paths("?sha="+first), "src/second.ts")
}

func (s *ReviewSuite) TestRegression_SearchScopedToACommitOnlySeesThatCommit() {
	_, second := s.commitScopeFixture()

	hits := func(query string) []string {
		resp := s.env.GET(s.T(), s.base()+"/review/search"+query)
		kit.RequireStatus(s.T(), resp, http.StatusOK)
		defer func() { _ = resp.Body.Close() }()
		var body struct {
			Data struct {
				Hits []struct {
					Path string `json:"path"`
				} `json:"hits"`
			} `json:"data"`
		}
		raw, err := io.ReadAll(resp.Body)
		s.Require().NoError(err)
		s.Require().NoError(json.Unmarshal(raw, &body))
		out := make([]string, 0, len(body.Data.Hits))
		for _, h := range body.Data.Hits {
			out = append(out, h.Path)
		}
		return out
	}

	s.Assert().Contains(hits("?q=export&sha="+second), "src/second.ts")
	s.Assert().NotContains(hits("?q=export&sha="+second), "src/first.ts")
}

func (s *ReviewSuite) TestRegression_RootCommitIsReadableAgainstTheEmptyTree() {
	worktree := s.env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.wsID)
	root := strings.TrimSpace(kit.GitRun(s.T(), worktree, "rev-list", "--max-parents=0", "HEAD"))
	s.Require().NotEmpty(root, "the fixture repo must have a root commit")
	s.Require().NotContains(root, "\n", "the fixture must have exactly one root commit")

	s.Assert().NotNil(s.outlinePaths("?sha=" + root))

	resp := s.env.GET(s.T(), s.base()+"/review/patch?sha="+root+"&path=whatever.ts")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	_ = resp.Body.Close()
}

func (s *ReviewSuite) TestRegression_NonHexCommitScopeIsRefused() {
	for _, bad := range []string{"--output=/tmp/pwned", "HEAD", "main..other", "$(whoami)"} {
		resp := s.env.GET(s.T(), s.base()+"/review/outline?sha="+bad)
		kit.RequireStatus(s.T(), resp, http.StatusBadRequest)
		_ = resp.Body.Close()
	}
}
