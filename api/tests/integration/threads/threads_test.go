//go:build integration

package threads_test

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

// ThreadsSuite exercises the first-class workspace-scoped review-thread endpoint
// and its Broadcaster[ThreadDTO] (W9): open/reply/resolve each return the scoped
// DTO synchronously (201/200) AND broadcast the same DTO on the threads WS.
type ThreadsSuite struct {
	kit.IntegrationSuite
	imported kit.ImportedRepo
	wsID     string
}

// SetupTest imports a repo and creates the workspace used by every test.
func (s *ThreadsSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	s.imported = s.Env.ImportRepo(s.T(), "threads", "")
	s.wsID = s.Env.CreateWorkspace(s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/threads-test")
}

// base returns the workspace-scoped threads route prefix.
func (s *ThreadsSuite) threadsBase() string {
	return "/v0/projects/" + s.imported.ProjectID +
		"/repos/" + s.imported.RepoID +
		"/workspaces/" + s.wsID + "/threads"
}

func TestThreadsSuite(t *testing.T) {
	suite.Run(t, new(ThreadsSuite))
}

// openThread opens a thread and returns its decoded DTO.
func (s *ThreadsSuite) openThread(filePath string, line int, body string) map[string]any {
	s.T().Helper()
	resp := s.Env.POST(s.T(), s.threadsBase(), map[string]any{
		"filePath": filePath,
		"line":     line,
		"side":     "new",
		"body":     body,
	})
	kit.RequireStatus(s.T(), resp, http.StatusCreated)
	var thread map[string]any
	kit.DecodeEnvData(s.T(), resp, &thread)
	return thread
}

// TestThreads_Open_Broadcasts verifies POST .../threads opens a thread (201 +
// scoped DTO) and broadcasts the same ThreadDTO on the threads WS.
func (s *ThreadsSuite) TestThreads_Open_Broadcasts() {
	t := s.T()

	watcher := s.Env.DialThreads(t, s.imported.ProjectID, s.imported.RepoID, s.wsID)

	thread := s.openThread("src/main.go", 10, "This needs a refactor.")
	threadID, _ := thread["id"].(string)
	s.Require().NotEmpty(threadID)
	s.Assert().Equal("src/main.go", thread["filePath"])
	s.Assert().Equal(s.wsID, thread["workspaceId"])
	s.Assert().Equal(false, thread["resolved"])
	s.Assert().Equal("This needs a refactor.", thread["body"], "opening body is the root comment")
	replies, ok := thread["replies"].([]any)
	s.Require().True(ok)
	s.Assert().Empty(replies, "a freshly opened thread has no replies")

	msg := watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["id"] == threadID
	})
	s.Assert().Equal(s.imported.ProjectID, msg["projectId"])
	s.Assert().Equal(s.imported.RepoID, msg["repoId"])
	s.Assert().Equal(s.wsID, msg["workspaceId"])
}

// TestThreads_Reply_Broadcasts verifies POST .../threads/:id/replies appends a
// reply (200 + scoped DTO) and broadcasts the updated ThreadDTO.
func (s *ThreadsSuite) TestThreads_Reply_Broadcasts() {
	t := s.T()

	thread := s.openThread("foo.go", 5, "Initial comment.")
	threadID := thread["id"].(string)

	watcher := s.Env.DialThreads(t, s.imported.ProjectID, s.imported.RepoID, s.wsID)

	replyResp := s.Env.POST(t, s.threadsBase()+"/"+threadID+"/replies", map[string]any{
		"body": "Reply message.",
	})
	kit.RequireStatus(t, replyResp, http.StatusOK)
	var replied map[string]any
	kit.DecodeEnvData(t, replyResp, &replied)
	replies, ok := replied["replies"].([]any)
	s.Require().True(ok)
	s.Assert().GreaterOrEqual(len(replies), 1, "reply must append to the thread's replies")

	msg := watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		if m["id"] != threadID {
			return false
		}
		r, _ := m["replies"].([]any)
		return len(r) >= 1
	})
	s.Assert().Equal(threadID, msg["id"])
}

// TestThreads_Resolve_Broadcasts verifies PATCH .../threads/:id toggles resolved
// (200 + scoped DTO) and broadcasts the updated ThreadDTO both ways.
func (s *ThreadsSuite) TestThreads_Resolve_Broadcasts() {
	t := s.T()

	thread := s.openThread("bar.go", 1, "Resolve me.")
	threadID := thread["id"].(string)

	watcher := s.Env.DialThreads(t, s.imported.ProjectID, s.imported.RepoID, s.wsID)

	resolveResp := s.Env.PATCH(t, s.threadsBase()+"/"+threadID, map[string]any{
		"isResolved": true,
	})
	kit.RequireStatus(t, resolveResp, http.StatusOK)
	var resolved map[string]any
	kit.DecodeEnvData(t, resolveResp, &resolved)
	s.Assert().Equal(true, resolved["resolved"])

	watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		if m["id"] != threadID {
			return false
		}
		r, _ := m["resolved"].(bool)
		return r
	})

	unresolveResp := s.Env.PATCH(t, s.threadsBase()+"/"+threadID, map[string]any{
		"isResolved": false,
	})
	kit.RequireStatus(t, unresolveResp, http.StatusOK)
	var unresolved map[string]any
	kit.DecodeEnvData(t, unresolveResp, &unresolved)
	s.Assert().Equal(false, unresolved["resolved"])

	watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		if m["id"] != threadID {
			return false
		}
		r, _ := m["resolved"].(bool)
		return !r
	})
}
