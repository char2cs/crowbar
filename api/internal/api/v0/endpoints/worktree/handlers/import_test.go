package handlers_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	worktreehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/worktree/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// Import refuses a branch that is not on the remote SYNCHRONOUSLY, on the
// request path, where the caller is still looking.
//
// The degradation paths matter as much as the refusal. A fetch that fails, or a
// ref check that errors, must NOT turn into a refusal: the remote being
// unreachable is not evidence that a branch is absent, and importing without the
// check is the weaker but correct answer.
type fakeRemote struct {
	fetchErr   error
	exists     map[string]bool
	existsErr  error
	fetchCalls int
}

func (f *fakeRemote) FetchPrune(_ context.Context, _ string) error {
	f.fetchCalls++
	return f.fetchErr
}

func (f *fakeRemote) RemoteTrackingBranchExists(_ context.Context, _, branch string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.exists[branch], nil
}

func importRouterWith(
	remote worktreehandlers.RemoteRefs,
	hierarchy *fakeHierarchy,
	chats *fakeChats,
) (*gin.Engine, *recordedOps) {
	repos := &fakeRepos{repo: &domain.Repository{
		ID:            "r1",
		ProjectID:     "p1",
		Path:          "/repo",
		RemoteURL:     "git@example.com:acme/app.git",
		DefaultBranch: "main",
	}}
	ops := newRecordedOps()
	h := worktreehandlers.
		New(&fakeReader{}, hierarchy, repos, ops, chats).
		WithRemoteRefs(remote)
	r := gin.New()
	r.POST("/v0/projects/:projectId/repos/:repoId/chats/import-batch", h.Import)
	return r, ops
}

func importRouter(remote worktreehandlers.RemoteRefs) *gin.Engine {
	r, _ := importRouterWith(remote, &fakeHierarchy{}, &fakeChats{})
	return r
}

const importBatchTarget = "/v0/projects/p1/repos/r1/chats/import-batch"

const (
	chatA = "7f0c6a52-6c1e-4d55-9c55-0a9a1b3f0a01"
	chatB = "7f0c6a52-6c1e-4d55-9c55-0a9a1b3f0a02"
)

