//go:build integration

package crash_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestMain is the integration test entry point; delegates to kit.Main.
func TestMain(m *testing.M) {
	kit.Main(m)
}

// CrashSuite tests crash recovery and orphan-run reconciliation scenarios.
type CrashSuite struct {
	kit.IntegrationSuite
}

// SetupTest is a no-op override that suppresses the inherited env build;
// each test in this suite manages its own env via kit.BuildEnv / kit.BuildEnvAt.
// Do not access s.Env inside any test in this suite — it will be nil.
func (s *CrashSuite) SetupTest() {}

// TestCrashSuite runs the CrashSuite via testify.
func TestCrashSuite(t *testing.T) {
	suite.Run(
		t,
		new(CrashSuite),
	)
}

// TestCrash_AgentRunRecoveryMarksOrphansError verifies that when a new Env is
// built (simulating a server restart), any AgentRun that was left in the
// "running" state is recovered to "error" — the RecoverOrphans path (00 §6.2).
func (s *CrashSuite) TestCrash_AgentRunRecoveryMarksOrphansError() {
	s.Require().Nil(
		s.Env,
		"CrashSuite tests must not use s.Env; call kit.BuildEnv locally",
	)
	t := s.T()

	// homeDir is shared between the two Envs so they open the same SQLite DB.
	homeDir := t.TempDir()

	// Env 1: create a workspace and an AgentRun that gets stuck mid-run.
	env1 := kit.BuildEnvAt(t, homeDir)

	base := strings.ReplaceAll(
		s.T().Name(),
		"/",
		"-",
	)
	repoID := base + "-repo"
	runID := base + "-run"
	var wsID, chatID string

	// Create a git repo on disk so the workspace worktree path is valid.
	repoPath := kit.InitRepo(t)

	// Register the repo via HTTP.
	repoResp := env1.POST(t, "/v0/repos", map[string]any{
		"id":        repoID,
		"projectId": "p1",
		"name":      "crash-repo",
		"path":      repoPath,
	})
	kit.RequireStatus(t, repoResp, http.StatusCreated)
	repoResp.Body.Close()

	// Create a workspace; capture the server-assigned UUID.
	wsResp := env1.POST(t, "/v0/workspaces", map[string]any{
		"repoId": repoID,
		"branch": "main",
	})
	kit.RequireStatus(t, wsResp, http.StatusCreated)
	wsID = kit.MutationID(t, wsResp)

	// Create a chat; capture the server-assigned UUID.
	chatResp := env1.POST(t, "/v0/workspaces/"+wsID+"/chats", map[string]any{
		"title": "Crash Chat",
	})
	kit.RequireStatus(t, chatResp, http.StatusCreated)
	var chatDTO map[string]any
	kit.DecodeEnvData(t, chatResp, &chatDTO)
	chatID = chatDTO["id"].(string)

	// Create a run (agentrun handler accepts explicit id).
	runResp := env1.POST(t, "/v0/workspaces/"+wsID+"/runs", map[string]any{
		"id":     runID,
		"chatId": chatID,
	})
	kit.RequireStatus(t, runResp, http.StatusCreated)

	// Start the run (transitions it to "running").
	startResp := env1.POST(t, "/v0/runs/"+runID+"/start", nil)
	kit.RequireStatus(t, startResp, http.StatusOK)

	// Verify the run is in the running list before "crash".
	runningResp := env1.GET(t, "/v0/runs/running")
	kit.RequireStatus(t, runningResp, http.StatusOK)
	var runningList []map[string]any
	kit.DecodeEnvData(t, runningResp, &runningList)
	found := false
	for _, r := range runningList {
		if r["id"] == runID {
			found = true
			break
		}
	}
	s.Require().True(
		found,
		"run must be in the running list before crash",
	)

	// Simulate crash: close env1's adapter eagerly so its SQLite file handles are
	// released before env2 opens the same database files. Without this, even
	// WAL-mode SQLite can return SQLITE_BUSY during DDL (AutoMigrate) because
	// gorm's pool keeps the connection open until the sql.DB is closed.
	env1.Close(t)

	// Env 2: reopen the same database, simulating a server restart.
	// app.New calls repos.RecoverOrphans internally, so the orphaned run must
	// be flipped to "error" by the time BuildEnvAt returns.
	env2 := kit.BuildEnvAt(t, homeDir)

	// The running list must be empty after recovery.
	runningAfterResp := env2.GET(t, "/v0/runs/running")
	kit.RequireStatus(t, runningAfterResp, http.StatusOK)
	var runningAfter []map[string]any
	kit.DecodeEnvData(t, runningAfterResp, &runningAfter)
	s.Assert().Empty(
		runningAfter,
		"RecoverOrphans must drain all running runs on restart",
	)

	// The recovered run must carry the error status.
	recoveredResp := env2.GET(t, "/v0/runs/"+runID)
	kit.RequireStatus(t, recoveredResp, http.StatusOK)
	var recoveredRun map[string]any
	kit.DecodeEnvData(t, recoveredResp, &recoveredRun)
	s.Assert().Equal(
		"error",
		recoveredRun["status"],
		"orphaned run must be marked error after restart",
	)

	// The chat status must be reset to idle as part of crash recovery.
	chatGetResp := env2.GET(t, "/v0/workspaces/"+wsID+"/chats")
	kit.RequireStatus(t, chatGetResp, http.StatusOK)
	var chats []map[string]any
	kit.DecodeEnvData(t, chatGetResp, &chats)
	var chatStatus string
	for _, c := range chats {
		if c["id"] == chatID {
			if st, ok := c["status"].(string); ok {
				chatStatus = st
			}
			break
		}
	}
	s.Assert().Equal(
		"idle",
		chatStatus,
		"crash recovery must reset chat status from agent_running back to idle",
	)
}

