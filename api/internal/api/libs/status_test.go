package libs_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	asynxmodels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
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

func TestStatusAndMessageWrappedAsynxNotFound(t *testing.T) {
	wrapped := fmt.Errorf(
		"workspace: get: %w",
		asynxmodels.ErrNotFound,
	)

	status, msg := libs.StatusAndMessage(wrapped)

	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, wrapped.Error(), msg)
}
