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

// ReviewSuite exercises the branch-review REST endpoints end-to-end.
type ReviewSuite struct {
	suite.Suite
	env        *kit.Env
	repoPath   string
	baseBranch string
	wsID       string
}

// SetupTest spins up a fresh Env, initialises a git repo, and creates the
// workspace aggregate used by every test in this suite.
func (s *ReviewSuite) SetupTest() {
	s.env = kit.BuildEnv(s.T())
	s.repoPath = kit.InitRepo(s.T())
	s.baseBranch = kit.BranchName(s.T(), s.repoPath)

	repoResp := s.env.POST(
		s.T(),
		"/v0/repos",
		map[string]any{
			"id":        "r1",
			"projectId": "p1",
			"name":      "repo",
			"path":      s.repoPath,
		},
	)
	kit.RequireStatus(s.T(), repoResp, http.StatusCreated)
	repoResp.Body.Close()

	wsResp := s.env.POST(
		s.T(),
		"/v0/workspaces",
		map[string]any{
			"repoId": "r1",
			"branch": s.baseBranch,
		},
	)
	kit.RequireStatus(s.T(), wsResp, http.StatusCreated)
	s.wsID = kit.MutationID(s.T(), wsResp)
}

func TestReviewSuite(t *testing.T) {
	suite.Run(t, new(ReviewSuite))
}

// TestReview_GetReviewReturnsPanel verifies the review read-model endpoint responds.
func (s *ReviewSuite) TestReview_GetReviewReturnsPanel() {
	resp := s.env.GET(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/review",
	)
	kit.RequireStatus(
		s.T(),
		resp,
		http.StatusOK,
	)

	var panel map[string]any
	kit.DecodeJSON(
		s.T(),
		resp,
		&panel,
	)
	s.Assert().NotNil(panel)
	_, hasThreads := panel["threads"]
	s.Assert().True(
		hasThreads,
		"panel must expose threads",
	)
	_, hasMergeStrategy := panel["mergeStrategy"]
	s.Assert().True(
		hasMergeStrategy,
		"panel must expose mergeStrategy",
	)
}

// TestReview_OpenThreadCreatesThread verifies POST /review/threads creates a thread.
func (s *ReviewSuite) TestReview_OpenThreadCreatesThread() {
	resp := s.env.POST(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/review/threads",
		map[string]any{
			"filePath":   "src/main.go",
			"lineNumber": 10,
			"side":       "new",
			"body":       "This needs a refactor.",
		},
	)
	kit.RequireStatus(
		s.T(),
		resp,
		http.StatusCreated,
	)

	var thread map[string]any
	kit.DecodeJSON(
		s.T(),
		resp,
		&thread,
	)
	s.Assert().NotEmpty(
		thread["id"],
	)
	s.Assert().Equal(
		"src/main.go",
		thread["filePath"],
	)
	s.Assert().Equal(
		s.wsID,
		thread["wsId"],
	)
	s.Assert().Equal(
		"open",
		thread["status"],
	)
	messages, ok := thread["messages"].([]any)
	s.Require().True(ok)
	s.Assert().Len(
		messages,
		1,
		"opening body must appear as first message",
	)
}

// TestReview_ReplyAddsMessage verifies POST /review/threads/:id/reply adds a message.
func (s *ReviewSuite) TestReview_ReplyAddsMessage() {
	createResp := s.env.POST(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/review/threads",
		map[string]any{
			"filePath":   "foo.go",
			"lineNumber": 5,
			"side":       "new",
			"body":       "Initial comment.",
		},
	)
	kit.RequireStatus(
		s.T(),
		createResp,
		http.StatusCreated,
	)
	var thread map[string]any
	kit.DecodeJSON(
		s.T(),
		createResp,
		&thread,
	)
	threadID := thread["id"].(string)

	replyResp := s.env.POST(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/review/threads/"+threadID+"/reply",
		map[string]any{
			"body": "Reply message.",
		},
	)
	kit.RequireStatus(
		s.T(),
		replyResp,
		http.StatusOK,
	)

	var replied map[string]any
	kit.DecodeJSON(
		s.T(),
		replyResp,
		&replied,
	)
	messages, ok := replied["messages"].([]any)
	s.Require().True(ok)
	s.Assert().GreaterOrEqual(
		len(messages),
		2,
		"thread must have opening message + reply",
	)
}

// TestReview_SetResolvedMarksThread verifies PATCH can resolve and unresolve threads.
func (s *ReviewSuite) TestReview_SetResolvedMarksThread() {
	createResp := s.env.POST(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/review/threads",
		map[string]any{
			"filePath":   "bar.go",
			"lineNumber": 1,
			"side":       "old",
			"body":       "Resolve me.",
		},
	)
	kit.RequireStatus(
		s.T(),
		createResp,
		http.StatusCreated,
	)
	var thread map[string]any
	kit.DecodeJSON(
		s.T(),
		createResp,
		&thread,
	)
	threadID := thread["id"].(string)

	resolveResp := s.env.PATCH(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/review/threads/"+threadID,
		map[string]any{
			"isResolved": true,
		},
	)
	kit.RequireStatus(
		s.T(),
		resolveResp,
		http.StatusOK,
	)
	var resolved map[string]any
	kit.DecodeJSON(
		s.T(),
		resolveResp,
		&resolved,
	)
	s.Assert().Equal(
		"resolved",
		resolved["status"],
	)

	unresolveResp := s.env.PATCH(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/review/threads/"+threadID,
		map[string]any{
			"isResolved": false,
		},
	)
	kit.RequireStatus(
		s.T(),
		unresolveResp,
		http.StatusOK,
	)
	var unresolved map[string]any
	kit.DecodeJSON(
		s.T(),
		unresolveResp,
		&unresolved,
	)
	s.Assert().Equal(
		"open",
		unresolved["status"],
	)
}

// TestReview_SetMergeStrategy verifies PATCH /review updates the merge strategy.
func (s *ReviewSuite) TestReview_SetMergeStrategy() {
	resp := s.env.PATCH(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/review",
		map[string]any{
			"mergeStrategy": "squash",
		},
	)
	kit.RequireStatus(
		s.T(),
		resp,
		http.StatusOK,
	)

	var result map[string]any
	kit.DecodeJSON(
		s.T(),
		resp,
		&result,
	)
	s.Assert().Equal(
		"squash",
		result["mergeStrategy"],
	)

	// Verify persistence via GET workspace.
	wsResp := s.env.GET(
		s.T(),
		"/v0/workspaces/"+s.wsID,
	)
	kit.RequireStatus(s.T(), wsResp, http.StatusOK)
	var ws map[string]any
	kit.DecodeEnvData(s.T(), wsResp, &ws)
	s.Assert().Equal(
		"squash",
		ws["mergeStrategy"],
		"merge strategy must be persisted in the workspace aggregate",
	)
}

// TestReview_UnknownWorkspaceReturns404 checks 404 on non-existent workspace.
func (s *ReviewSuite) TestReview_UnknownWorkspaceReturns404() {
	resp := s.env.GET(
		s.T(),
		"/v0/workspaces/no-such-ws/review",
	)
	defer resp.Body.Close()
	kit.RequireStatus(
		s.T(),
		resp,
		http.StatusNotFound,
	)
}
