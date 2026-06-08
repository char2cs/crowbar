//go:build integration

package agentrun_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestMain is the integration test entry point; delegates to kit.Main.
func TestMain(m *testing.M) {
	kit.Main(m)
}

// AgentRunSuite verifies AgentRun lifecycle flows through the full HTTP+WS+SQLite stack.
type AgentRunSuite struct {
	kit.IntegrationSuite
}

// TestAgentRunSuite runs the AgentRunSuite via testify.
func TestAgentRunSuite(t *testing.T) {
	suite.Run(
		t,
		new(AgentRunSuite),
	)
}

// TestAgentRun_CreateStartComplete verifies the full happy-path lifecycle:
// create workspace + chat, start a run, complete it, and confirm WS events at each step.
func (s *AgentRunSuite) TestAgentRun_CreateStartComplete() {
	t := s.T()

	repoResp := s.Env.POST(t, "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
	})
	kit.RequireStatus(t, repoResp, http.StatusCreated)
	repoResp.Body.Close()

	wsResp := s.Env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": "r1",
		"branch": "main",
	})
	kit.RequireStatus(t, wsResp, http.StatusCreated)
	wsID := kit.MutationID(t, wsResp)

	watcher := s.Env.DialChats(t, "?wsId="+wsID)

	chatResp := s.Env.POST(t, "/v0/workspaces/"+wsID+"/chats", map[string]any{
		"title": "AgentRun Happy Path",
	})
	kit.RequireStatus(t, chatResp, http.StatusCreated)
	var chatObj map[string]any
	kit.DecodeJSON(t, chatResp, &chatObj)
	chatID := chatObj["id"].(string)
	s.Require().NotEmpty(chatID)

	_ = kit.WaitForChat(t, watcher, chatID, wsID, "idle", 5*time.Second)

	runResp := s.Env.POST(t, "/v0/workspaces/"+wsID+"/runs", map[string]any{
		"chatId": chatID,
	})
	kit.RequireStatus(t, runResp, http.StatusCreated)

	var runObj map[string]any
	kit.DecodeJSON(t, runResp, &runObj)
	runID, _ := runObj["id"].(string)
	s.Require().NotEmpty(runID)

	startResp := s.Env.POST(t, "/v0/runs/"+runID+"/start", nil)
	kit.RequireStatus(t, startResp, http.StatusOK)
	startResp.Body.Close()

	runningMsg := kit.WaitForChat(t, watcher, chatID, wsID, "agent-running", 5*time.Second)
	s.Assert().Equal(chatID, runningMsg["chatId"])
	s.Assert().Equal(wsID, runningMsg["wsId"])
	s.Assert().Equal("agent-running", runningMsg["status"])

	completeResp := s.Env.POST(t, "/v0/runs/"+runID+"/complete", nil)
	kit.RequireStatus(t, completeResp, http.StatusOK)
	completeResp.Body.Close()

	_ = kit.WaitForChat(t, watcher, chatID, wsID, "idle", 5*time.Second)

	getResp := s.Env.GET(t, "/v0/runs/"+runID)
	kit.RequireStatus(t, getResp, http.StatusOK)

	var getObj map[string]any
	kit.DecodeJSON(t, getResp, &getObj)
	s.Assert().Equal(runID, getObj["id"])
	s.Assert().NotEmpty(getObj["status"])
}

// TestAgentRun_CreateStartFail verifies the failure path:
// create workspace + chat, start a run, fail it, and confirm WS idle broadcast.
func (s *AgentRunSuite) TestAgentRun_CreateStartFail() {
	t := s.T()

	repoResp := s.Env.POST(t, "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
	})
	kit.RequireStatus(t, repoResp, http.StatusCreated)
	repoResp.Body.Close()

	wsResp := s.Env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": "r1",
		"branch": "main",
	})
	kit.RequireStatus(t, wsResp, http.StatusCreated)
	wsID := kit.MutationID(t, wsResp)

	watcher := s.Env.DialChats(t, "?wsId="+wsID)

	chatResp := s.Env.POST(t, "/v0/workspaces/"+wsID+"/chats", map[string]any{
		"title": "AgentRun Failure Path",
	})
	kit.RequireStatus(t, chatResp, http.StatusCreated)
	var chatObj map[string]any
	kit.DecodeJSON(t, chatResp, &chatObj)
	chatID := chatObj["id"].(string)
	s.Require().NotEmpty(chatID)

	_ = kit.WaitForChat(t, watcher, chatID, wsID, "idle", 5*time.Second)

	runResp := s.Env.POST(t, "/v0/workspaces/"+wsID+"/runs", map[string]any{
		"chatId": chatID,
	})
	kit.RequireStatus(t, runResp, http.StatusCreated)

	var runObj map[string]any
	kit.DecodeJSON(t, runResp, &runObj)
	runID, _ := runObj["id"].(string)
	s.Require().NotEmpty(runID)

	startResp := s.Env.POST(t, "/v0/runs/"+runID+"/start", nil)
	kit.RequireStatus(t, startResp, http.StatusOK)
	startResp.Body.Close()

	_ = kit.WaitForChat(t, watcher, chatID, wsID, "agent-running", 5*time.Second)

	failResp := s.Env.POST(t, "/v0/runs/"+runID+"/fail", nil)
	kit.RequireStatus(t, failResp, http.StatusOK)
	failResp.Body.Close()

	_ = kit.WaitForChat(t, watcher, chatID, wsID, "idle", 5*time.Second)

	getResp := s.Env.GET(t, "/v0/runs/"+runID)
	kit.RequireStatus(t, getResp, http.StatusOK)

	var getObj map[string]any
	kit.DecodeJSON(t, getResp, &getObj)
	s.Assert().Equal(runID, getObj["id"])

	s.Assert().Equal("error", getObj["status"])
}

