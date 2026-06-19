package git_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/git"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubGit struct{}

func (stubGit) Status(_ context.Context, _ string) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, nil
}

func (stubGit) Diff(_ context.Context, _ string, _ bool) ([]gitdomain.FileDiff, error) {
	return nil, nil
}

func (stubGit) Log(_ context.Context, _ string, _ int, _ int) ([]gitdomain.Commit, error) {
	return nil, nil
}

func (stubGit) Blame(_ context.Context, _ string, _ string) ([]gitdomain.BlameEntry, error) {
	return nil, nil
}

func (stubGit) Branches(_ context.Context, _ string) ([]gitdomain.Branch, error) {
	return nil, nil
}

func (stubGit) Stashes(_ context.Context, _ string) ([]gitdomain.Stash, error) {
	return nil, nil
}
func (stubGit) ConflictedFiles(_ context.Context, _ string) ([]string, error) { return nil, nil }
func (stubGit) ConflictHunks(_ context.Context, _ string, _ string) ([]gitdomain.ConflictHunk, error) {
	return nil, nil
}

func (stubGit) CommitDiff(_ context.Context, _ string, _ string) (gitdomain.MultiFileDiff, error) {
	return gitdomain.MultiFileDiff{}, nil
}

func (stubGit) StageFile(_ context.Context, _ string, _ string, _ time.Time) error { return nil }

func (stubGit) StageHunk(_ context.Context, _ string, _ string, _ string, _ time.Time) error {
	return nil
}
func (stubGit) UnstageFile(_ context.Context, _ string, _ string, _ time.Time) error { return nil }
func (stubGit) UnstageHunk(_ context.Context, _ string, _ string, _ string, _ time.Time) error {
	return nil
}
func (stubGit) Discard(_ context.Context, _ string, _ string, _ time.Time) error { return nil }
func (stubGit) Commit(_ context.Context, _ string, _ string, _ string, _ time.Time) error {
	return nil
}
func (stubGit) Push(_ context.Context, _ string, _ time.Time) error  { return nil }
func (stubGit) Fetch(_ context.Context, _ string, _ time.Time) error { return nil }
func (stubGit) Pull(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}

func (stubGit) CreateBranch(_ context.Context, _ string, _ string, _ string, _ bool, _ time.Time) error {
	return nil
}

func (stubGit) RenameBranch(_ context.Context, _ string, _ string, _ string, _ time.Time) error {
	return nil
}

func (stubGit) DeleteBranch(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}

func (stubGit) SwitchBranch(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}
func (stubGit) StashPush(_ context.Context, _ string, _ string, _ time.Time) error { return nil }
func (stubGit) StashApply(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}
func (stubGit) StashPop(_ context.Context, _ string, _ string, _ time.Time) error  { return nil }
func (stubGit) StashDrop(_ context.Context, _ string, _ string, _ time.Time) error { return nil }
func (stubGit) Reset(_ context.Context, _ string, _ string, _ string, _ time.Time) error {
	return nil
}
func (stubGit) Merge(_ context.Context, _ string, _ string, _ time.Time) error  { return nil }
func (stubGit) Rebase(_ context.Context, _ string, _ string, _ time.Time) error { return nil }
func (stubGit) ResolveHunk(_ context.Context, _ string, _ string, _ string, _ gitdomain.ConflictResolution, _ string, _ time.Time) error {
	return nil
}
func (stubGit) OperationContinue(_ context.Context, _ string, _ time.Time) error { return nil }
func (stubGit) OperationAbort(_ context.Context, _ string, _ time.Time) error    { return nil }

type stubLastErrors struct{}

func (stubLastErrors) SetLastError(
	_ context.Context,
	id string,
	message string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, LastError: message}, nil
}

func passthrough(
	rest gin.HandlerFunc,
	_ gin.HandlerFunc,
) gin.HandlerFunc {
	return rest
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	git.Register(r.Group("/v0"), stubGit{}, stubLastErrors{}, func(_ *gin.Context) {}, passthrough)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v0/workspaces/ws1/git/status"},
		{http.MethodGet, "/v0/workspaces/ws1/git/diff"},
		{http.MethodGet, "/v0/workspaces/ws1/git/log"},
		{http.MethodGet, "/v0/workspaces/ws1/git/blame"},
		{http.MethodGet, "/v0/workspaces/ws1/git/branches"},
		{http.MethodGet, "/v0/workspaces/ws1/git/stashes"},
		{http.MethodGet, "/v0/workspaces/ws1/git/conflicts"},
		{http.MethodGet, "/v0/workspaces/ws1/git/conflict-hunks"},
		{http.MethodGet, "/v0/workspaces/ws1/git/commit-diff"},
		{http.MethodPost, "/v0/workspaces/ws1/git/stage"},
		{http.MethodPost, "/v0/workspaces/ws1/git/stage-hunk"},
		{http.MethodPost, "/v0/workspaces/ws1/git/unstage"},
		{http.MethodPost, "/v0/workspaces/ws1/git/unstage-hunk"},
		{http.MethodPost, "/v0/workspaces/ws1/git/discard"},
		{http.MethodPost, "/v0/workspaces/ws1/git/commit"},
		{http.MethodPost, "/v0/workspaces/ws1/git/push"},
		{http.MethodPost, "/v0/workspaces/ws1/git/fetch"},
		{http.MethodPost, "/v0/workspaces/ws1/git/pull"},
		{http.MethodPost, "/v0/workspaces/ws1/git/branches"},
		{http.MethodPatch, "/v0/workspaces/ws1/git/branches"},
		{http.MethodDelete, "/v0/workspaces/ws1/git/branches"},
		{http.MethodPost, "/v0/workspaces/ws1/git/switch"},
		{http.MethodPost, "/v0/workspaces/ws1/git/stash"},
		{http.MethodPost, "/v0/workspaces/ws1/git/stash-apply"},
		{http.MethodPost, "/v0/workspaces/ws1/git/stash-pop"},
		{http.MethodDelete, "/v0/workspaces/ws1/git/stash"},
		{http.MethodPost, "/v0/workspaces/ws1/git/reset"},
		{http.MethodPost, "/v0/workspaces/ws1/git/merge"},
		{http.MethodPost, "/v0/workspaces/ws1/git/rebase"},
		{http.MethodPost, "/v0/workspaces/ws1/git/resolve-hunk"},
		{http.MethodPost, "/v0/workspaces/ws1/git/operation/continue"},
		{http.MethodPost, "/v0/workspaces/ws1/git/operation/abort"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
