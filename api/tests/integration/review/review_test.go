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
}

// SetupTest imports a repo and creates the workspace used by every test.
func (s *ReviewSuite) SetupTest() {
	s.env = kit.BuildEnv(s.T())
	s.imported = s.env.ImportRepo(s.T(), "review", "")
	s.wsID = s.env.CreateWorkspace(s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/review-test")
}

// base returns the workspace-scoped route prefix.
func (s *ReviewSuite) base() string {
	return "/v0/projects/" + s.imported.ProjectID +
		"/repos/" + s.imported.RepoID +
		"/workspaces/" + s.wsID
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

	// Verify persistence via GET workspace.
	wsResp := s.env.GET(s.T(), s.base())
	kit.RequireStatus(s.T(), wsResp, http.StatusOK)
	var ws map[string]any
	kit.DecodeEnvData(s.T(), wsResp, &ws)
	s.Assert().Equal("squash", ws["mergeStrategy"],
		"merge strategy must be persisted in the workspace aggregate")
}

// TestReview_UnknownWorkspaceReturns404 checks 404 on non-existent workspace.
func (s *ReviewSuite) TestReview_UnknownWorkspaceReturns404() {
	resp := s.env.GET(s.T(),
		"/v0/projects/"+s.imported.ProjectID+"/repos/"+s.imported.RepoID+"/workspaces/no-such-ws/review")
	defer resp.Body.Close()
	kit.RequireStatus(s.T(), resp, http.StatusNotFound)
}