// importBody is the batch body for branches named with their chat ids in turn.
func importBody(lock bool, pairs ...string) string {
	var b strings.Builder
	b.WriteString(`{"branches":[`)
	for i := 0; i < len(pairs); i += 2 {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"name":%q,"chatId":%q}`, pairs[i], pairs[i+1])
	}
	fmt.Fprintf(&b, `],"lock":%t}`, lock)
	return b.String()
}

// The proof the batch route has to carry is that it resolves the repo facts off
// :repoId and hands CreateFromImport the identical input — not that it merely
// answers 202. A loop over POST .../chats would answer 202 too, and drop the
// PR-graph parenting, the missing-ancestor creation and the held-branch
// placeholder that only CreateFromImport performs.
func TestImport_TheBatchRouteHandsCreateFromImportTheRepoFacts(t *testing.T) {
	hierarchy := &fakeHierarchy{importDone: make(chan struct{})}
	remote := &fakeRemote{exists: map[string]bool{"feature/x": true}}
	r, _ := importRouterWith(remote, hierarchy, &fakeChats{})

	rec := do(r, http.MethodPost, importBatchTarget, importBody(true, "feature/x", chatA))

	op := accepted(t, rec)
	assert.Equal(t, domain.OpBranchImport, op.Kind)
	assert.Equal(t, domain.EntityRef{Kind: domain.EntityRepo, ID: "r1"}, op.Subject)
	assert.Equal(t, "p1", op.ProjectID)
	assert.Equal(t, "Import feature/x", op.Label)
	waitClosed(t, hierarchy.importDone)
	assert.Equal(t, workspace.ImportInput{
		RepoID:        "r1",
		ProjectID:     "p1",
		RepoPath:      "/repo",
		RemoteURL:     "git@example.com:acme/app.git",
		DefaultBranch: "main",
		Branches:      []workspace.ImportBranch{{Name: "feature/x", ChatID: chatA}},
		Lock:          true,
	}, hierarchy.gotImport)
}

// The 202 already names one row per branch, under the chat the client drew it
// as, so the sidebar has something to draw before any branch lands.
func TestImport_EachBranchIsARunningItemAtAccept(t *testing.T) {
	r, _ := importRouterWith(&fakeRemote{exists: map[string]bool{"a": true, "b": true}},
		&fakeHierarchy{}, &fakeChats{})

	op := accepted(t, do(r, http.MethodPost, importBatchTarget, importBody(false, "a", chatA, "b", chatB)))

	assert.Equal(t, []domain.OperationItem{
		{Ref: domain.EntityRef{Kind: domain.EntityChat, ID: chatA}, Label: "a", State: domain.OpRunning},
		{Ref: domain.EntityRef{Kind: domain.EntityChat, ID: chatB}, Label: "b", State: domain.OpRunning},
	}, op.Items)
}

// The user's bug: a branch that cannot be imported says so, on its own item,
// and the operation fails as partial while the other branch lands.
func TestImport_AFailedBranchFailsItsItemWithTheReason(t *testing.T) {
	hierarchy := &fakeHierarchy{outcomes: []workspace.ImportOutcome{
		{Branch: "a", ChatID: chatA, WorkspaceID: "ws-a", Err: errors.New("fatal: Not a directory")},
		{Branch: "b", ChatID: chatB, WorkspaceID: "ws-b"},
	}}
	r, ops := importRouterWith(&fakeRemote{exists: map[string]bool{"a": true, "b": true}}, hierarchy, &fakeChats{})

	op := ops.settled(t, do(r, http.MethodPost, importBatchTarget, importBody(false, "a", chatA, "b", chatB)))

	assert.Equal(t, domain.OpFailed, op.State)
	require.NotNil(t, op.Error)
	assert.Equal(t, problem.Partial, op.Error.Code)
	require.Len(t, op.Items, 2)
	assert.Equal(t, domain.OpFailed, op.Items[0].State)
	require.NotNil(t, op.Items[0].Error)
	assert.Contains(t, op.Items[0].Error.Message, "Not a directory")
	assert.Equal(t, domain.OpSucceeded, op.Items[1].State)
}

func TestImport_AChatIDThatIsNotAUUIDIsRefused(t *testing.T) {
	rec := do(importRouter(&fakeRemote{exists: map[string]bool{"a": true}}),
		http.MethodPost, importBatchTarget, importBody(false, "a", "row-1"))

	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestImport_ATakenChatIDIsAnIDConflict(t *testing.T) {
	r, ops := importRouterWith(&fakeRemote{exists: map[string]bool{"a": true}},
		&fakeHierarchy{}, &fakeChats{taken: map[string]bool{chatA: true}})

	rec := do(r, http.MethodPost, importBatchTarget, importBody(false, "a", chatA))

	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), string(problem.IDConflict))
	snapshot, err := ops.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Empty(t, snapshot, "a refused import records no operation")
}

// Review Focus 1: a retry after the first import created its chats is the same
// operation, not a refusal by what that attempt made.
func TestImport_ARetryAfterTheChatsExistIsTheFirstOperation(t *testing.T) {
	chats := &fakeChats{}
	r, _ := importRouterWith(&fakeRemote{exists: map[string]bool{"a": true}}, &fakeHierarchy{}, chats)
	key := "7f0c6a52-6c1e-4d55-9c55-0a9a1b3f0aff"
	send := func() *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, importBatchTarget, strings.NewReader(importBody(false, "a", chatA)))
		req.Header.Set("Idempotency-Key", key)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	first := accepted(t, send())
	chats.taken = map[string]bool{chatA: true}
	again := accepted(t, send())

	assert.Equal(t, first.ID, again.ID)
}

// The default branch is refused on the relocated route for the same reason it
// always was: the chain walk terminates there, so a 202 would promise work that
// creates nothing and leave the caller's optimistic row spinning.
func TestImport_TheRelocatedRouteStillRefusesTheDefaultBranch(t *testing.T) {
	rec := do(importRouter(&fakeRemote{exists: map[string]bool{"main": true}}),
		http.MethodPost, importBatchTarget, importBody(false, "main", chatA))

	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "main")
}

func TestImport_RefusesABranchTheRemoteDoesNotHave(t *testing.T) {
	remote := &fakeRemote{exists: map[string]bool{"feature/real": true}}

	rec := do(importRouter(remote), http.MethodPost, importBatchTarget,
		importBody(false, "feature/real", chatA, "feature/ghost", chatB))

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "feature/ghost")
	// Refreshed first: the check is against the remote as it is NOW, not as the
	// clone last heard it.
	assert.Equal(t, 1, remote.fetchCalls)
}

func TestImport_ImportsWithoutTheCheckWhenTheFetchFails(t *testing.T) {
	// An unreachable remote is not evidence that a branch is absent.
	remote := &fakeRemote{fetchErr: errors.New("network down")}

	rec := do(importRouter(remote), http.MethodPost, importBatchTarget, importBody(false, "feature/x", chatA))

	assert.Less(t, rec.Code, 400, "a failed refresh must not become a refusal")
}

func TestImport_ImportsWithoutTheCheckWhenTheRefLookupErrors(t *testing.T) {
	remote := &fakeRemote{existsErr: errors.New("bad object")}

	rec := do(importRouter(remote), http.MethodPost, importBatchTarget, importBody(false, "feature/x", chatA))

	assert.Less(t, rec.Code, 400, "a failed ref check must not become a refusal")
}

func TestImport_SkipsTheCheckWithNoRemoteWired(t *testing.T) {
	// A nil remote degrades rather than panicking — the same wiring contract
	// WithRemoteRefs documents.
	rec := do(importRouter(nil), http.MethodPost, importBatchTarget, importBody(false, "feature/x", chatA))

	assert.Less(t, rec.Code, 400)
}
