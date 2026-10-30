package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// fakeRepoDeleter stands in for project.DeleteUsecase's repo delete. refuse is
// what BeginRepoDelete answers a delete without consent with.
type fakeRepoDeleter struct {
	mu       sync.Mutex
	deleted  []domain.Repository
	err      error
	refuse   error
	consents []domain.DeleteConsent
	hold     chan struct{} // when set, DeleteRepo waits for it
}

func (f *fakeRepoDeleter) BeginRepoDelete(
	_ context.Context,
	repo domain.Repository,
	consent domain.DeleteConsent,
) (domain.Repository, error) {
	if f.refuse != nil && consent == domain.KeepWorkAtRisk {
		return domain.Repository{}, f.refuse
	}
	repo.Deleting = true
	return repo, nil
}

func (f *fakeRepoDeleter) DeleteRepo(
	ctx context.Context,
	repo domain.Repository,
	consent domain.DeleteConsent,
) error {
	if f.hold != nil {
		select {
		case <-f.hold:
		case <-ctx.Done():
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.consents = append(f.consents, consent)
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, repo)
	return nil
}

func (f *fakeRepoDeleter) handed() ([]domain.Repository, []domain.DeleteConsent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.Repository(nil), f.deleted...), append([]domain.DeleteConsent(nil), f.consents...)
}

func newDeleteRouter(
	store repohandlers.Store,
	deleter repohandlers.RepoDeleter,
	ops repohandlers.Operations,
	bc *recordingRepoBroadcaster,
) *gin.Engine {
	h := repohandlers.NewWithDeps(store, nil, nil, ops, bc.push, unusedHome)
	if deleter != nil {
		h = h.WithRepoDeleter(deleter)
	}
	r := gin.New()
	r.Group("/v0/projects/:projectId/repos/:repoId").DELETE("", h.DeleteRepo)
	return r
}

func doDelete(r *gin.Engine, discard bool) *httptest.ResponseRecorder {
	return doDeleteKeyed(r, discard, "")
}

func doDeleteKeyed(r *gin.Engine, discard bool, key string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/v0/projects/p1/repos/r1", http.NoBody)
	if key != "" {
		req.Header.Set(libs.IdempotencyKeyHeader, key)
	}
	if discard {
		req.Header.Set("Crowbar-Discard-Work", "true")
	}
	r.ServeHTTP(rec, req)
	return rec
}

// A delete announces its intent, answers with its operation, and tombstones
// the repo once it is gone. The whole repo (default branch included) reaches
// the lifecycle: that branch is the one the cascade keeps (spec §3 P0-1).
func TestDelete_AnswersWithTheOperation(t *testing.T) {
	repo := domain.Repository{ID: "r1", ProjectID: "p1", Name: "alpha", Path: "/repo", DefaultBranch: "main"}
	deleter := &fakeRepoDeleter{}
	ops, bc := newRecordedOps(t), newRecordingRepoBroadcaster()

	op := ops.settled(t, doDelete(newDeleteRouter(&fakeStore{byKey: &repo}, deleter, ops, bc), false))
	assert.Equal(t, domain.OpRepoDelete, op.Kind)
	assert.Equal(t, "Remove alpha", op.Label)
	assert.Equal(t, domain.OpSucceeded, op.State)
	assert.Equal(t, []domain.OperationItem{{
		Ref: domain.EntityRef{Kind: domain.EntityRepo, ID: "r1"}, Label: "alpha", State: domain.OpSucceeded,
	}}, op.Items)
	assert.True(t, bc.next(t).Deleting, "the intent is announced first")
	tomb := bc.next(t)
	assert.Equal(t, "deleted", tomb.Status)
	assert.Equal(t, "p1", tomb.ProjectID)
	marked := repo
	marked.Deleting = true
	deleted, _ := deleter.handed()
	assert.Equal(t, []domain.Repository{marked}, deleted)
}

