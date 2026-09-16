//go:build integration

package review_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(m *testing.M) {
	kit.Main(m)
}

// ReviewSuite exercises the branch-review read model and merge-strategy mutation.
// Thread CRUD was promoted out of this surface into the first-class /threads
// endpoint (W9) — see the threads integration package.
type ReviewSuite struct {
	suite.Suite
	env      *kit.Env
	imported kit.ImportedRepo
	wsID     string
	chatID   string
}

// SetupTest imports a repo and creates the workspace used by every test.
func (s *ReviewSuite) SetupTest() {
	s.env = kit.BuildEnv(s.T())
	s.imported = s.env.ImportRepo(s.T(), "review", "")
	s.wsID, s.chatID = s.env.CreateWorkspaceWithChat(
		s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/review-test", "",
	)
}

// base returns the chat-scoped route prefix.
func (s *ReviewSuite) base() string {
	return "/v0/chats/" + s.chatID
}

func TestReviewSuite(t *testing.T) {
	suite.Run(t, new(ReviewSuite))
}

// TestReview_GetReviewReturnsPanel verifies the review read-model endpoint responds.
func (s *ReviewSuite) TestReview_GetReviewReturnsPanel() {
	resp := s.env.GET(s.T(), s.base()+"/review")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var panel map[string]any
	kit.DecodeEnvData(s.T(), resp, &panel)
	s.Assert().NotNil(panel)
	_, hasThreads := panel["threads"]
	s.Assert().True(hasThreads, "panel must expose threads")
	_, hasMergeStrategy := panel["mergeStrategy"]
	s.Assert().True(hasMergeStrategy, "panel must expose mergeStrategy")
}

// TestReview_SetMergeStrategy verifies PATCH /review updates the merge strategy.
func (s *ReviewSuite) TestReview_SetMergeStrategy() {
	resp := s.env.PATCH(s.T(), s.base()+"/review", map[string]any{
		"mergeStrategy": "squash",
	})
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var result map[string]any
	kit.DecodeEnvData(s.T(), resp, &result)
	s.Assert().Equal("squash", result["mergeStrategy"])

	// Verify persistence via GET chat: the git fields a worktree-owning chat
	// carries on its own DTO (spec §5) nest the workspace's mergeStrategy under
	// "worktree" now that there is no bare GET .../workspaces/:wsId any more.
	chatResp := s.env.GET(s.T(),
		"/v0/projects/"+s.imported.ProjectID+"/repos/"+s.imported.RepoID+"/chats/"+s.chatID)
	kit.RequireStatus(s.T(), chatResp, http.StatusOK)
	var chat map[string]any
	kit.DecodeEnvData(s.T(), chatResp, &chat)
	worktree, ok := chat["worktree"].(map[string]any)
	s.Require().True(ok, "chat must carry a worktree object")
	s.Assert().Equal("squash", worktree["mergeStrategy"],
		"merge strategy must be persisted in the workspace aggregate")
}

// TestReview_UnknownChatReturns404 checks 404 on a chat resolveChatWorktree
// cannot resolve a worktree for.
func (s *ReviewSuite) TestReview_UnknownChatReturns404() {
	resp := s.env.GET(s.T(), "/v0/chats/no-such-chat/review")
	defer resp.Body.Close()
	kit.RequireStatus(s.T(), resp, http.StatusNotFound)
}
