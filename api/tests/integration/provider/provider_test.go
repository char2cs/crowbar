//go:build integration

package provider_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(m *testing.M) {
	kit.Main(m)
}

// ProviderSuite is the integration test suite for the provider engine HTTP endpoints.
type ProviderSuite struct {
	suite.Suite
	env      *kit.Env
	repoPath string
	wsID     string
	repoID   string
}

// SetupTest spins up a fresh Env, initialises a git repo, and creates the workspace aggregate used by every test in this suite.
func (s *ProviderSuite) SetupTest() {
	t := s.T()
	s.env = kit.BuildEnv(t)
	s.repoPath = kit.InitRepo(t)
	kit.GitRun(t, s.repoPath, "branch", "-m", "main", "feature/provider-test")
	s.repoID = "r1"

	repoResp := s.env.POST(t, "/v0/repos", map[string]any{
		"id":        s.repoID,
		"projectId": "p1",
		"name":      "repo",
		"path":      s.repoPath,
	})
	kit.RequireStatus(t, repoResp, http.StatusCreated)
	repoResp.Body.Close()

	wsResp := s.env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": s.repoID,
		"branch": kit.BranchName(t, s.repoPath),
	})
	kit.RequireStatus(t, wsResp, http.StatusCreated)
	s.wsID = kit.MutationID(t, wsResp)
}

// TestProviderSuite is the testify suite entry point for ProviderSuite.
func TestProviderSuite(t *testing.T) {
	suite.Run(t, new(ProviderSuite))
}

// TestProvider_pollReturnsCapabilityDisabledForLocalRepo verifies that a local
// repo (no GitHub/GitLab remote) returns a disabled provider state gracefully.
func (s *ProviderSuite) TestProvider_pollReturnsCapabilityDisabledForLocalRepo() {
	resp := s.env.GET(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/provider",
	)
	kit.RequireStatus(
		s.T(),
		resp,
		http.StatusOK,
	)

	var state map[string]any
	kit.DecodeJSON(
		s.T(),
		resp,
		&state,
	)

	protected, _ := state["protected"].(bool)
	s.Assert().False(
		protected,
		"local repo must not be marked protected",
	)
	s.Assert().Nil(
		state["pr"],
		"local repo must not have a PR",
	)
}

// TestProvider_unknownWorkspaceReturns404 verifies 404 on missing workspace.
func (s *ProviderSuite) TestProvider_unknownWorkspaceReturns404() {
	resp := s.env.GET(
		s.T(),
		"/v0/workspaces/no-such-ws/provider",
	)
	defer resp.Body.Close()
	kit.RequireStatus(
		s.T(),
		resp,
		http.StatusNotFound,
	)
}

// TestProvider_protectedBranchesForLocalRepoReturnsDefaults verifies the
// protected-branches endpoint degrades gracefully without a hosted remote.
func (s *ProviderSuite) TestProvider_protectedBranchesForLocalRepoReturnsDefaults() {
	resp := s.env.GET(
		s.T(),
		"/v0/repos/"+s.wsID+"/protected-branches",
	)
	kit.RequireStatus(
		s.T(),
		resp,
		http.StatusOK,
	)

	var branches []any
	kit.DecodeJSON(
		s.T(),
		resp,
		&branches,
	)
	s.Assert().NotNil(branches)
}

// TestProvider_disabledStateReflectsInWorkspace verifies that when the provider
// engine finds no hosted remote, the workspace is not marked locked.
func (s *ProviderSuite) TestProvider_disabledStateReflectsInWorkspace() {
	t := s.T()

	provResp := s.env.GET(t, "/v0/workspaces/"+s.wsID+"/provider")
	kit.RequireStatus(t, provResp, http.StatusOK)
	provResp.Body.Close()

	wsResp := s.env.GET(t, "/v0/workspaces/"+s.wsID)
	kit.RequireStatus(t, wsResp, http.StatusOK)

	var ws map[string]any
	kit.DecodeEnvData(t, wsResp, &ws)

	locked, _ := ws["locked"].(bool)
	s.Assert().False(
		locked,
		"workspace must not be locked when provider is disabled",
	)
}
