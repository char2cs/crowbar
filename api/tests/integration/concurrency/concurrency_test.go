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

// ConcurrencySuite exercises concurrent access patterns for repositories, git
// engine calls, and hub broadcasts over the hierarchical, 202+WS API.
type ConcurrencySuite struct {
	kit.IntegrationSuite
	imported kit.ImportedRepo
}

func (s *ConcurrencySuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	s.imported = s.Env.ImportRepo(s.T(), "concurrency", "")
}

// workspacesBase returns the repo-scoped workspaces route prefix.
func (s *ConcurrencySuite) workspacesBase() string {
	return "/v0/projects/" + s.imported.ProjectID + "/repos/" + s.imported.RepoID + "/workspaces"
}

// TestConcurrencySuite runs the ConcurrencySuite integration tests.
func TestConcurrencySuite(t *testing.T) {
	suite.Run(t, new(ConcurrencySuite))
}

// TestConcurrency_ParallelWorkspaceCreatesAreConsistent verifies that many
// concurrent workspace Create calls (each 202) all eventually project and appear
// on the repo-scoped Workspaces WS as distinct status:"new" frames. The WS is
// dialled BEFORE the POSTs so no create broadcast is missed.
func (s *ConcurrencySuite) TestConcurrency_ParallelWorkspaceCreatesAreConsistent() {
	t := s.T()
	const n = 20

	watcher := s.Env.DialWorkspaces(t, s.imported.ProjectID, s.imported.RepoID)

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp := s.Env.POST(s.T(), s.workspacesBase(), map[string]any{
				"branch": fmt.Sprintf("feature/concurrent-%d", idx),
			})
			if resp.StatusCode != http.StatusAccepted {
				errs[idx] = fmt.Errorf("workspace create %d: unexpected status %d", idx, resp.StatusCode)
			}
			resp.Body.Close()
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		s.Require().NoError(err, "workspace create %d failed", i)
	}

	// Drain WS frames until all n distinct created branches have been observed in
	// a status:"new" frame (projection-complete, no time.Sleep).
	seen := map[string]bool{}
	watcher.ReadUntil(t, 15*time.Second, func(m map[string]any) bool {
		if m["status"] != "new" {
			return false
		}
		branch, _ := m["branch"].(string)
		if branch != "" {
			seen[branch] = true
		}
		return len(seen) >= n
	})
	s.Assert().GreaterOrEqual(len(seen), n, "all created workspaces must broadcast a new frame")
}

// TestConcurrency_ParallelGitStatusCallsAreRaceClean verifies that concurrent
// git status calls across distinct workspaces do not error or corrupt results.
func (s *ConcurrencySuite) TestConcurrency_ParallelGitStatusCallsAreRaceClean() {
	t := s.T()
	const n = 10

	wsIDs := make([]string, n)
	for i := range n {
		wsIDs[i] = s.Env.CreateWorkspace(
			t,
			s.imported.ProjectID,
			s.imported.RepoID,
			fmt.Sprintf("feature/status-%d", i),
		)
	}

	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = s.httpGitStatusClean(wsIDs[idx])
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		s.Require().NoError(err, "concurrent git status %d failed", i)
	}
}

// httpGitStatusClean calls GET .../git/status via HTTP and returns an error if
// the call fails or the working tree has unexpected dirty files.
func (s *ConcurrencySuite) httpGitStatusClean(
	wsID string,
) error {
	resp := s.Env.GET(s.T(), s.workspacesBase()+"/"+wsID+"/git/status")
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
	if len(status.Files) == 0 {
		return nil
	}
	return fmt.Errorf(
		"concurrency: httpGitStatusClean: unexpected dirty files in workspace %s: %v",
		wsID,
		status.Files,
	)
}

// TestConcurrency_ParallelWorkspaceCreatesDoNotRaceBroadcaster verifies that
// concurrent Create calls — which each trigger an internal hub broadcast — do not
// race on the broadcaster's subscriber slice (race detector is the assertion).
func (s *ConcurrencySuite) TestConcurrency_ParallelWorkspaceCreatesDoNotRaceBroadcaster() {
	const n = 30
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp := s.Env.POST(s.T(), s.workspacesBase(), map[string]any{
				"branch": fmt.Sprintf("feature/fanout-%d", idx),
			})
			resp.Body.Close()
		}(i)
	}
	wg.Wait()
}

// TestConcurrency_ParallelWsBroadcastsDoNotPanic ensures concurrent broadcasts
// with registered WS clients on the repo-scoped prefix do not race the
// broadcaster's subscriber slice.
func (s *ConcurrencySuite) TestConcurrency_ParallelWsBroadcastsDoNotPanic() {
	t := s.T()
	const n = 10

	// Dial N repo-scoped WS clients so the broadcaster's subscriber slice is
	// non-empty under concurrent writes.
	for range n {
		s.Env.DialWorkspaces(t, s.imported.ProjectID, s.imported.RepoID)
	}

	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp := s.Env.POST(s.T(), s.workspacesBase(), map[string]any{
				"branch": fmt.Sprintf("feature/broadcast-%d", idx),
			})
			resp.Body.Close()
		}(i)
	}
	wg.Wait()
	_ = time.Millisecond // import guard; synchronisation is via wg.Wait above
}
