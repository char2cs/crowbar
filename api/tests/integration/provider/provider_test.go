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

// ProviderSuite is the integration test suite for the chat-scoped provider
// state endpoint, the repo-scoped protected-branches endpoint, and the
// mock-provider PR-status transition seam (spec §11).
type ProviderSuite struct {
	suite.Suite
	env      *kit.Env
	imported kit.ImportedRepo
	wsID     string
	// chatID is the chat that OWNS wsID's worktree — the provider state route
	// moved off /workspaces/:wsId onto this chat-scoped prefix (spec §8 step 5).
	chatID string
}

// SetupTest spins up a fresh Env, imports a repo, and creates the workspace +
// owning chat used by every test in this suite.
func (s *ProviderSuite) SetupTest() {
	t := s.T()
	s.env = kit.BuildEnv(t)
	s.imported = s.env.ImportRepo(t, "provider", "")
	s.wsID, s.chatID = s.env.CreateWorkspaceWithChat(
		t, s.imported.ProjectID, s.imported.RepoID, "feature/provider-test", "",
	)
}

// repoBase returns the repo-scoped route prefix.
func (s *ProviderSuite) repoBase() string {
	return "/v0/projects/" + s.imported.ProjectID + "/repos/" + s.imported.RepoID
}

// chatBase returns the chat-scoped route prefix the provider state route now
// mounts on (spec §8 step 5 deleted the /workspaces/:wsId/provider mount).
func (s *ProviderSuite) chatBase() string {
	return "/v0/chats/" + s.chatID
}

// TestProviderSuite is the testify suite entry point for ProviderSuite.
func TestProviderSuite(t *testing.T) {
	suite.Run(t, new(ProviderSuite))
}

// TestProvider_pollReturnsCapabilityDisabledForLocalRepo verifies that a local
// repo (no GitHub/GitLab remote) returns a disabled provider state gracefully.
func (s *ProviderSuite) TestProvider_pollReturnsCapabilityDisabledForLocalRepo() {
	resp := s.env.GET(s.T(), s.chatBase()+"/provider")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var state map[string]any
	kit.DecodeEnvData(s.T(), resp, &state)

	protected, _ := state["protected"].(bool)
	s.Assert().False(protected, "local repo must not be marked protected")
	s.Assert().Nil(state["pr"], "local repo must not have a PR")
}

// TestProvider_unknownChatReturns404 verifies 404 on a chatId that resolves to
// no worktree. The route is chat-scoped now, so the descendant of the old
// bad-:wsId case is a bad :chatId, rejected by resolveChatWorktree before any
// provider handler runs (mirrors terminal's CreateForUnknownChatReturns404).
func (s *ProviderSuite) TestProvider_unknownChatReturns404() {
	resp := s.env.GET(s.T(), "/v0/chats/no-such-chat/provider")
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

	provResp := s.env.GET(t, s.chatBase()+"/provider")
	kit.RequireStatus(t, provResp, http.StatusOK)
	provResp.Body.Close()

	// There is no more standalone GET .../workspaces/:wsId (spec §8 step 6
	// deleted the whole group); the chat list's worktree field is the read
	// model replacement (kit.WorktreeChats flattens it to the same shape).
	ws, ok := s.env.WorktreeChats(t, s.imported.ProjectID, s.imported.RepoID)[s.wsID]
	s.Require().True(ok, "workspace %s must appear in the repo's chat list", s.wsID)

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

	watcher := s.env.DialChat(t, s.chatID)

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
