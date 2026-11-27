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

type ReviewSuite struct {
	suite.Suite
	env      *kit.Env
	imported kit.ImportedRepo
	wsID     string
	chatID   string
}

func (s *ReviewSuite) SetupTest() {
	s.env = kit.BuildEnv(s.T())
	s.imported = s.env.ImportRepo(s.T(), "review", "")
	s.wsID, s.chatID = s.env.CreateWorkspaceWithChat(
		s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/review-test", "",
	)
}

func (s *ReviewSuite) base() string {
	return "/v0/chats/" + s.chatID
}

func TestReviewSuite(t *testing.T) {
	suite.Run(t, new(ReviewSuite))
}

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

func (s *ReviewSuite) TestReview_SetMergeStrategy() {
	resp := s.env.PATCH(s.T(), s.base()+"/review", map[string]any{
		"mergeStrategy": "squash",
	})
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var result map[string]any
	kit.DecodeEnvData(s.T(), resp, &result)
	s.Assert().Equal("squash", result["mergeStrategy"])

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

func (s *ReviewSuite) TestReview_UnknownChatReturns404() {
	resp := s.env.GET(s.T(), "/v0/chats/no-such-chat/review")
	defer resp.Body.Close()
	kit.RequireStatus(s.T(), resp, http.StatusNotFound)
}