// TestCrash_FailedRunNoLongerInRunningList verifies that after recovery marks a
// run as failed, ListRunning no longer includes it. This exercises the state
// transition that RecoverOrphans relies on to detect and repair orphaned runs.
func (s *CrashSuite) TestCrash_FailedRunNoLongerInRunningList() {
	s.Require().Nil(
		s.Env,
		"CrashSuite tests must not use s.Env; call kit.BuildEnv locally",
	)
	t := s.T()

	env := kit.BuildEnv(t)

	base := strings.ReplaceAll(
		s.T().Name(),
		"/",
		"-",
	)
	repoID := base + "-repo"
	runID := base + "-run"
	var wsID, chatID string

	// Create a git repo on disk so the workspace worktree path is valid.
	repoPath := kit.InitRepo(t)

	// Register the repo via HTTP.
	repoResp := env.POST(t, "/v0/repos", map[string]any{
		"id":        repoID,
		"projectId": "p1",
		"name":      "reconcile-repo",
		"path":      repoPath,
	})
	kit.RequireStatus(t, repoResp, http.StatusCreated)
	repoResp.Body.Close()

	// Create a workspace; capture the server-assigned UUID.
	wsResp := env.POST(t, "/v0/workspaces", map[string]any{
		"repoId": repoID,
		"branch": "main",
	})
	kit.RequireStatus(t, wsResp, http.StatusCreated)
	wsID = kit.MutationID(t, wsResp)

	// Create a chat; capture the server-assigned UUID.
	chatResp := env.POST(t, "/v0/workspaces/"+wsID+"/chats", map[string]any{
		"title": "Reconcile Chat",
	})
	kit.RequireStatus(t, chatResp, http.StatusCreated)
	var chatDTO map[string]any
	kit.DecodeEnvData(t, chatResp, &chatDTO)
	chatID = chatDTO["id"].(string)

	// Create a run (agentrun handler accepts explicit id).
	runResp := env.POST(t, "/v0/workspaces/"+wsID+"/runs", map[string]any{
		"id":     runID,
		"chatId": chatID,
	})
	kit.RequireStatus(t, runResp, http.StatusCreated)

	// Start the run (transitions it to "running").
	startResp := env.POST(t, "/v0/runs/"+runID+"/start", nil)
	kit.RequireStatus(t, startResp, http.StatusOK)

	// Verify it's running before recovery.
	runningResp := env.GET(t, "/v0/runs/running")
	kit.RequireStatus(t, runningResp, http.StatusOK)
	var runningList []map[string]any
	kit.DecodeEnvData(t, runningResp, &runningList)
	runningIDs := map[string]bool{}
	for _, r := range runningList {
		if id, ok := r["id"].(string); ok {
			runningIDs[id] = true
		}
	}
	s.Require().True(
		runningIDs[runID],
		"run must appear in ListRunning before recovery",
	)

	// Simulate recovery: fail the orphaned run via HTTP.
	// Note: the domain calls this terminal state "error" (AgentRunStatusError /
	// Fail); the word "failed" used in the test name refers to the intent, not
	// the domain constant.
	failResp := env.POST(t, "/v0/runs/"+runID+"/fail", nil)
	kit.RequireStatus(t, failResp, http.StatusOK)

	// After Fail, the run must no longer be in the running list.
	runningAfterResp := env.GET(t, "/v0/runs/running")
	kit.RequireStatus(t, runningAfterResp, http.StatusOK)
	var runningAfter []map[string]any
	kit.DecodeEnvData(t, runningAfterResp, &runningAfter)
	runningAfterIDs := map[string]bool{}
	for _, r := range runningAfter {
		if id, ok := r["id"].(string); ok {
			runningAfterIDs[id] = true
		}
	}
	s.Assert().False(
		runningAfterIDs[runID],
		"failed run must be absent from ListRunning after recovery",
	)
}

// TestCrash_NewEnvStartsClean verifies that a fresh Env has no running AgentRuns.
func (s *CrashSuite) TestCrash_NewEnvStartsClean() {
	s.Require().Nil(
		s.Env,
		"CrashSuite tests must not use s.Env; call kit.BuildEnv locally",
	)
	t := s.T()

	env := kit.BuildEnv(t)

	runningResp := env.GET(t, "/v0/runs/running")
	kit.RequireStatus(t, runningResp, http.StatusOK)
	var running []map[string]any
	kit.DecodeEnvData(t, runningResp, &running)
	s.Assert().Empty(
		running,
		"fresh environment must start with no running agent runs",
	)
}
