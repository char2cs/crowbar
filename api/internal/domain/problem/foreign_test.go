package problem_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"testing"

	asynxmodels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
)

func TestCodeOf_ForeignErrors(t *testing.T) {
	cases := []struct {
		err  error
		want problem.Code
	}{
		{fs.ErrNotExist, problem.NotFound},
		{fs.ErrExist, problem.Conflict},
		{fs.ErrInvalid, problem.InvalidArgument},
		{asynxmodels.ErrNotFound, problem.NotFound},
		{asynxmodels.ErrValidation, problem.Unprocessable},
		{asynxmodels.ErrPipelineFailed, problem.Conflict},
		{asynxmodels.ErrQueueFull, problem.Unavailable},
		{asynxmodels.ErrShuttingDown, problem.Unavailable},
		{asynxmodels.ErrDeliveryParked, problem.Unavailable},
		{asynxmodels.ErrForgetInProgress, problem.Conflict},
		{context.DeadlineExceeded, problem.Timeout},
	}
	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			assert.Equal(t, tc.want, problem.CodeOf(fmt.Errorf("op: %w", tc.err)))
		})
	}
}

// The fs engine hands up the os error itself; its errno is what says "missing".
func TestCodeOf_AnOSPathErrorIsNotFound(t *testing.T) {
	_, err := os.Lstat(t.TempDir() + "/ghost")

	assert.Equal(t, problem.NotFound, problem.CodeOf(fmt.Errorf("mutate: copy: %w", err)))
}

// Every sentinel the old libs status union named answers the status it did,
// bare and wrapped: the usecases annotate before a handler sees them.
func TestCodeOf_EverySentinelKeepsItsStatus(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{problem.ErrNotFound, http.StatusNotFound},
		{engineterminal.ErrSessionNotFound, http.StatusNotFound},
		{project.ErrFolderNotFound, http.StatusNotFound},
		{enginegit.ErrBranchNotFound, http.StatusNotFound},
		{agentchat.ErrNotFound, http.StatusNotFound},
		{agentusecase.ErrNoLiveRunner, http.StatusNotFound},
		{repoattachments.ErrNotFound, http.StatusNotFound},
		{tree.ErrUnknownNode, http.StatusNotFound},
		{enginesearch.ErrBadPattern, http.StatusBadRequest},
		{enginesearch.ErrPathOutsideWorkspace, http.StatusBadRequest},
		{safepath.ErrPathEscapesWorkspace, http.StatusBadRequest},
		{safepath.ErrNotAbsolute, http.StatusBadRequest},
		{safepath.ErrDanglingSymlink, http.StatusBadRequest},
		{problem.ErrInvalidArgument, http.StatusBadRequest},
		{agentusecase.ErrTreeNameRequired, http.StatusBadRequest},
		{enginegit.ErrNoRemote, http.StatusBadRequest},
		{safepath.ErrFileTooLarge, http.StatusRequestEntityTooLarge},
		{enginegit.ErrAuthFailed, http.StatusForbidden},
		{domain.ErrHostDocumentNeedsLocalTransport, http.StatusForbidden},
		{domain.ErrDaemonStateDocument, http.StatusForbidden},
		{agentusecase.ErrToolUnauthorized, http.StatusForbidden},
		{engineterminal.ErrCommandNotFound, http.StatusFailedDependency},
		{problem.ErrFailedDependency, http.StatusFailedDependency},
		{agentusecase.ErrProviderExitedDuringStartup, http.StatusFailedDependency},
		{problem.ErrUnprocessable, http.StatusUnprocessableEntity},
		{problem.ErrTimeout, http.StatusGatewayTimeout},
		{problem.ErrBadGateway, http.StatusBadGateway},
		{problem.ErrUnavailable, http.StatusServiceUnavailable},
		{problem.ErrLocked, http.StatusConflict},
		{problem.ErrConflict, http.StatusConflict},
		{enginesearch.ErrLocked, http.StatusConflict},
		{workspace.ErrParentLocked, http.StatusConflict},
		{workspace.ErrWorkspaceLocked, http.StatusConflict},
		{workspace.ErrWorkspaceIsDefault, http.StatusConflict},
		{workspace.ErrWorkspaceIsHome, http.StatusConflict},
		{workspace.ErrParentUnprovisioned, http.StatusConflict},
		{project.ErrRepoAlreadyImported, http.StatusConflict},
		{workspace.ErrRebaseNonLeaf, http.StatusConflict},
		{workspace.ErrChildHasChildren, http.StatusConflict},
		{workspace.ErrBranchWorkspaceExists, http.StatusConflict},
		{workspace.ErrRenameTargetExists, http.StatusConflict},
		{workspace.ErrRenameUnmanagedWorkspace, http.StatusConflict},
		{workspace.ErrWorkspaceWorking, http.StatusConflict},
		{workspace.ErrCrossRepoWorktreeMove, http.StatusConflict},
		{domain.ErrWorkAtRisk, http.StatusConflict},
		{agentusecase.ErrAlreadyPromoted, http.StatusConflict},
		{agentusecase.ErrNoForkParent, http.StatusConflict},
		{agentusecase.ErrNothingToPromote, http.StatusConflict},
		{agentusecase.ErrTreeWorkspaceUnprovisioned, http.StatusConflict},
		{branchreview.ErrWorkspaceUnprovisioned, http.StatusConflict},
		{agentusecase.ErrTreeCycle, http.StatusConflict},
		{agentusecase.ErrTreeCrossWorkspace, http.StatusConflict},
		{agentusecase.ErrTreeCrossContext, http.StatusConflict},
		{agentusecase.ErrTreeForkChain, http.StatusConflict},
		{agentusecase.ErrTreeSubtreeWorking, http.StatusConflict},
		{enginegit.ErrConflict, http.StatusConflict},
		{enginegit.ErrDirtyTree, http.StatusConflict},
		{enginegit.ErrRejectedNonFastForward, http.StatusConflict},
		{enginegit.ErrNothingToCommit, http.StatusConflict},
		{enginegit.ErrStaleHunk, http.StatusConflict},
		{enginegit.ErrHasChildren, http.StatusConflict},
		{enginegit.ErrBranchAlreadyExists, http.StatusConflict},
		{enginegit.ErrNonFastForward, http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			assert.Equal(t, tc.status, problem.CodeOf(tc.err).Status())
			wrapped := fmt.Errorf("agent chat folder: move f1: %w", tc.err)
			assert.Equal(t, tc.status, problem.CodeOf(wrapped).Status())
			// A declared sentinel speaks for itself; a bare category through its wrap.
			want := tc.err.Error()
			if isCategory(tc.err) {
				want = wrapped.Error()
			}
			assert.Equal(t, want, problem.Of(wrapped).Message)
		})
	}
}

