//go:build integration

package review_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// The windowed review routes answer the workspace's whole branch by default.
// `?sha=` narrows them to ONE commit, which is what lets history be read on the
// same surface as the review instead of on a second, Monaco-based one.
//
// The distinction these tests defend is not "does a commit-scoped read return
// something" but "does it return the commit AND NOT the branch". Both diffs are
// non-empty here on purpose: a scoping bug that quietly answered the branch
// would pass any assertion that only checked for content.

// commitScopeFixture lays down two commits, each touching a different file, and
// returns their SHAs oldest-first.
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

	// And the other way round, so the test cannot pass by always answering the
	// LAST commit.
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

// A root commit has no parent, so `<commit>^` does not resolve and the diff has
// to be taken against git's empty tree instead. Without that fallback the ref
// would come out as "..<sha>", which git reads as HEAD..<sha> — a silently
// wrong answer rather than an error.
//
// The assertion is that the request SUCCEEDS, not that it returns files: this
// suite's fixture repo is created with an `--allow-empty` initial commit (see
// kit.InitRepo), so its root introduces no files and an empty list is the
// correct answer here. That the ref is built against the empty tree at all is
// pinned by TestCommitRange_RootCommitDiffsAgainstTheEmptyTree, which can name
// the ref directly. Do not "fix" this by asserting a non-empty list — it would
// only pass by changing the fixture.
func (s *ReviewSuite) TestRegression_RootCommitIsReadableAgainstTheEmptyTree() {
	worktree := s.env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.wsID)
	root := strings.TrimSpace(kit.GitRun(s.T(), worktree, "rev-list", "--max-parents=0", "HEAD"))
	s.Require().NotEmpty(root, "the fixture repo must have a root commit")
	s.Require().NotContains(root, "\n", "the fixture must have exactly one root commit")

	// A 500 here is the failure mode: an unresolvable parent taking the whole
	// read down. outlinePaths requires 200.
	s.Assert().NotNil(s.outlinePaths("?sha=" + root))

	// The patch route resolves the same ref, so it must survive the same commit.
	resp := s.env.GET(s.T(), s.base()+"/review/patch?sha="+root+"&path=whatever.ts")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	_ = resp.Body.Close()
}

// The ref this builds becomes an argument to `git diff`, where a leading `-` is
// a flag. A rejection here is a 400, never a subprocess.
func (s *ReviewSuite) TestRegression_NonHexCommitScopeIsRefused() {
	for _, bad := range []string{"--output=/tmp/pwned", "HEAD", "main..other", "$(whoami)"} {
		resp := s.env.GET(s.T(), s.base()+"/review/outline?sha="+bad)
		kit.RequireStatus(s.T(), resp, http.StatusBadRequest)
		_ = resp.Body.Close()
	}
}
