//go:build integration

package concurrency_test

import (
	"fmt"
	"net/http"
	"sync"
	"testing"

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

// TestConcurrencySuite runs the ConcurrencySuite integration tests.
func TestConcurrencySuite(t *testing.T) {
	suite.Run(t, new(ConcurrencySuite))
}

// TestConcurrency_ParallelWorkspaceCreatesAreConsistent verifies that many
// concurrent workspace creates all eventually succeed and are consistently
// observable afterward with distinct branches.
//
// spec §8 step 6 deleted POST .../workspaces entirely: the only live creation
// path is CreateWorkspaceWithChat (mirroring the atomic fork+chat create the
// product surface now uses), so that is what n goroutines drive concurrently.
// The old assertion read completion off the repo-scoped Workspaces WS stream;
// that stream is gone, and its chat-feed replacement carries NO snapshot and
// silently drops any frame for a workspace with no owning chat yet
// (container.go's pushChatWorktree: "a workspace with no resolved owning chat
// pushes nothing") — so there is no reliable WS signal to drain here even with
// a chat wired up per create. Consistency is instead checked against the real
// read model, GET .../chats, which is what a client actually reads.
func (s *ConcurrencySuite) TestConcurrency_ParallelWorkspaceCreatesAreConsistent() {
	t := s.T()
	const n = 20

	var wg sync.WaitGroup
	wsIDs := make([]string, n)
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			branch := fmt.Sprintf("feature/concurrent-%d", idx)
			wsID, _ := s.Env.CreateWorkspaceWithChat(t, s.imported.ProjectID, s.imported.RepoID, branch, "")
			wsIDs[idx] = wsID
		}(i)
	}
	wg.Wait()

	chats := s.Env.WorktreeChats(t, s.imported.ProjectID, s.imported.RepoID)
	seenBranches := map[string]bool{}
	for i, wsID := range wsIDs {
		row, ok := chats[wsID]
		s.Require().True(ok, "workspace %d (%s) must appear in the repo's chat list", i, wsID)
		branch, _ := row["branch"].(string)
		s.Assert().Equal(fmt.Sprintf("feature/concurrent-%d", i), branch)
		seenBranches[branch] = true
	}
	s.Assert().Len(seenBranches, n, "all created workspaces must be independently observable with distinct branches")
}

// TestConcurrency_ParallelGitStatusCallsAreRaceClean verifies that concurrent
// git status calls across distinct workspaces do not error or corrupt results.
func (s *ConcurrencySuite) TestConcurrency_ParallelGitStatusCallsAreRaceClean() {
	t := s.T()
	const n = 10

	chatIDs := make([]string, n)
	for i := range n {
		_, chatID := s.Env.CreateWorkspaceWithChat(
			t,
			s.imported.ProjectID,
			s.imported.RepoID,
			fmt.Sprintf("feature/status-%d", i),
			"",
		)
		chatIDs[i] = chatID
	}

	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = s.httpGitStatusClean(chatIDs[idx])
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		s.Require().NoError(err, "concurrent git status %d failed", i)
	}
}

// httpGitStatusClean calls GET .../git/status via HTTP (chat-scoped, spec §8
// step 6 — the workspace-scoped mount is gone) and returns an error if the
// call fails or the working tree has unexpected dirty files.
func (s *ConcurrencySuite) httpGitStatusClean(
	chatID string,
) error {
	resp := s.Env.GET(s.T(), "/v0/chats/"+chatID+"/git/status")
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf(
			"concurrency: httpGitStatusClean: unexpected status %d for chat %s",
			resp.StatusCode,
			chatID,
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
		"concurrency: httpGitStatusClean: unexpected dirty files in chat %s: %v",
		chatID,
		status.Files,
	)
}

// TestConcurrency_ParallelWorkspaceCreatesDoNotRaceBroadcaster verifies that
// concurrent creates — which each trigger an internal hub broadcast onto the
// chat feed once their owning chat exists (container.go's pushChatWorktree) —
// do not race on the broadcaster's subscriber slice (race detector is the
// assertion). CreateWorkspaceWithChat, not the bare CreateWorkspace, is what's
// needed here: a workspace with no owning chat never reaches the broadcaster's
// Push at all, so a bare create would not exercise this path.
func (s *ConcurrencySuite) TestConcurrency_ParallelWorkspaceCreatesDoNotRaceBroadcaster() {
	t := s.T()
	const n = 30
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			branch := fmt.Sprintf("feature/fanout-%d", idx)
			s.Env.CreateWorkspaceWithChat(t, s.imported.ProjectID, s.imported.RepoID, branch, "")
		}(i)
	}
	wg.Wait()
}

// TestConcurrency_ParallelWsBroadcastsDoNotPanic ensures concurrent broadcasts
// with registered WS clients on the repo-scoped chat prefix do not race the
// broadcaster's subscriber slice.
func (s *ConcurrencySuite) TestConcurrency_ParallelWsBroadcastsDoNotPanic() {
	t := s.T()
	const n = 10

	// Dial N repo-scoped WS clients so the broadcaster's subscriber slice is
	// non-empty under concurrent writes.
	for range n {
		s.Env.DialRepoChats(t, s.imported.ProjectID, s.imported.RepoID)
	}

	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			branch := fmt.Sprintf("feature/broadcast-%d", idx)
			s.Env.CreateWorkspaceWithChat(t, s.imported.ProjectID, s.imported.RepoID, branch, "")
		}(i)
	}
	wg.Wait()
}
