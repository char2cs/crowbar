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

// ProviderSuite is the integration test suite for the workspace-scoped provider
// endpoints and the mock-provider PR-status transition seam (spec §11).
type ProviderSuite struct {
	suite.Suite
	env      *kit.Env
	imported kit.ImportedRepo
	wsID     string
}

// SetupTest spins up a fresh Env, imports a repo, and creates the workspace used
// by every test in this suite.
func (s *ProviderSuite) SetupTest() {
	t := s.T()
	s.env = kit.BuildEnv(t)
	s.imported = s.env.ImportRepo(t, "provider", "")
	s.wsID = s.env.CreateWorkspace(t, s.imported.ProjectID, s.imported.RepoID, "feature/provider-test")
}

// repoBase returns the repo-scoped route prefix.
func (s *ProviderSuite) repoBase() string {
	return "/v0/projects/" + s.imported.ProjectID + "/repos/" + s.imported.RepoID
}

// wsBase returns the workspace-scoped route prefix.
func (s *ProviderSuite) wsBase() string {
	return s.repoBase() + "/workspaces/" + s.wsID
}

// TestProviderSuite is the testify suite entry point for ProviderSuite.
func TestProviderSuite(t *testing.T) {
	suite.Run(t, new(ProviderSuite))
}

// TestProvider_pollReturnsCapabilityDisabledForLocalRepo verifies that a local
// repo (no GitHub/GitLab remote) returns a disabled provider state gracefully.
func (s *ProviderSuite) TestProvider_pollReturnsCapabilityDisabledForLocalRepo() {
	resp := s.env.GET(s.T(), s.wsBase()+"/provider")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var state map[string]any
	kit.DecodeEnvData(s.T(), resp, &state)

	protected, _ := state["protected"].(bool)
	s.Assert().False(protected, "local repo must not be marked protected")
	s.Assert().Nil(state["pr"], "local repo must not have a PR")
}

// TestProvider_unknownWorkspaceReturns404 verifies 404 on missing workspace.
func (s *ProviderSuite) TestProvider_unknownWorkspaceReturns404() {
	resp := s.env.GET(s.T(), s.repoBase()+"/workspaces/no-such-ws/provider")
	defer resp.Body.Close()
	kit.RequireStatus(s.T(), resp, http.StatusNotFound)
}

// TestProvider_protectedBranchesForLocalRepoReturnsDefaults verifies the
// repo-scoped protected-branches endpoint degrades gracefully without a hosted
// remote.
func (s *ProviderSuite) TestProvider_protectedBranchesForLocalRepoReturnsDefaults() {
	resp := s.env.GET(s.T(), s.repoBase()+"/protected-branches")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var branches []any
	kit.DecodeEnvData(s.T(), resp, &branches)
	s.Assert().NotNil(branches)
}

// TestProvider_disabledStateReflectsInWorkspace verifies that when the provider
// engine finds no hosted remote, the workspace is not marked locked (status).
func (s *ProviderSuite) TestProvider_disabledStateReflectsInWorkspace() {
	t := s.T()

	provResp := s.env.GET(t, s.wsBase()+"/provider")
	kit.RequireStatus(t, provResp, http.StatusOK)
	provResp.Body.Close()

	wsResp := s.env.GET(t, s.wsBase())
	kit.RequireStatus(t, wsResp, http.StatusOK)

	var ws map[string]any
	kit.DecodeEnvData(t, wsResp, &ws)

	status, _ := ws["status"].(string)
	s.Assert().NotEqual("locked", status,
		"workspace must not be locked when provider is disabled")
}

// TestRegression_ProviderPoll_PROpenToMerged drives a pr-open → pr-merged
// transition through the mock-provider seam (PushProviderState) and asserts the
// resulting WorkspaceDTO arrives on the workspace WS with status pr-merged. No
// real network and no fixed delay — the seam applies projection-synchronously and
// the projection broadcasts the row (spec §11/§13).
func (s *ProviderSuite) TestRegression_ProviderPoll_PROpenToMerged() {
	t := s.T()

	watcher := s.env.DialWorkspace(t, s.imported.ProjectID, s.imported.RepoID, s.wsID)

	// First poll: the PR is open.
	s.env.PushProviderState(t, s.wsID, kit.ProviderState{
		HasPR:    true,
		PRStatus: "open",
		PRUrl:    "https://example.test/pr/1",
		PRTitle:  "feat: provider poll",
	})
	openMsg := kit.WaitForWorkspaceState(t, watcher, s.wsID, "pr-open", 5*time.Second)
	s.Assert().Equal("https://example.test/pr/1", openMsg["prUrl"])

	// Second poll: the PR has merged.
	s.env.PushProviderState(t, s.wsID, kit.ProviderState{
		HasPR:    true,
		PRStatus: "merged",
		PRUrl:    "https://example.test/pr/1",
		PRTitle:  "feat: provider poll",
	})
	mergedMsg := kit.WaitForWorkspaceState(t, watcher, s.wsID, "pr-merged", 5*time.Second)
	s.Assert().Equal(s.wsID, mergedMsg["id"])
}
