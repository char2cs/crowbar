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

func importBody(pairs ...string) string {
	var b strings.Builder
	b.WriteString(`{"branches":[`)
	for i := 0; i < len(pairs); i += 2 {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"name":%q,"chatId":%q}`, pairs[i], pairs[i+1])
	}
	b.WriteString(`]}`)
	return b.String()
}

func TestImport_TheBatchRouteHandsCreateFromImportTheRepoFacts(t *testing.T) {
	hierarchy := &fakeHierarchy{importDone: make(chan struct{})}
	remote := &fakeRemote{exists: map[string]bool{"feature/x": true, "feature/y": true}}
	r, _ := importRouterWith(remote, hierarchy, &fakeChats{})

	rec := do(r, http.MethodPost, importBatchTarget, `{"branches":[`+
		`{"name":"feature/x","chatId":"`+chatA+`","lock":true},{"name":"feature/y","chatId":"`+chatB+`"}]}`)

	op := accepted(t, rec)
	assert.Equal(t, domain.OpBranchImport, op.Kind)
	assert.Equal(t, domain.EntityRef{Kind: domain.EntityRepo, ID: "r1"}, op.Subject)
	assert.Equal(t, "p1", op.ProjectID)
	assert.Equal(t, "Import 2 branches", op.Label)
	waitClosed(t, hierarchy.importDone)
	assert.Equal(t, workspace.ImportInput{
		RepoID:        "r1",
		ProjectID:     "p1",
		RepoPath:      "/repo",
		RemoteURL:     "git@example.com:acme/app.git",
		DefaultBranch: "main",
		Branches: []workspace.ImportBranch{
			{Name: "feature/x", ChatID: chatA, Lock: true},
			{Name: "feature/y", ChatID: chatB},
		},
	}, hierarchy.gotImport, "each branch carries its own lock")
}

func TestImport_EachBranchIsARunningItemAtAccept(t *testing.T) {
	r, _ := importRouterWith(&fakeRemote{exists: map[string]bool{"a": true, "b": true}},
		&fakeHierarchy{}, &fakeChats{})

	op := accepted(t, do(r, http.MethodPost, importBatchTarget, importBody("a", chatA, "b", chatB)))

	assert.Equal(t, []domain.OperationItem{
		{Ref: domain.EntityRef{Kind: domain.EntityChat, ID: chatA}, Label: "a", State: domain.OpRunning},
		{Ref: domain.EntityRef{Kind: domain.EntityChat, ID: chatB}, Label: "b", State: domain.OpRunning},
	}, op.Items)
}

func TestImport_AFailedBranchFailsItsItemWithTheReason(t *testing.T) {
	hierarchy := &fakeHierarchy{outcomes: []workspace.ImportOutcome{
		{Branch: "a", ChatID: chatA, WorkspaceID: "ws-a", Err: errors.New("fatal: Not a directory")},
		{Branch: "b", ChatID: chatB, WorkspaceID: "ws-b"},
	}}
	r, ops := importRouterWith(&fakeRemote{exists: map[string]bool{"a": true, "b": true}}, hierarchy, &fakeChats{})

	op := ops.settled(t, do(r, http.MethodPost, importBatchTarget, importBody("a", chatA, "b", chatB)))

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
		http.MethodPost, importBatchTarget, importBody("a", "row-1"))

	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestImport_ABranchGivenTwiceIsRefused(t *testing.T) {
	rec := do(importRouter(&fakeRemote{exists: map[string]bool{"good": true}}),
		http.MethodPost, importBatchTarget, importBody("good", chatA, "good", chatB))
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestImport_ATakenChatIDIsAnIDConflict(t *testing.T) {
	r, ops := importRouterWith(&fakeRemote{exists: map[string]bool{"a": true}},
		&fakeHierarchy{}, &fakeChats{taken: map[string]bool{chatA: true}})

	rec := do(r, http.MethodPost, importBatchTarget, importBody("a", chatA))

	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), string(problem.IDConflict))
	snapshot, err := ops.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Empty(t, snapshot, "a refused import records no operation")
}

func TestImport_ARetryAfterTheChatsExistIsTheFirstOperation(t *testing.T) {
	chats := &fakeChats{}
	r, _ := importRouterWith(&fakeRemote{exists: map[string]bool{"a": true}}, &fakeHierarchy{}, chats)
	key := "7f0c6a52-6c1e-4d55-9c55-0a9a1b3f0aff"
	send := func() *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, importBatchTarget, strings.NewReader(importBody("a", chatA)))
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

func TestImport_TheRelocatedRouteStillRefusesTheDefaultBranch(t *testing.T) {
	rec := do(importRouter(&fakeRemote{exists: map[string]bool{"main": true}}),
		http.MethodPost, importBatchTarget, importBody("main", chatA))

	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "main")
}

func TestImport_RefusesABranchTheRemoteDoesNotHave(t *testing.T) {
	remote := &fakeRemote{exists: map[string]bool{"feature/real": true}}

	rec := do(importRouter(remote), http.MethodPost, importBatchTarget,
		importBody("feature/real", chatA, "feature/ghost", chatB))

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "feature/ghost")
	assert.Equal(t, 1, remote.fetchCalls)
}

func TestImport_ImportsWithoutTheCheckWhenTheFetchFails(t *testing.T) {
	remote := &fakeRemote{fetchErr: errors.New("network down")}

	rec := do(importRouter(remote), http.MethodPost, importBatchTarget, importBody("feature/x", chatA))

	assert.Less(t, rec.Code, 400, "a failed refresh must not become a refusal")
}

func TestImport_ImportsWithoutTheCheckWhenTheRefLookupErrors(t *testing.T) {
	remote := &fakeRemote{existsErr: errors.New("bad object")}

	rec := do(importRouter(remote), http.MethodPost, importBatchTarget, importBody("feature/x", chatA))

	assert.Less(t, rec.Code, 400, "a failed ref check must not become a refusal")
}

func TestImport_SkipsTheCheckWithNoRemoteWired(t *testing.T) {
	rec := do(importRouter(nil), http.MethodPost, importBatchTarget, importBody("feature/x", chatA))

	assert.Less(t, rec.Code, 400)
}
