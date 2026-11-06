package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	worktreehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/worktree/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
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
	outcomes     []workspace.ImportOutcome
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
	p workspace.ImportProgress,
) error {
	f.gotImport = in
	for _, o := range f.outcomes {
		p.Branch(o)
	}
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

func (*fakeHierarchy) GuardMergeFold(context.Context, string) error { return nil }

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
	_ domain.DeleteConsent,
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

// fakeChats answers GetChat for the chat ids in taken; any other is not found.
type fakeChats struct {
	taken map[string]bool
}

func (f *fakeChats) GetChat(
	_ context.Context,
	id string,
) (domain.Chat, error) {
	if f.taken[id] {
		return domain.Chat{ID: id}, nil
	}
	return domain.Chat{}, problem.ErrNotFound
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

// recordedOps is a real operation service whose every published row lands on
// frames, so a test reads what the client would see on the operations stream.
type recordedOps struct {
	*operation.Service
	frames chan domain.Operation
}

func newRecordedOps() *recordedOps {
	db, err := storesqlite.OpenDB(":memory:")
	if err != nil {
		panic(err)
	}
	opStore, err := storesqlite.NewFromDB[domain.Operation, string](db)
	if err != nil {
		panic(err)
	}
	frames := make(chan domain.Operation, 64)
	return &recordedOps{
		Service: operation.New(opStore, func(op domain.Operation) { frames <- op }, time.Now),
		frames:  frames,
	}
}

// accepted decodes the operation a 202 answers with.
func accepted(t *testing.T, rec *httptest.ResponseRecorder) domain.Operation {
	t.Helper()
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	var env struct {
		Data struct {
			Operation domain.Operation `json:"operation"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	return env.Data.Operation
}

// settled reads frames until the operation the 202 answered with is settled;
// its work has returned by then, so a negative after it is exact.
func (o *recordedOps) settled(t *testing.T, rec *httptest.ResponseRecorder) domain.Operation {
	t.Helper()
	id := accepted(t, rec).ID
	deadline := time.After(waitBound)
	for {
		select {
		case op := <-o.frames:
			if op.ID == id && (op.State == domain.OpSucceeded || op.State == domain.OpFailed) {
				return op
			}
		case <-deadline:
			t.Fatalf("operation %s never settled", id)
			return domain.Operation{}
		}
	}
}

// newChatRouter mounts the nine surviving chat-keyed routes on ONE Handlers
// value, under the repo-scoped prefix the production Register uses, and hands
// back the operations they start.
func newChatRouter(
	reader worktreehandlers.Reader,
	hierarchy worktreehandlers.Hierarchy,
	worktrees worktreehandlers.Worktrees,
) (*gin.Engine, *recordedOps) {
	r := gin.New()
	ops := newRecordedOps()
	h := worktreehandlers.New(reader, hierarchy, &fakeRepos{}, ops, &fakeChats{}).
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
	return r, ops
}

// waitClosed blocks until done is closed: the close IS the "the background
// work ran" signal. The bound only turns a hang into a failure.
func waitClosed(
	t *testing.T,
	done chan struct{},
) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(waitBound):
		t.Fatal("the background work never ran")
	}
}

// waitBound caps a wait on a signal the code under test must send.
const waitBound = 10 * time.Second

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
