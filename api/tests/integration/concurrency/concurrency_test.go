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

type ConcurrencySuite struct {
	kit.IntegrationSuite
	imported kit.ImportedRepo
}

func (s *ConcurrencySuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	s.imported = s.Env.ImportRepo(s.T(), "concurrency", "")
}

func TestConcurrencySuite(t *testing.T) {
	suite.Run(t, new(ConcurrencySuite))
}

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

func (s *ConcurrencySuite) TestConcurrency_ParallelWorkspaceCreatesDoNotRaceBroadcaster() {
	t := s.T()
	const n = 30
	ready := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-ready
			branch := fmt.Sprintf("feature/fanout-%d", idx)
			s.Env.CreateWorkspaceWithChat(t, s.imported.ProjectID, s.imported.RepoID, branch, "")
		}(i)
	}
	close(ready)
	wg.Wait()
	s.Env.Quiesce()
}

func (s *ConcurrencySuite) TestConcurrency_ParallelWsBroadcastsDoNotPanic() {
	t := s.T()
	const n = 10

	for range n {
		s.Env.HoldProjects(t)
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