// TestAgentRun_ListRunning verifies that a started run appears in GET /runs/running
// and is removed from the list after it completes.
func (s *AgentRunSuite) TestAgentRun_ListRunning() {
	t := s.T()

	repoResp := s.Env.POST(t, "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
	})
	kit.RequireStatus(t, repoResp, http.StatusCreated)
	repoResp.Body.Close()

	wsResp := s.Env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": "r1",
		"branch": "main",
	})
	kit.RequireStatus(t, wsResp, http.StatusCreated)
	wsID := kit.MutationID(t, wsResp)

	watcher := s.Env.DialChats(t, "?wsId="+wsID)

	chatResp := s.Env.POST(t, "/v0/workspaces/"+wsID+"/chats", map[string]any{
		"title": "AgentRun List Running",
	})
	kit.RequireStatus(t, chatResp, http.StatusCreated)
	var chatObj map[string]any
	kit.DecodeJSON(t, chatResp, &chatObj)
	chatID := chatObj["id"].(string)
	s.Require().NotEmpty(chatID)

	_ = kit.WaitForChat(t, watcher, chatID, wsID, "idle", 5*time.Second)

	runResp := s.Env.POST(t, "/v0/workspaces/"+wsID+"/runs", map[string]any{
		"chatId": chatID,
	})
	kit.RequireStatus(t, runResp, http.StatusCreated)

	var runObj map[string]any
	kit.DecodeJSON(t, runResp, &runObj)
	runID, _ := runObj["id"].(string)
	s.Require().NotEmpty(runID)

	startResp := s.Env.POST(t, "/v0/runs/"+runID+"/start", nil)
	kit.RequireStatus(t, startResp, http.StatusOK)
	startResp.Body.Close()

	_ = kit.WaitForChat(t, watcher, chatID, wsID, "agent-running", 5*time.Second)

	listResp := s.Env.GET(t, "/v0/runs/running")
	kit.RequireStatus(t, listResp, http.StatusOK)

	var runningList []map[string]any
	kit.DecodeJSON(t, listResp, &runningList)

	found := false
	for _, r := range runningList {
		if r["id"] == runID {
			found = true
			break
		}
	}
	s.Assert().True(found, "run %q must appear in /v0/runs/running while started", runID)

	completeResp := s.Env.POST(t, "/v0/runs/"+runID+"/complete", nil)
	kit.RequireStatus(t, completeResp, http.StatusOK)
	completeResp.Body.Close()

	_ = kit.WaitForChat(t, watcher, chatID, wsID, "idle", 5*time.Second)

	listResp2 := s.Env.GET(t, "/v0/runs/running")
	kit.RequireStatus(t, listResp2, http.StatusOK)

	var runningList2 []map[string]any
	kit.DecodeJSON(t, listResp2, &runningList2)

	foundAfter := false
	for _, r := range runningList2 {
		if r["id"] == runID {
			foundAfter = true
			break
		}
	}
	s.Assert().False(foundAfter, "run %q must not appear in /v0/runs/running after completing", runID)
}

// TestAgentRun_GetUnknownReturns404 verifies that GET /runs/:id returns 404 for an unknown run ID.
func (s *AgentRunSuite) TestAgentRun_GetUnknownReturns404() {
	t := s.T()

	resp := s.Env.GET(t, "/v0/runs/does-not-exist")
	defer resp.Body.Close()
	s.Assert().Equal(http.StatusNotFound, resp.StatusCode)
}
