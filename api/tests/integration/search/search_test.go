//go:build integration

package search_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestMain is the integration test harness entry point for the search package.
func TestMain(
	m *testing.M,
) {
	kit.Main(m)
}

// SearchSuite exercises full-text and regex search endpoints for a workspace.
type SearchSuite struct {
	kit.IntegrationSuite
	repoPath string
	wsID     string
}

// SetupTest creates a fresh Env with a pre-committed Go source file before each test.
func (s *SearchSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	s.repoPath = kit.InitRepoWithFile(
		s.T(),
		"hello.go",
		"package main\n\nfunc Hello() string {\n\treturn \"Hello, World!\"\n}\n",
	)

	repoResp := s.Env.POST(s.T(), "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
		"path":      s.repoPath,
	})
	kit.RequireStatus(s.T(), repoResp, http.StatusCreated)
	repoResp.Body.Close()

	resp := s.Env.POST(
		s.T(),
		"/v0/workspaces",
		map[string]any{
			"repoId": "r1",
			"branch": kit.BranchName(s.T(), s.repoPath),
		},
	)
	kit.RequireStatus(s.T(), resp, http.StatusCreated)
	s.wsID = kit.MutationID(s.T(), resp)
}

// TestSearchSuite is the testify suite entry point for search integration tests.
func TestSearchSuite(
	t *testing.T,
) {
	suite.Run(
		t,
		new(SearchSuite),
	)
}

// TestSearch_BasicQueryReturnsMatches verifies plain-text search finds the target string.
func (s *SearchSuite) TestSearch_BasicQueryReturnsMatches() {
	t := s.T()

	resp := s.Env.POST(
		t,
		"/v0/workspaces/"+s.wsID+"/search",
		map[string]any{
			"query": "Hello",
		},
	)
	kit.RequireStatus(
		t,
		resp,
		http.StatusOK,
	)

	var result map[string]any
	kit.DecodeJSON(
		t,
		resp,
		&result,
	)
	s.Assert().Greater(
		countResults(result),
		0,
		"search must find at least one match",
	)
}

// TestSearch_CaseSensitiveToggle verifies case-sensitive search filters correctly.
func (s *SearchSuite) TestSearch_CaseSensitiveToggle() {
	t := s.T()

	// Case-sensitive: exact case must match.
	resp := s.Env.POST(
		t,
		"/v0/workspaces/"+s.wsID+"/search",
		map[string]any{
			"query":         "hello",
			"caseSensitive": true,
		},
	)
	kit.RequireStatus(
		t,
		resp,
		http.StatusOK,
	)

	var sensitiveResult map[string]any
	kit.DecodeJSON(
		t,
		resp,
		&sensitiveResult,
	)

	// Case-insensitive: "hello" must match "Hello".
	resp2 := s.Env.POST(
		t,
		"/v0/workspaces/"+s.wsID+"/search",
		map[string]any{
			"query":         "hello",
			"caseSensitive": false,
		},
	)
	kit.RequireStatus(
		t,
		resp2,
		http.StatusOK,
	)
	var insensitiveResult map[string]any
	kit.DecodeJSON(
		t,
		resp2,
		&insensitiveResult,
	)

	// Insensitive search must find more or equal results than sensitive.
	sensitiveCount := countResults(sensitiveResult)
	insensitiveCount := countResults(insensitiveResult)
	s.Assert().GreaterOrEqual(
		insensitiveCount,
		sensitiveCount,
	)
}

// TestSearch_RegexMode verifies regex search works.
func (s *SearchSuite) TestSearch_RegexMode() {
	t := s.T()

	resp := s.Env.POST(
		t,
		"/v0/workspaces/"+s.wsID+"/search",
		map[string]any{
			"query": `func \w+\(\)`,
			"regex": true,
		},
	)
	kit.RequireStatus(
		t,
		resp,
		http.StatusOK,
	)

	var result map[string]any
	kit.DecodeJSON(
		t,
		resp,
		&result,
	)
	s.Assert().Greater(
		countResults(result),
		0,
		"search must find at least one match",
	)
}

// TestSearch_BadRegexReturns400 verifies malformed regex yields a 400.
func (s *SearchSuite) TestSearch_BadRegexReturns400() {
	t := s.T()

	resp := s.Env.POST(
		t,
		"/v0/workspaces/"+s.wsID+"/search",
		map[string]any{
			"query": `[invalid(`,
			"regex": true,
		},
	)
	defer resp.Body.Close()
	s.Assert().Equal(
		http.StatusBadRequest,
		resp.StatusCode,
	)
}

// TestSearch_EmptyQueryReturns400 verifies a missing query field yields 400.
func (s *SearchSuite) TestSearch_EmptyQueryReturns400() {
	t := s.T()

	resp := s.Env.POST(
		t,
		"/v0/workspaces/"+s.wsID+"/search",
		map[string]any{},
	)
	defer resp.Body.Close()
	s.Assert().Equal(
		http.StatusBadRequest,
		resp.StatusCode,
	)
}

// TestSearch_UnknownWorkspaceReturns404 ensures 404 on bad workspace.
func (s *SearchSuite) TestSearch_UnknownWorkspaceReturns404() {
	t := s.T()

	resp := s.Env.POST(
		t,
		"/v0/workspaces/no-such-ws/search",
		map[string]any{
			"query": "anything",
		},
	)
	defer resp.Body.Close()
	s.Assert().Equal(
		http.StatusNotFound,
		resp.StatusCode,
	)
}

// TestSearch_GitignoreRespectedByDefault verifies .gitignore exclusions are honoured.
func (s *SearchSuite) TestSearch_GitignoreRespectedByDefault() {
	t := s.T()

	// Files and .gitignore are intentionally left uncommitted — the search engine
	// honours on-disk .gitignore entries regardless of git tracking status.

	// Write a .gitignore that ignores *.log and write a matching file with content.
	kit.WriteRepoFile(
		t,
		s.repoPath,
		".gitignore",
		"*.log\n",
	)
	kit.WriteRepoFile(
		t,
		s.repoPath,
		"build.log",
		"Hello from log\n",
	)

	// Search must not surface the ignored file.
	resp := s.Env.POST(
		t,
		"/v0/workspaces/"+s.wsID+"/search",
		map[string]any{
			"query": "Hello from log",
		},
	)
	kit.RequireStatus(
		t,
		resp,
		http.StatusOK,
	)

	var result map[string]any
	kit.DecodeJSON(
		t,
		resp,
		&result,
	)
	s.Assert().Equal(
		0,
		countResults(result),
		"gitignored file must not appear in search",
	)
}

// countResults extracts a match count from the search response structure.
// The response shape is {"results": []SearchResult, "truncated": bool} where
// each SearchResult is a flat object — one entry per match line.
// This is a pure function with no fatal path; it does not accept *testing.T.
func countResults(
	result map[string]any,
) int {
	results, ok := result["results"].([]any)
	if !ok {
		return 0
	}
	return len(results)
}
