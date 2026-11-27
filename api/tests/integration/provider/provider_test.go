//go:build integration

package provider_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(m *testing.M) {
	kit.Main(m)
}

type ProviderSuite struct {
	suite.Suite
	env      *kit.Env
	imported kit.ImportedRepo
	wsID     string
	chatID   string
}

func (s *ProviderSuite) SetupTest() {
	t := s.T()
	s.env = kit.BuildEnv(t)
	s.imported = s.env.ImportRepo(t, "provider", "")
	s.wsID, s.chatID = s.env.CreateWorkspaceWithChat(
		t, s.imported.ProjectID, s.imported.RepoID, "feature/provider-test", "",
	)
}

func (s *ProviderSuite) repoBase() string {
	return "/v0/projects/" + s.imported.ProjectID + "/repos/" + s.imported.RepoID
}

func (s *ProviderSuite) chatBase() string {
	return "/v0/chats/" + s.chatID
}

func TestProviderSuite(t *testing.T) {
	suite.Run(t, new(ProviderSuite))
}

func (s *ProviderSuite) TestProvider_pollReturnsCapabilityDisabledForLocalRepo() {
	resp := s.env.GET(s.T(), s.chatBase()+"/provider")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var state map[string]any
	kit.DecodeEnvData(s.T(), resp, &state)

	protected, _ := state["protected"].(bool)
	s.Assert().False(protected, "local repo must not be marked protected")
	s.Assert().Nil(state["pr"], "local repo must not have a PR")
}

func (s *ProviderSuite) TestProvider_unknownChatReturns404() {
	resp := s.env.GET(s.T(), "/v0/chats/no-such-chat/provider")
	defer resp.Body.Close()
	kit.RequireStatus(s.T(), resp, http.StatusNotFound)
}

func (s *ProviderSuite) TestProvider_protectedBranchesForLocalRepoReturnsDefaults() {
	resp := s.env.GET(s.T(), s.repoBase()+"/protected-branches")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var branches []any
	kit.DecodeEnvData(s.T(), resp, &branches)
	s.Assert().NotNil(branches)
}

func (s *ProviderSuite) TestProvider_disabledStateReflectsInWorkspace() {
	t := s.T()

	provResp := s.env.GET(t, s.chatBase()+"/provider")
	kit.RequireStatus(t, provResp, http.StatusOK)
	provResp.Body.Close()

	ws, ok := s.env.WorktreeChats(t, s.imported.ProjectID, s.imported.RepoID)[s.wsID]
	s.Require().True(ok, "workspace %s must appear in the repo's chat list", s.wsID)

	status, _ := ws["status"].(string)
	s.Assert().NotEqual("locked", status,
		"workspace must not be locked when provider is disabled")
}

func (s *ProviderSuite) TestRegression_ProviderPoll_PROpenToMerged() {
	t := s.T()

	watcher := s.env.HoldProjects(t)

	s.env.PushProviderState(t, s.wsID, kit.ProviderState{
		HasPR:    true,
		PRStatus: "open",
		PRUrl:    "https://example.test/pr/1",
		PRTitle:  "feat: provider poll",
	})
	openMsg := kit.WaitForWorkspaceState(t, watcher, s.wsID, "pr-open", 5*time.Second)
	s.Assert().Equal("https://example.test/pr/1", openMsg["prUrl"])

	s.env.PushProviderState(t, s.wsID, kit.ProviderState{
		HasPR:    true,
		PRStatus: "merged",
		PRUrl:    "https://example.test/pr/1",
		PRTitle:  "feat: provider poll",
	})
	mergedMsg := kit.WaitForWorkspaceState(t, watcher, s.wsID, "pr-merged", 5*time.Second)
	s.Assert().Equal(s.wsID, mergedMsg["id"])
}