// The reparent and retry refusals the old union missed are conflicts, the
// family of the other reparent and branch refusals; they answered 500.
func TestCodeOf_SelfParentIsAConflict(t *testing.T) {
	for _, err := range []error{
		workspace.ErrSelfParent, workspace.ErrBranchStillHeld, workspace.ErrBranchHeldByManagedWorkspace,
	} {
		assert.Equal(t, problem.Conflict, problem.CodeOf(fmt.Errorf("reparent: %w", err)), err.Error())
	}
}

// The usecases that wrap two sentinels at once order them arbitrarily; each
// keeps the status the old union gave it.
func TestCodeOf_PriorityAtTheMultiWrapSites(t *testing.T) {
	raw := errors.New("raw")
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"ingress telemetry", fmt.Errorf("agent: telemetry: %w: %w", problem.ErrUnprocessable, raw), http.StatusUnprocessableEntity},
		{"branch review search", fmt.Errorf("branch review: search: %w: %w", problem.ErrInvalidArgument, enginesearch.ErrBadPattern), http.StatusBadRequest},
		{"branch review lookup", fmt.Errorf("%w: %w", problem.ErrNotFound, asynxmodels.ErrNotFound), http.StatusNotFound},
		{"answer desk decide", fmt.Errorf("%w: %w", problem.ErrInvalidArgument, raw), http.StatusBadRequest},
		{"provider", fmt.Errorf("agent: provider %q: %w: %w", "p", problem.ErrInvalidArgument, problem.ErrUnprocessable), http.StatusBadRequest},
		{"chat feed", fmt.Errorf("chat feed: c: %w: %w", agentchat.ErrNotFound, problem.ErrNotFound), http.StatusNotFound},
		{"chat get", fmt.Errorf("%w: %w", asynxmodels.ErrPipelineFailed, problem.ErrNotFound), http.StatusNotFound},
		{"set order", fmt.Errorf("set order: %w: %w", raw, asynxmodels.ErrValidation), http.StatusUnprocessableEntity},
		{"queue full under a conflict", fmt.Errorf("%w: %w", problem.ErrConflict, asynxmodels.ErrQueueFull), http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, problem.CodeOf(tc.err).Status())
		})
	}
}

func isCategory(err error) bool {
	for _, c := range []error{
		problem.ErrNotFound, problem.ErrLocked, problem.ErrInvalidArgument, problem.ErrUnavailable, problem.ErrConflict,
		problem.ErrUnprocessable, problem.ErrTimeout, problem.ErrBadGateway, problem.ErrFailedDependency,
	} {
		if err == c { //nolint:errorlint // identity of the bare sentinel is the question
			return true
		}
	}
	return false
}
