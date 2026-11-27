//go:build integration

package search_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(
	m *testing.M,
) {
	kit.Main(m)
}

type SearchSuite struct {
	kit.IntegrationSuite
	imported kit.ImportedRepo
}

func (s *SearchSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	repoPath := kit.InitRepoWithFile(
		s.T(),
		"hello.go",
		"package main\n\nfunc Hello() string {\n\treturn \"Hello, World!\"\n}\n",
	)
	s.imported = s.Env.ImportRepo(s.T(), "search", repoPath)
}

func (s *SearchSuite) searchBase() string {
	return "/v0/chats/" + s.imported.ChatID + "/search"
}

func TestSearchSuite(
	t *testing.T,
) {
	suite.Run(
		t,
		new(SearchSuite),
	)
}

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

func (s *SearchSuite) TestSearch_BadRegexReturns400() {
	t := s.T()

	resp := s.Env.POST(t, s.searchBase(), map[string]any{
		"query": `[invalid(`,
		"regex": true,
	})
	defer resp.Body.Close()
	s.Assert().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *SearchSuite) TestSearch_EmptyQueryReturns400() {
	t := s.T()

	resp := s.Env.POST(t, s.searchBase(), map[string]any{})
	defer resp.Body.Close()
	s.Assert().Equal(http.StatusBadRequest, resp.StatusCode)
}

func (s *SearchSuite) TestSearch_UnknownChatReturns404() {
	t := s.T()

	resp := s.Env.POST(t, "/v0/chats/no-such-chat/search", map[string]any{
		"query": "anything",
	})
	defer resp.Body.Close()
	s.Assert().Equal(http.StatusNotFound, resp.StatusCode)
}

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

func countResults(
	result map[string]any,
) int {
	results, ok := result["results"].([]any)
	if !ok {
		return 0
	}
	return len(results)
}
