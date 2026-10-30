package mocks

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// AgentWorkspaceGitStatus fakes the chat tree usecase's WorkspaceGitStatus
// seam: the read-model facts about each workspace, keyed by workspace id,
// with no live git call behind it.
type AgentWorkspaceGitStatus struct {
	// Repos answers RepoOf, keyed by workspace id; a workspace never Set here
	// answers "", as the real home workspace does.
	Repos map[string]string
	Err   error
	// HomeRepoMembers answers RepoIDsForHome, keyed by home workspace id; one
	// never Set here answers nil, mergeHomeForest's "do not filter".
	HomeRepoMembers map[string]map[string]bool
	RepoIDsErr      error
	// Branches answers RendersAsBranch, keyed by workspace id. A workspace set
	// only in Repos answers false (an unlocked fork); one in neither is gone.
	Branches map[string]bool
	// ForkParents answers VisibleForkParent, keyed by workspace id. A
	// workspace never Set here answers "" — the same answer the real adapter
	// gives for a workspace that was never forked, and for one cut straight
	// off the repo's own default checkout.
	ForkParents map[string]string
	// Defaults answers DefaultWorkspaceOf, keyed by repo id.
	Defaults map[string]string
	// CreatedAts answers CreatedAtOf, keyed by workspace id; a workspace never
	// Set here answers the zero time.
	CreatedAts map[string]time.Time
	// CreatedAtErr and ForkParentErr fail only their own read.
	CreatedAtErr  error
	ForkParentErr error
	// Placeholders inverts Provisioned, keyed by workspace id: a workspace
	// never Set here is provisioned, matching the real adapter's own
	// permissive default for a row it cannot resolve.
	Placeholders map[string]bool
}

// SetCreatedAt records workspaceID's creation time for CreatedAtOf.
func (s *AgentWorkspaceGitStatus) SetCreatedAt(workspaceID string, at time.Time) {
	if s.CreatedAts == nil {
		s.CreatedAts = map[string]time.Time{}
	}
	s.CreatedAts[workspaceID] = at
}

// CreatedAtOf implements tree.WorkspaceGitStatus.
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

// BranchRowsOf implements tree.WorkspaceGitStatus off the Repos, Branches and
// Defaults the other setters recorded.
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

// SetDefault records repoID's default checkout for DefaultWorkspaceOf.
func (s *AgentWorkspaceGitStatus) SetDefault(repoID, workspaceID string) {
	if s.Defaults == nil {
		s.Defaults = map[string]string{}
	}
	s.Defaults[repoID] = workspaceID
}

// DefaultWorkspaceOf implements tree.RepoRoots.
func (s *AgentWorkspaceGitStatus) DefaultWorkspaceOf(
	ctx context.Context,
	repoID string,
) (string, error) {
	if s.Err != nil {
		return "", s.Err
	}
	return s.Defaults[repoID], nil
}

// NewAgentWorkspaceGitStatus returns an AgentWorkspaceGitStatus with no
// workspace summaries or repos recorded.
func NewAgentWorkspaceGitStatus() *AgentWorkspaceGitStatus {
	return &AgentWorkspaceGitStatus{Repos: map[string]string{}}
}

// SetHomeRepoMembers records homeWorkspaceID's own project's repo id set for
// RepoIDsForHome to answer with.
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

// HomeOfRepo implements tree.WorkspaceGitStatus, as the reverse of the
// HomeRepoMembers SetHomeRepoMembers recorded.
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

// OutlivesItsChats implements tree.WorkspaceGitStatus: a home (no repo) or a default.
func (s *AgentWorkspaceGitStatus) OutlivesItsChats(_ context.Context, workspaceID string) (bool, error) {
	if s.Err != nil {
		return false, s.Err
	}
	repo := s.Repos[workspaceID]
	return repo == "" || s.Defaults[repo] == workspaceID, nil
}

// RepoIDsForHome implements tree.WorkspaceGitStatus.
func (s *AgentWorkspaceGitStatus) RepoIDsForHome(
	ctx context.Context,
	homeWorkspaceID string,
) (map[string]bool, error) {
	if s.RepoIDsErr != nil {
		return nil, s.RepoIDsErr
	}
	return s.HomeRepoMembers[homeWorkspaceID], nil
}

// SetRepo records workspaceID's owning repo for RepoOf to answer with.
func (s *AgentWorkspaceGitStatus) SetRepo(workspaceID, repoID string) {
	s.Repos[workspaceID] = repoID
}

// Exists implements tree.WorkspaceGitStatus: a workspace is live once any
// setter here has named it.
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

// Provisioned implements tree.WorkspaceGitStatus. Provisioned unless
// SetPlaceholder named the row: the guard that reads this only ever refuses,
// so the default has to be the permissive one.
func (s *AgentWorkspaceGitStatus) Provisioned(
	ctx context.Context,
	workspaceID string,
) (bool, error) {
	if s.Err != nil {
		return false, s.Err
	}
	return !s.Placeholders[workspaceID], nil
}

// SetPlaceholder records workspaceID as a worktree-less row for Provisioned.
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

// SetBranch records whether workspaceID renders as its own sidebar row for
// RendersAsBranch to answer with.
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

// SetForkParent records workspaceID's VISIBLE fork parent — already reduced
// the way the real adapter reduces it, so "" is what a fork cut off the repo's
// own default checkout is recorded as.
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
