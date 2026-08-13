package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type stubWorkspaceRepo struct {
	workspace.Workspace
	rows []domain.Workspace
	err  error
}

func (s stubWorkspaceRepo) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	return s.rows, s.err
}

// ListInRepo mirrors List: ResolveMergeEligibility filters siblings by ID and
// repo itself (see merge_eligibility.go), so returning the unfiltered rows
// here preserves every existing test's behavior unchanged.
func (s stubWorkspaceRepo) ListInRepo(
	_ context.Context,
	_ string,
	_ string,
) ([]domain.Workspace, error) {
	return s.rows, s.err
}

type discardHub struct {
	last dto.WorkspaceDTO
}

func (h *discardHub) BroadcastProject(_ dto.ProjectDTO)                 {}
func (h *discardHub) BroadcastRepo(_ dto.RepoDTO)                       {}
func (h *discardHub) BroadcastFolder(_ dto.FolderDTO)                   {}
func (h *discardHub) BroadcastWorkspace(w dto.WorkspaceDTO)             { h.last = w }
func (h *discardHub) BroadcastThread(_ dto.ThreadDTO)                   {}
func (h *discardHub) BroadcastTerminalSession(_ dto.TerminalSessionDTO) {}
func (h *discardHub) BroadcastGit(_ string, _ gitdomain.GitStatus)      {}
func (h *discardHub) BroadcastFile(_ domain.FileChangeEvent)            {}
func (h *discardHub) BroadcastAgentChat(_, _, _ string, _ bool)         {}
func (h *discardHub) BroadcastAgentChatFolder(_, _, _ string)           {}

func (h *discardHub) BroadcastAgentRunner(_, _, _, _ string) {}

func TestBroadcastWorkspace_NoParent_EmptyEligibility(t *testing.T) {
	h := &discardHub{}
	c := &Container{
		hub:       h,
		Workspace: stubWorkspaceRepo{},
	}

	c.broadcastWorkspace(context.Background(), domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1"})

	assert.False(t, h.last.CanMergeLocally)
	assert.Equal(t, "", h.last.ParentBranch)
}

func TestBroadcastWorkspace_ListError_DegradesToEmptyEligibility(t *testing.T) {
	h := &discardHub{}
	c := &Container{
		hub:       h,
		Workspace: stubWorkspaceRepo{err: errors.New("list failed")},
	}

	c.broadcastWorkspace(
		context.Background(),
		domain.Workspace{ID: "child", ProjectID: "p1", RepoID: "r1", ParentID: "parent"},
	)

	// The broadcast still lands, just without eligibility.
	assert.Equal(t, "child", h.last.ID)
	assert.False(t, h.last.CanMergeLocally)
}

func TestBroadcastWorkspace_SkipsWrongRepoSiblings(t *testing.T) {
	h := &discardHub{}
	c := &Container{
		hub: h,
		Workspace: stubWorkspaceRepo{rows: []domain.Workspace{
			// Same id as the parent but a different repo: must be skipped.
			{ID: "parent", ProjectID: "p1", RepoID: "rX", Branch: "wrong"},
			{ID: "parent", ProjectID: "p1", RepoID: "r1", Branch: "main"},
		}},
	}

	c.broadcastWorkspace(
		context.Background(),
		domain.Workspace{ID: "child", ProjectID: "p1", RepoID: "r1", ParentID: "parent"},
	)

	assert.True(t, h.last.CanMergeLocally)
	assert.Equal(t, "main", h.last.ParentBranch)
}

func TestBroadcastWorkspace_ParentMissing_EmptyEligibility(t *testing.T) {
	h := &discardHub{}
	c := &Container{
		hub: h,
		Workspace: stubWorkspaceRepo{rows: []domain.Workspace{
			{ID: "other", ProjectID: "p1", RepoID: "r1", Branch: "x"},
		}},
	}

	c.broadcastWorkspace(
		context.Background(),
		domain.Workspace{ID: "child", ProjectID: "p1", RepoID: "r1", ParentID: "parent"},
	)

	assert.False(t, h.last.CanMergeLocally)
	assert.Equal(t, "", h.last.ParentBranch)
}

func TestBroadcastWorkspace_ParentLocked_NotEligible(t *testing.T) {
	h := &discardHub{}
	c := &Container{
		hub: h,
		Workspace: stubWorkspaceRepo{rows: []domain.Workspace{
			{ID: "parent", ProjectID: "p1", RepoID: "r1", Branch: "main", Status: domain.WorkspaceStatusLocked},
		}},
	}

	c.broadcastWorkspace(
		context.Background(),
		domain.Workspace{ID: "child", ProjectID: "p1", RepoID: "r1", ParentID: "parent"},
	)

	assert.False(t, h.last.CanMergeLocally)
	assert.Equal(t, "main", h.last.ParentBranch)
}
