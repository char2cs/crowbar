package libs_test

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	asynxmodels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

func TestStatusAndMessageNil(t *testing.T) {
	status, msg := libs.StatusAndMessage(nil)

	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, "internal error", msg)
}

func TestStatusAndMessageUnknown(t *testing.T) {
	err := errors.New("boom")

	status, msg := libs.StatusAndMessage(err)

	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, "boom", msg)
}

func TestStatusAndMessageMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{
			name:   "not found app",
			err:    apperr.ErrNotFound,
			status: http.StatusNotFound,
		},
		{
			name:   "session not found",
			err:    engineterminal.ErrSessionNotFound,
			status: http.StatusNotFound,
		},
		{
			name:   "asynx aggregate not found",
			err:    asynxmodels.ErrNotFound,
			status: http.StatusNotFound,
		},
		{
			name:   "agentchat not found",
			err:    agentchat.ErrNotFound,
			status: http.StatusNotFound,
		},
		{
			name:   "bad pattern",
			err:    enginesearch.ErrBadPattern,
			status: http.StatusBadRequest,
		},
		{
			name:   "path outside workspace",
			err:    enginesearch.ErrPathOutsideWorkspace,
			status: http.StatusBadRequest,
		},
		{
			name:   "search locked",
			err:    enginesearch.ErrLocked,
			status: http.StatusConflict,
		},
		{
			name:   "parent locked",
			err:    worktree.ErrParentLocked,
			status: http.StatusConflict,
		},
		{
			name:   "duplicate branch workspace is conflict",
			err:    worktree.ErrBranchWorkspaceExists,
			status: http.StatusConflict,
		},
		{
			name:   "new parent locked",
			err:    worktree.ErrNewParentLocked,
			status: http.StatusConflict,
		},
		{
			name:   "rebase non leaf",
			err:    worktree.ErrRebaseNonLeaf,
			status: http.StatusConflict,
		},
		{
			name:   "child has children",
			err:    worktree.ErrChildHasChildren,
			status: http.StatusConflict,
		},
		{
			name:   "app workspace locked",
			err:    apperr.ErrLocked,
			status: http.StatusConflict,
		},
		{
			name:   "git conflict",
			err:    enginegit.ErrConflict,
			status: http.StatusConflict,
		},
		{
			name:   "git dirty tree",
			err:    enginegit.ErrDirtyTree,
			status: http.StatusConflict,
		},
		{
			name:   "git rejected non fast forward",
			err:    enginegit.ErrRejectedNonFastForward,
			status: http.StatusConflict,
		},
		{
			name:   "git nothing to commit",
			err:    enginegit.ErrNothingToCommit,
			status: http.StatusConflict,
		},
		{
			name:   "git stale hunk",
			err:    enginegit.ErrStaleHunk,
			status: http.StatusConflict,
		},
		{
			name:   "git has children",
			err:    enginegit.ErrHasChildren,
			status: http.StatusConflict,
		},
		{
			name:   "git auth failed",
			err:    enginegit.ErrAuthFailed,
			status: http.StatusForbidden,
		},
		{
			name:   "file too large",
			err:    safepath.ErrFileTooLarge,
			status: http.StatusRequestEntityTooLarge,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, msg := libs.StatusAndMessage(tc.err)
			assert.Equal(t, tc.status, status)
			assert.Equal(t, tc.err.Error(), msg)
		})
	}
}

func TestStatusAndMessageWrapped(t *testing.T) {
	wrapped := fmt.Errorf(
		"usecase: load: %w",
		apperr.ErrNotFound,
	)

	status, msg := libs.StatusAndMessage(wrapped)

	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, wrapped.Error(), msg)
}

func TestStatusAndMessageWrappedLocked(t *testing.T) {
	wrapped := fmt.Errorf(
		"git: mutate: workspace locked: %w",
		apperr.ErrLocked,
	)

	status, msg := libs.StatusAndMessage(wrapped)

	assert.Equal(t, http.StatusConflict, status)
	assert.Equal(t, wrapped.Error(), msg)
}

func TestStatusAndMessageWrappedGitConflict(t *testing.T) {
	wrapped := fmt.Errorf(
		"git: mutate: %w",
		enginegit.ErrConflict,
	)

	status, msg := libs.StatusAndMessage(wrapped)

	assert.Equal(t, http.StatusConflict, status)
	assert.Equal(t, wrapped.Error(), msg)
}

func TestStatusAndMessageWrappedGitAuthFailed(t *testing.T) {
	wrapped := fmt.Errorf(
		"git: mutate: %w",
		enginegit.ErrAuthFailed,
	)

	status, msg := libs.StatusAndMessage(wrapped)

	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, wrapped.Error(), msg)
}

func TestStatusAndMessageWrappedFSNotExist(t *testing.T) {
	wrapped := fmt.Errorf(
		"read: %w",
		os.ErrNotExist,
	)

	status, msg := libs.StatusAndMessage(wrapped)

	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, wrapped.Error(), msg)
}

func TestStatusAndMessageWrappedAsynxNotFound(t *testing.T) {
	wrapped := fmt.Errorf(
		"workspace: get: %w",
		asynxmodels.ErrNotFound,
	)

	status, msg := libs.StatusAndMessage(wrapped)

	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, wrapped.Error(), msg)
}

func TestStatusAndMessageWrappedFileTooLarge(t *testing.T) {
	wrapped := fmt.Errorf(
		"content: read big.bin: %w",
		safepath.ErrFileTooLarge,
	)

	status, msg := libs.StatusAndMessage(wrapped)

	assert.Equal(t, http.StatusRequestEntityTooLarge, status)
	assert.Equal(t, wrapped.Error(), msg)
}
