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

func (stubGit) Log(_ context.Context, _ string, _, _ int) ([]gitdomain.Commit, error) {
	return nil, nil
}

func (stubGit) Blame(_ context.Context, _, _ string) ([]gitdomain.BlameEntry, error) {
	return nil, nil
}

func (stubGit) Branches(_ context.Context, _ string) ([]gitdomain.Branch, error) {
	return nil, nil
}

func (stubGit) Stashes(_ context.Context, _ string) ([]gitdomain.Stash, error) {
	return nil, nil
}
func (stubGit) ConflictedFiles(_ context.Context, _ string) ([]string, error) { return nil, nil }
func (stubGit) ConflictHunks(_ context.Context, _, _ string) ([]gitdomain.ConflictHunk, error) {
	return nil, nil
}

func (stubGit) CommitDiff(_ context.Context, _, _ string) (gitdomain.MultiFileDiff, error) {
	return gitdomain.MultiFileDiff{}, nil
}

func (stubGit) StageFile(_ context.Context, _, _ string, _ time.Time) error { return nil }

func (stubGit) StageHunk(_ context.Context, _, _, _ string, _ time.Time) error {
	return nil
}
func (stubGit) UnstageFile(_ context.Context, _, _ string, _ time.Time) error { return nil }
func (stubGit) UnstageHunk(_ context.Context, _, _, _ string, _ time.Time) error {
	return nil
}
func (stubGit) Discard(_ context.Context, _, _ string, _ time.Time) error { return nil }
func (stubGit) Commit(_ context.Context, _, _, _ string, _ time.Time) error {
	return nil
}
func (stubGit) Push(_ context.Context, _ string, _ time.Time) error  { return nil }
func (stubGit) Fetch(_ context.Context, _ string, _ time.Time) error { return nil }
func (stubGit) Pull(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}

func (stubGit) CreateBranch(_ context.Context, _, _, _ string, _ bool, _ time.Time) error {
	return nil
}

func (stubGit) RenameBranch(_ context.Context, _, _, _ string, _ time.Time) error {
	return nil
}

func (stubGit) DeleteBranch(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}

func (stubGit) SwitchBranch(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}
func (stubGit) StashPush(_ context.Context, _, _ string, _ time.Time) error { return nil }
func (stubGit) StashApply(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}
func (stubGit) StashPop(_ context.Context, _, _ string, _ time.Time) error  { return nil }
func (stubGit) StashDrop(_ context.Context, _, _ string, _ time.Time) error { return nil }
func (stubGit) Reset(_ context.Context, _, _, _ string, _ time.Time) error {
	return nil
}
func (stubGit) Merge(_ context.Context, _, _ string, _ time.Time) error  { return nil }
func (stubGit) Rebase(_ context.Context, _, _ string, _ time.Time) error { return nil }
func (stubGit) ResolveHunk(_ context.Context, _, _, _ string, _ gitdomain.ConflictResolution, _ string, _ time.Time) error {
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

type stubWork struct{}

func (stubWork) BeginWork(_ context.Context, _ string) {}
func (stubWork) EndWork(_ context.Context, _ string)   {}

func passthrough(
	rest gin.HandlerFunc,
	_ gin.HandlerFunc,
) gin.HandlerFunc {
	return rest
}

// gitSurface is the method+relative-path set git.Register mounts, written once
// and asserted against BOTH live prefixes. The relative half is deliberately
// prefix-free: a route that reached only one of the two mounts is the failure
// this shape makes impossible to miss.
func gitSurface() []struct {
	method string
	path   string
} {
	return []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/status"},
		{http.MethodGet, "/diff"},
		{http.MethodGet, "/log"},
		{http.MethodGet, "/blame"},
		{http.MethodGet, "/branches"},
		{http.MethodGet, "/stashes"},
		{http.MethodGet, "/conflicts"},
		{http.MethodGet, "/conflict-hunks"},
		{http.MethodGet, "/commit-diff"},
		{http.MethodPost, "/stage"},
		{http.MethodPost, "/stage-hunk"},
		{http.MethodPost, "/unstage"},
		{http.MethodPost, "/unstage-hunk"},
		{http.MethodPost, "/discard"},
		{http.MethodPost, "/commit"},
		{http.MethodPost, "/push"},
		{http.MethodPost, "/fetch"},
		{http.MethodPost, "/pull"},
		{http.MethodPost, "/branches"},
		{http.MethodPatch, "/branches"},
		{http.MethodDelete, "/branches"},
		{http.MethodPost, "/switch"},
		{http.MethodPost, "/stash"},
		{http.MethodPost, "/stash-apply"},
		{http.MethodPost, "/stash-pop"},
		{http.MethodDelete, "/stash"},
		{http.MethodPost, "/reset"},
		{http.MethodPost, "/merge"},
		{http.MethodPost, "/rebase"},
		{http.MethodPost, "/resolve-hunk"},
		{http.MethodPost, "/operation/continue"},
		{http.MethodPost, "/operation/abort"},
	}
}

// registerBothMounts wires git.Register the way router.go does: the old
// workspace-scoped group and the flat chat-scoped one, on one engine.
func registerBothMounts(
	t *testing.T,
) *gin.Engine {
	t.Helper()
	r := gin.New()
	v0 := r.Group("/v0")
	git.Register(
		v0,
		v0.Group("/chats/:chatId"),
		stubGit{},
		stubLastErrors{},
		stubWork{},
		func(_ *gin.Context) {},
		passthrough,
	)
	return r
}

// TestRegisterMountsChatScopedRoutes is the route half of this step: every git
// route is reachable at the flat /v0/chats/:chatId prefix (spec §7.1).
func TestRegisterMountsChatScopedRoutes(
	t *testing.T,
) {
	r := registerBothMounts(t)

	for _, tc := range gitSurface() {
		path := "/v0/chats/chat1/git" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, path)
	}
}

// TestRegisterKeepsWorkspaceScopedRoutes is the regression bar for the
// coexistence this step deliberately ships: the workspace-scoped surface is
// NOT retired here (spec §8 step 6 does that, once every group has moved), so
// every one of its routes must still answer exactly as before.
func TestRegisterKeepsWorkspaceScopedRoutes(
	t *testing.T,
) {
	r := registerBothMounts(t)

	for _, tc := range gitSurface() {
		path := "/v0/workspaces/ws1/git" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, path)
	}
}
