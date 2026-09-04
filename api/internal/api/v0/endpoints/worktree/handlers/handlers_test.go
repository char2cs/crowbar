package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	worktreehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/worktree/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type fakeReader struct {
	list     []domain.Workspace
	listErr  error
	synced   domain.Workspace
	syncErr  error
	gotSync  string
	syncDone chan struct{}
	// The user's own lock decision: what the handler passed down, how often, and
	// a canned refusal for the paths the daemon rejects (project home, a
	// placeholder with no worktree of its own).
	lockCalls int
	gotLocked *bool
	lockErr   error
}

func (f *fakeReader) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	return f.list, f.listErr
}

func (f *fakeReader) SyncWorkingTreeState(
	_ context.Context,
	id string,
	_ time.Time,
) (domain.Workspace, error) {
	f.gotSync = id
	if f.syncDone != nil {
		close(f.syncDone)
	}
	return f.synced, f.syncErr
}

func (f *fakeReader) SetLock(
	_ context.Context,
	id string,
	locked *bool,
) (domain.Workspace, error) {
	f.lockCalls++
	f.gotLocked = locked
	if f.lockErr != nil {
		return domain.Workspace{}, f.lockErr
	}
	return domain.Workspace{ID: id, LockOverride: locked}, nil
}

type fakeHierarchy struct {
	mergeResult  workspace.MergeResult
	mergeErr     error
	gotMergeID   string
	gotStrategy  gitdomain.MergeStrategy
	reparented   domain.Workspace
	reparentErr  error
	gotReparent  string
	gotNewParent string
	deleteErr    error
	gotDeleteID  string
	deleteDone   chan struct{}
	mergeDone    chan struct{}
	reparentDone chan struct{}
	renamed      domain.Workspace
	renameErr    error
	gotRenameID  string
	gotRenameTo  string
	gotImport    workspace.ImportInput
	importErr    error
	importDone   chan struct{}
	gotRebaseID  string
	rebaseErr    error
	rebaseDone   chan struct{}
	gotRetryID   string
	retryErr     error
	retryDone    chan struct{}
	gotDetachID  string
	detachErr    error
	detachDone   chan struct{}
}

func (f *fakeHierarchy) CreateFromImport(
	_ context.Context,
	in workspace.ImportInput,
) error {
	f.gotImport = in
	if f.importDone != nil {
		close(f.importDone)
	}
	return f.importErr
}

func (f *fakeHierarchy) RenameBranch(
	_ context.Context,
	wsID string,
	newBranch string,
) (domain.Workspace, error) {
	f.gotRenameID, f.gotRenameTo = wsID, newBranch
	if f.renameErr != nil {
		return domain.Workspace{}, f.renameErr
	}
	return f.renamed, nil
}

func (f *fakeHierarchy) MergeIntoParent(
	_ context.Context,
	childID string,
	strategy gitdomain.MergeStrategy,
) (workspace.MergeResult, error) {
	f.gotMergeID = childID
	f.gotStrategy = strategy
	if f.mergeDone != nil {
		close(f.mergeDone)
	}
	return f.mergeResult, f.mergeErr
}

func (f *fakeHierarchy) Reparent(
	_ context.Context,
	childID string,
	newParentID string,
) (domain.Workspace, error) {
	f.gotReparent = childID
	f.gotNewParent = newParentID
	if f.reparentDone != nil {
		close(f.reparentDone)
	}
	return f.reparented, f.reparentErr
}

func (f *fakeHierarchy) RebaseOntoParent(
	_ context.Context,
	childID string,
) (domain.Workspace, error) {
	f.gotRebaseID = childID
	if f.rebaseDone != nil {
		close(f.rebaseDone)
	}
	return domain.Workspace{}, f.rebaseErr
}

func (f *fakeHierarchy) DeleteCascade(
	_ context.Context,
	rootID string,
) error {
	f.gotDeleteID = rootID
	if f.deleteDone != nil {
		close(f.deleteDone)
	}
	return f.deleteErr
}

func (f *fakeHierarchy) RetryProvision(
	_ context.Context,
	wsID string,
) (domain.Workspace, error) {
	f.gotRetryID = wsID
	if f.retryDone != nil {
		close(f.retryDone)
	}
	return domain.Workspace{}, f.retryErr
}

