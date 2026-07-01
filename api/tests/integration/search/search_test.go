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
// It searches against the adopted main worktree (= the imported repo path) which
// holds the pre-committed Go source file.
type SearchSuite struct {
	kit.IntegrationSuite
	imported kit.ImportedRepo
}

// SetupTest imports a repo with a pre-committed Go source file before each test.
func (s *SearchSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	repoPath := kit.InitRepoWithFile(
		s.T(),
		"hello.go",
		"package main\n\nfunc Hello() string {\n\treturn \"Hello, World!\"\n}\n",
	)
	s.imported = s.Env.ImportRepo(s.T(), "search", repoPath)
}

// searchBase returns the workspace-scoped search route prefix for the adopted
// main worktree.
func (s *SearchSuite) searchBase() string {
	return "/v0/projects/" + s.imported.ProjectID +
		"/repos/" + s.imported.RepoID +
		"/workspaces/" + s.imported.WorkspaceID + "/search"
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

	resp := s.Env.POST(t, s.searchBase(), map[string]any{
		"query": "Hello",
	})
	kit.RequireStatus(t, resp, http.StatusOK)

	var result map[string]any
	kit.DecodeEnvData(t, resp, &result)
	s.Assert().Greater(countResults(result), 0, "search must find at least one match")
}

// TestSearch_CaseSensitiveToggle verifies case-sensitive search filters correctly.
func (s *SearchSuite) TestSearch_CaseSensitiveToggle() {
	t := s.T()

	resp := s.Env.POST(t, s.searchBase(), map[string]any{
		"query":         "hello",
		"caseSensitive": true,
	})
	kit.RequireStatus(t, resp, http.StatusOK)
	var sensitiveResult map[string]any
	kit.DecodeEnvData(t, resp, &sensitiveResult)

	resp2 := s.Env.POST(t, s.searchBase(), map[string]any{
		"query":         "hello",
		"caseSensitive": false,
	})
	kit.RequireStatus(t, resp2, http.StatusOK)
	var insensitiveResult map[string]any
	kit.DecodeEnvData(t, resp2, &insensitiveResult)

	s.Assert().GreaterOrEqual(countResults(insensitiveResult), countResults(sensitiveResult))
}

// TestSearch_RegexMode verifies regex search works.
func (s *SearchSuite) TestSearch_RegexMode() {
	t := s.T()

	resp := s.Env.POST(t, s.searchBase(), map[string]any{
		"query": `func \w+\(\)`,
		"regex": true,
	})
	kit.RequireStatus(t, resp, http.StatusOK)

	var result map[string]any
	kit.DecodeEnvData(t, resp, &result)
	s.Assert().Greater(countResults(result), 0, "search must find at least one match")
}

// TestSearch_BadRegexReturns400 verifies malformed regex yields a 400.
func (s *SearchSuite) TestSearch_BadRegexReturns400() {
	t := s.T()

	resp := s.Env.POST(t, s.searchBase(), map[string]any{
		"query": `[invalid(`,
		"regex": true,
	})
	defer resp.Body.Close()
	s.Assert().Equal(http.StatusBadRequest, resp.StatusCode)
}

// TestSearch_EmptyQueryReturns400 verifies a missing query field yields 400.
func (s *SearchSuite) TestSearch_EmptyQueryReturns400() {
	t := s.T()

	resp := s.Env.POST(t, s.searchBase(), map[string]any{})
	defer resp.Body.Close()
	s.Assert().Equal(http.StatusBadRequest, resp.StatusCode)
}

// TestSearch_UnknownWorkspaceReturns404 ensures 404 on bad workspace.
func (s *SearchSuite) TestSearch_UnknownWorkspaceReturns404() {
	t := s.T()

	bad := "/v0/projects/" + s.imported.ProjectID +
		"/repos/" + s.imported.RepoID + "/workspaces/no-such-ws/search"
	resp := s.Env.POST(t, bad, map[string]any{
		"query": "anything",
	})
	defer resp.Body.Close()
	s.Assert().Equal(http.StatusNotFound, resp.StatusCode)
}

// TestSearch_GitignoreRespectedByDefault verifies .gitignore exclusions are honoured.
func (s *SearchSuite) TestSearch_GitignoreRespectedByDefault() {
	t := s.T()

	kit.WriteRepoFile(t, s.imported.RepoPath, ".gitignore", "*.log\n")
	kit.WriteRepoFile(t, s.imported.RepoPath, "build.log", "Hello from log\n")

	resp := s.Env.POST(t, s.searchBase(), map[string]any{
		"query": "Hello from log",
	})
	kit.RequireStatus(t, resp, http.StatusOK)

	var result map[string]any
	kit.DecodeEnvData(t, resp, &result)
	s.Assert().Equal(0, countResults(result), "gitignored file must not appear in search")
}

// countResults extracts a match count from the search response structure.
func countResults(
	result map[string]any,
) int {
	results, ok := result["results"].([]any)
	if !ok {
		return 0
	}
	return len(results)
}
