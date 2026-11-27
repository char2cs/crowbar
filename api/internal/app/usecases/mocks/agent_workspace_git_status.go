package mocks

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type AgentWorkspaceGitStatus struct {
	Repos           map[string]string
	Err             error
	HomeRepoMembers map[string]map[string]bool
	RepoIDsErr      error
	Branches        map[string]bool
	ForkParents     map[string]string
	Defaults        map[string]string
	CreatedAts      map[string]time.Time
	CreatedAtErr    error
	ForkParentErr   error
	Placeholders    map[string]bool
}

func (s *AgentWorkspaceGitStatus) SetCreatedAt(workspaceID string, at time.Time) {
	if s.CreatedAts == nil {
		s.CreatedAts = map[string]time.Time{}
	}
	s.CreatedAts[workspaceID] = at
}

func (s *AgentWorkspaceGitStatus) CreatedAtOf(
	ctx context.Context,
	workspaceID string,
) (time.Time, error) {
	if s.Err != nil {
		return time.Time{}, s.Err
	}
	if s.CreatedAtErr != nil {
		return time.Time{}, s.CreatedAtErr
	}
	return s.CreatedAts[workspaceID], nil
}

func (s *AgentWorkspaceGitStatus) BranchRowsOf(
	ctx context.Context,
	repoID string,
) ([]string, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	var ids []string
	for wsID, renders := range s.Branches {
		if renders && s.Repos[wsID] == repoID && s.Defaults[repoID] != wsID {
			ids = append(ids, wsID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (s *AgentWorkspaceGitStatus) SetDefault(repoID, workspaceID string) {
	if s.Defaults == nil {
		s.Defaults = map[string]string{}
	}
	s.Defaults[repoID] = workspaceID
}

func (s *AgentWorkspaceGitStatus) DefaultWorkspaceOf(
	ctx context.Context,
	repoID string,
) (string, error) {
	if s.Err != nil {
		return "", s.Err
	}
	return s.Defaults[repoID], nil
}

func NewAgentWorkspaceGitStatus() *AgentWorkspaceGitStatus {
	return &AgentWorkspaceGitStatus{Repos: map[string]string{}}
}

func (s *AgentWorkspaceGitStatus) SetHomeRepoMembers(homeWorkspaceID string, repoIDs ...string) {
	if s.HomeRepoMembers == nil {
		s.HomeRepoMembers = map[string]map[string]bool{}
	}
	ids := make(map[string]bool, len(repoIDs))
	for _, id := range repoIDs {
		ids[id] = true
	}
	s.HomeRepoMembers[homeWorkspaceID] = ids
}

func (s *AgentWorkspaceGitStatus) HomeOfRepo(
	ctx context.Context,
	repoID string,
) (string, error) {
	if s.RepoIDsErr != nil {
		return "", s.RepoIDsErr
	}
	for homeID, members := range s.HomeRepoMembers {
		if members[repoID] {
			return homeID, nil
		}
	}
	return "", nil
}

func (s *AgentWorkspaceGitStatus) OutlivesItsChats(_ context.Context, workspaceID string) (bool, error) {
	if s.Err != nil {
		return false, s.Err
	}
	repo := s.Repos[workspaceID]
	return repo == "" || s.Defaults[repo] == workspaceID, nil
}

func (s *AgentWorkspaceGitStatus) RepoIDsForHome(
	ctx context.Context,
	homeWorkspaceID string,
) (map[string]bool, error) {
	if s.RepoIDsErr != nil {
		return nil, s.RepoIDsErr
	}
	return s.HomeRepoMembers[homeWorkspaceID], nil
}

func (s *AgentWorkspaceGitStatus) SetRepo(workspaceID, repoID string) {
	s.Repos[workspaceID] = repoID
}

func (s *AgentWorkspaceGitStatus) Exists(
	ctx context.Context,
	workspaceID string,
) (bool, error) {
	if s.Err != nil {
		return false, s.Err
	}
	_, inRepos := s.Repos[workspaceID]
	_, inBranches := s.Branches[workspaceID]
	return inRepos || inBranches, nil
}

func (s *AgentWorkspaceGitStatus) Provisioned(
	ctx context.Context,
	workspaceID string,
) (bool, error) {
	if s.Err != nil {
		return false, s.Err
	}
	return !s.Placeholders[workspaceID], nil
}

func (s *AgentWorkspaceGitStatus) SetPlaceholder(workspaceID string) {
	if s.Placeholders == nil {
		s.Placeholders = map[string]bool{}
	}
	s.Placeholders[workspaceID] = true
}

func (s *AgentWorkspaceGitStatus) RepoOf(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	if s.Err != nil {
		return "", s.Err
	}
	return s.Repos[workspaceID], nil
}

func (s *AgentWorkspaceGitStatus) SetBranch(workspaceID string, renders bool) {
	if s.Branches == nil {
		s.Branches = map[string]bool{}
	}
	s.Branches[workspaceID] = renders
}

func (s *AgentWorkspaceGitStatus) RendersAsBranch(
	ctx context.Context,
	workspaceID string,
) (bool, error) {
	if s.Err != nil {
		return false, s.Err
	}
	renders, branch := s.Branches[workspaceID]
	if _, known := s.Repos[workspaceID]; !branch && !known {
		return false, fmt.Errorf("workspace %s: %w", workspaceID, problem.ErrNotFound)
	}
	return renders, nil
}

func (s *AgentWorkspaceGitStatus) SetForkParent(workspaceID, forkParentID string) {
	if s.ForkParents == nil {
		s.ForkParents = map[string]string{}
	}
	s.ForkParents[workspaceID] = forkParentID
}

func (s *AgentWorkspaceGitStatus) VisibleForkParent(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	if s.Err != nil {
		return "", s.Err
	}
	if s.ForkParentErr != nil {
		return "", s.ForkParentErr
	}
	return s.ForkParents[workspaceID], nil
}