func (f *fakeHierarchy) DetachHolder(
	_ context.Context,
	wsID string,
) (domain.Workspace, error) {
	f.gotDetachID = wsID
	if f.detachDone != nil {
		close(f.detachDone)
	}
	return domain.Workspace{}, f.detachErr
}

type fakeRepos struct {
	repo *domain.Repository
	err  error
}

func (f *fakeRepos) FindByKey(
	_ context.Context,
	_ string,
) (*domain.Repository, error) {
	return f.repo, f.err
}

type fakeLastErrors struct {
	gotID  string
	gotMsg string
	called chan struct{}
}

func (f *fakeLastErrors) SetLastError(
	_ context.Context,
	id string,
	message string,
) (domain.Workspace, error) {
	f.gotID = id
	f.gotMsg = message
	if f.called != nil {
		f.called <- struct{}{}
	}
	return domain.Workspace{ID: id, LastError: message}, nil
}

// fakeWork satisfies worktreehandlers.WorkSignal for tests that don't assert
// the working overlay.
type fakeWork struct{}

func (fakeWork) BeginWork(_ context.Context, _ string) {}
func (fakeWork) EndWork(_ context.Context, _ string)   {}
func (fakeWork) IsWorking(_ string) bool               { return false }
func (fakeWork) WorkingFor(_ string) bool              { return false }

// fakeWorktrees stands in for the chat→workspace resolver spec §3 describes:
// it records the chat id it was asked about and answers with ws, or with err.
type fakeWorktrees struct {
	ws      domain.Workspace
	err     error
	gotChat string
	calls   int
}

func (f *fakeWorktrees) Resolve(
	_ context.Context,
	chatID string,
) (domain.Workspace, error) {
	f.calls++
	f.gotChat = chatID
	return f.ws, f.err
}

// newChatRouter mounts the nine surviving chat-keyed routes on ONE Handlers
// value, under the repo-scoped prefix the production Register uses.
//
// It hands the *Handlers back because WaitAsync is the only sound completion
// signal for the fire-and-forget verbs: a test that must assert a NEGATIVE ("the
// non-leaf child was NOT deleted") can only do so once the detached goroutine is
// provably dead.
func newChatRouter(
	reader worktreehandlers.Reader,
	hierarchy worktreehandlers.Hierarchy,
	worktrees worktreehandlers.Worktrees,
) (*gin.Engine, *worktreehandlers.Handlers) {
	return newChatRouterWithErrors(reader, hierarchy, worktrees, &fakeLastErrors{})
}

// newChatRouterWithErrors is newChatRouter for the tests that inspect what a
// failed background op recorded on the entity.
func newChatRouterWithErrors(
	reader worktreehandlers.Reader,
	hierarchy worktreehandlers.Hierarchy,
	worktrees worktreehandlers.Worktrees,
	lastErrors worktreehandlers.LastErrorSetter,
) (*gin.Engine, *worktreehandlers.Handlers) {
	r := gin.New()
	h := worktreehandlers.New(reader, hierarchy, &fakeRepos{}, lastErrors, fakeWork{}).
		WithWorktrees(worktrees)
	rg := r.Group("/v0/projects/:projectId/repos/:repoId")
	rg.POST("/chats/import-batch", h.Import)
	rg.POST("/chats/:id/lock", h.ChatLock)
	rg.POST("/chats/:id/sync", h.ChatSync)
	rg.POST("/chats/:id/merge-into-parent", h.ChatMergeIntoParent)
	rg.POST("/chats/:id/reparent", h.ChatReparent)
	rg.POST("/chats/:id/rebase-onto-parent", h.ChatRebaseOntoParent)
	rg.POST("/chats/:id/retry-provision", h.ChatRetryProvision)
	rg.POST("/chats/:id/detach-holder", h.ChatDetachHolder)
	rg.PATCH("/chats/:id/branch", h.ChatRenameBranch)
	return r, h
}

// waitClosed blocks until done is closed. The close IS the "the background work
// ran" signal, so a plain receive is the whole synchronisation: it returns the
// instant the goroutine gets there, however loaded the machine. Background work
// that never runs hangs here until `go test -timeout` fires and dumps the
// goroutines — a real failure with a real stack, rather than a one-second guess
// that goes red on a busy CI box.
func waitClosed(
	t *testing.T,
	done chan struct{},
) {
	t.Helper()
	<-done
}

func do(
	r *gin.Engine,
	method string,
	target string,
	body string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, http.NoBody)
	}
	r.ServeHTTP(rec, req)
	return rec
}