// A retry of a delete that finished answers with its operation, not the 404 a
// fresh lookup of the removed repo gives.
func TestDelete_ARetryAfterItFinishedAnswersWithTheOperation(t *testing.T) {
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Name: "alpha"}}
	ops, bc := newRecordedOps(t), newRecordingRepoBroadcaster()
	r := newDeleteRouter(store, &fakeRepoDeleter{}, ops, bc)
	key := uuid.NewString()

	first := ops.settled(t, doDeleteKeyed(r, false, key))
	store.byKey = nil
	retry := doDeleteKeyed(r, false, key)

	require.Equal(t, http.StatusAccepted, retry.Code, retry.Body.String())
	assert.Contains(t, retry.Body.String(), first.ID)
}

// One teardown runs per repo: a second delete under another key, while the
// first runs, is refused.
func TestDelete_ASecondDeleteWhileOneRunsIsRefused(t *testing.T) {
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Name: "alpha"}}
	deleter := &fakeRepoDeleter{hold: make(chan struct{})}
	ops, bc := newRecordedOps(t), newRecordingRepoBroadcaster()
	r := newDeleteRouter(store, deleter, ops, bc)

	first := doDeleteKeyed(r, false, uuid.NewString())
	second := doDeleteKeyed(r, false, uuid.NewString())
	close(deleter.hold)
	ops.settled(t, first)

	assert.Equal(t, http.StatusConflict, second.Code, second.Body.String())
	assert.Contains(t, second.Body.String(), "operation_running")
}

// A delete that stops fails its operation with the reason; no tombstone.
func TestDelete_AFailureIsTheOperations(t *testing.T) {
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	ops, bc := newRecordedOps(t), newRecordingRepoBroadcaster()

	op := ops.settled(t, doDelete(newDeleteRouter(store, &fakeRepoDeleter{err: errStore}, ops, bc), false))
	assert.Equal(t, domain.OpFailed, op.State)
	require.NotNil(t, op.Items[0].Error)
	assert.Contains(t, op.Items[0].Error.Message, errStore.Error())
	bc.next(t) // the recorded intent
	assertNoBroadcast(t, bc)
}

// A delete refused over work at risk answers 409 with the list and announces
// nothing; the discard header retries it with consent.
func TestDelete_WorkAtRiskIsRefusedUntilTheDiscardHeaderConfirmsIt(t *testing.T) {
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1"}}
	deleter := &fakeRepoDeleter{refuse: domain.WorkAtRisked([]domain.WorkAtRisk{
		{WorkspaceID: "w1", Branch: "feature/x", UnmergedCommits: 1},
	})}
	ops, bc := newRecordedOps(t), newRecordingRepoBroadcaster()
	r := newDeleteRouter(store, deleter, ops, bc)

	rec := doDelete(r, false)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), `"code":"work_at_risk"`)
	assert.Contains(t, rec.Body.String(), `"branch":"feature/x"`)
	assertNoBroadcast(t, bc)

	ops.settled(t, doDelete(r, true))
	_, consents := deleter.handed()
	assert.Equal(t, []domain.DeleteConsent{domain.DiscardWorkAtRisk}, consents)
}

func TestDelete_FindError_5xx(t *testing.T) {
	rec := doDelete(newDeleteRouter(&fakeStore{byKeErr: errStore}, &fakeRepoDeleter{}, newRecordedOps(t), newRecordingRepoBroadcaster()), false)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestDelete_NotFound_4xx(t *testing.T) {
	rec := doDelete(newDeleteRouter(&fakeStore{}, &fakeRepoDeleter{}, newRecordedOps(t), newRecordingRepoBroadcaster()), false)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// Unwired, the delete refuses outright rather than accepting work it cannot do.
func TestDelete_Unwired_Refuses(t *testing.T) {
	store := &fakeStore{byKey: &domain.Repository{ID: "r1", ProjectID: "p1", Path: "/repo"}}
	rec := doDelete(newDeleteRouter(store, nil, newRecordedOps(t), newRecordingRepoBroadcaster()), false)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
