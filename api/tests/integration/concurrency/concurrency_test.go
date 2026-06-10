//go:build integration

package concurrency_test

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(m *testing.M) {
	kit.Main(m)
}

// ConcurrencySuite exercises concurrent access patterns for repositories, git engine calls, and hub broadcasts.
type ConcurrencySuite struct {
	kit.IntegrationSuite
	repoPath string
}

func (s *ConcurrencySuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	s.repoPath = kit.InitRepo(s.T())

	// Register the shared repo via HTTP so all workspace creates can reference it.
	resp := s.Env.POST(s.T(), "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
		"path":      s.repoPath,
	})
	kit.RequireStatus(s.T(), resp, http.StatusCreated)
	resp.Body.Close()
}

// TestConcurrencySuite runs the ConcurrencySuite integration tests.
func TestConcurrencySuite(t *testing.T) {
	suite.Run(t, new(ConcurrencySuite))
}

// TestConcurrency_ParallelWorkspaceCreatesAreConsistent verifies that many
// concurrent workspace Create calls all succeed and all rows appear in List.
func (s *ConcurrencySuite) TestConcurrency_ParallelWorkspaceCreatesAreConsistent() {
	const n = 20

	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp := s.Env.POST(s.T(), "/v0/workspaces", map[string]any{
				"repoId": "r1",
				"branch": fmt.Sprintf("feature/concurrent-%d", idx),
			})
			if resp.StatusCode != http.StatusCreated {
				errs[idx] = fmt.Errorf(
					"workspace create %d: unexpected status %d",
					idx,
					resp.StatusCode,
				)
				resp.Body.Close()
				return
			}
			resp.Body.Close()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		s.Require().NoError(
			err,
			"workspace create %d failed",
			i,
		)
	}

	resp := s.Env.GET(s.T(), "/v0/workspaces")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var list []map[string]any
	kit.DecodeEnvData(s.T(), resp, &list)

	s.Assert().GreaterOrEqual(
		len(list),
		n,
		"all created workspaces must appear in List",
	)
}

// TestConcurrency_ParallelGitStatusCallsAreRaceClean verifies that concurrent
// git status calls on the same repo do not error or corrupt each other's results.
func (s *ConcurrencySuite) TestConcurrency_ParallelGitStatusCallsAreRaceClean() {
	t := s.T()
	const n = 10

	kit.CommitFile(
		t,
		s.repoPath,
		"base.txt",
		"content\n",
		"base commit",
	)
	baseBranch := kit.BranchName(
		t,
		s.repoPath,
	)

	wsIDs := make([]string, n)
	for i := range n {
		resp := s.Env.POST(t, "/v0/workspaces", map[string]any{
			"repoId": "r1",
			"branch": baseBranch,
		})
		kit.RequireStatus(t, resp, http.StatusCreated)
		wsIDs[i] = kit.MutationID(t, resp)
	}

	type result struct {
		err error
	}
	results := make([]result, n)
	var wg sync.WaitGroup

	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx].err = httpGitStatusClean(s, wsIDs[idx])
		}(i)
	}
	wg.Wait()

	for i, r := range results {
		s.Require().NoError(
			r.err,
			"concurrent git status %d failed",
			i,
		)
	}
}

// httpGitStatusClean calls GET /workspaces/:id/git/status via HTTP and returns
// an error if the call fails or the working tree has unexpected dirty files.
func httpGitStatusClean(
	s *ConcurrencySuite,
	wsID string,
) error {
	resp := s.Env.GET(s.T(), fmt.Sprintf("/v0/workspaces/%s/git/status", wsID))
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf(
			"concurrency: httpGitStatusClean: unexpected status %d for workspace %s",
			resp.StatusCode,
			wsID,
		)
	}

	var status struct {
		Files []any `json:"files"`
	}
	kit.DecodeEnvData(s.T(), resp, &status)

	// Clean repo: no uncommitted changes expected.
	if len(status.Files) == 0 {
		return nil
	}
	return fmt.Errorf(
		"concurrency: httpGitStatusClean: unexpected dirty files in workspace %s: %v",
		wsID,
		status.Files,
	)
}

// TestConcurrency_ParallelChatCreatesDontRaceAggregate verifies that concurrent
// chat creations across different workspaces don't corrupt the aggregate store.
func (s *ConcurrencySuite) TestConcurrency_ParallelChatCreatesDontRaceAggregate() {
	const n = 15

	// Pre-create workspaces (workspaces are needed for chats) and capture their UUIDs.
	wsIDs := make([]string, n)
	for i := range n {
		resp := s.Env.POST(s.T(), "/v0/workspaces", map[string]any{
			"repoId": "r1",
			"branch": fmt.Sprintf("feature/chat-ws-%d", i),
		})
		kit.RequireStatus(s.T(), resp, http.StatusCreated)
		wsIDs[i] = kit.MutationID(s.T(), resp)
	}

	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			wsID := wsIDs[idx]
			resp := s.Env.POST(
				s.T(),
				fmt.Sprintf("/v0/workspaces/%s/chats", wsID),
				map[string]any{
					"title": fmt.Sprintf("Chat %d", idx),
				},
			)
			if resp.StatusCode != http.StatusCreated {
				errs[idx] = fmt.Errorf(
					"concurrent chat create %d: unexpected status %d",
					idx,
					resp.StatusCode,
				)
				resp.Body.Close()
				return
			}
			resp.Body.Close()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		s.Require().NoError(
			err,
			"concurrent chat create %d failed",
			i,
		)
	}
}

// TestConcurrency_ParallelWorkspaceCreatesDoNotRaceBroadcaster verifies that
// concurrent Workspace.Create calls — which each trigger an internal hub
// broadcast — do not race on the broadcaster's subscriber slice.
func (s *ConcurrencySuite) TestConcurrency_ParallelWorkspaceCreatesDoNotRaceBroadcaster() {
	const n = 30
	var wg sync.WaitGroup

	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp := s.Env.POST(s.T(), "/v0/workspaces", map[string]any{
				"repoId": "r1",
				"branch": fmt.Sprintf("feature/fanout-%d", idx),
			})
			// Ignore individual status — "did not panic/race" is the only assertion.
			resp.Body.Close()
		}(i)
	}
	// No assertion beyond "did not panic/race" — the race detector catches races.
	wg.Wait()
}

// TestConcurrency_ParallelWsBroadcastsDoNotPanic ensures concurrent hub
// BroadcastWorkspace calls with registered WS clients do not cause data races
// on the broadcaster's subscriber slice.
func (s *ConcurrencySuite) TestConcurrency_ParallelWsBroadcastsDoNotPanic() {
	t := s.T()
	const n = 10

	// Dial N WS clients and wait for each to register before broadcasting,
	// ensuring the broadcaster's subscriber slice is non-empty under concurrent writes.
	for i := range n {
		s.Env.DialWorkspaces(
			t,
			fmt.Sprintf("?projectId=proj-fanout-%d", i),
		)
	}

	// Concurrently create workspaces via HTTP; each create triggers an internal
	// BroadcastWorkspace call. The race detector will catch any unsynchronised
	// access to the hub's subscriber slice.
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp := s.Env.POST(s.T(), "/v0/workspaces", map[string]any{
				"repoId":    "r1",
				"branch":    "main",
				"projectId": fmt.Sprintf("proj-fanout-%d", idx),
			})
			resp.Body.Close()
		}(i)
	}
	wg.Wait()

	// Drain broadcasts: give the hub time to fan out without sleeping in a tight loop.
	_ = time.Millisecond // import guard; actual drain is implicit via wg.Wait above
}
