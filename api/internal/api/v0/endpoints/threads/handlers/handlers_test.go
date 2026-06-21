package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/threads/handlers"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

// fakeStore records the calls the handlers make and returns canned threads so
// the broadcast + response assertions can be made without an event store.
type fakeStore struct {
	openIn      reviewthread.OpenInput
	replyID     string
	replyAuthor string
	replyIsAgent bool
	replyBody   string
	resolveID   string
	reopenID    string
	getID       string
	listWsID    string
	thread      domain.ReviewThread
	listResult  []domain.ReviewThread
	err         error
}

func (f *fakeStore) Open(
	_ context.Context,
	in reviewthread.OpenInput,
	_ time.Time,
) (domain.ReviewThread, error) {
	f.openIn = in
	return f.thread, f.err
}

func (f *fakeStore) Reply(
	_ context.Context,
	id string,
	_ string,
	author string,
	isAgent bool,
	body string,
	_ time.Time,
) (domain.ReviewThread, error) {
	f.replyID = id
	f.replyAuthor = author
	f.replyIsAgent = isAgent
	f.replyBody = body
	return f.thread, f.err
}

func (f *fakeStore) Resolve(
	_ context.Context,
	id string,
) (domain.ReviewThread, error) {
	f.resolveID = id
	return f.thread, f.err
}

func (f *fakeStore) Reopen(
	_ context.Context,
	id string,
) (domain.ReviewThread, error) {
	f.reopenID = id
	return f.thread, f.err
}

func (f *fakeStore) Get(
	_ context.Context,
	id string,
) (domain.ReviewThread, error) {
	f.getID = id
	return f.thread, f.err
}

func (f *fakeStore) ListByWorkspace(
	_ context.Context,
	wsID string,
) ([]domain.ReviewThread, error) {
	f.listWsID = wsID
	return f.listResult, f.err
}

type fakeBroadcaster struct {
	pushed []dto.ThreadDTO
}

func (b *fakeBroadcaster) Push(
	d dto.ThreadDTO,
) {
	b.pushed = append(b.pushed, d)
}

func newRouter(
	store handlers.ThreadStore,
	bc handlers.ThreadBroadcaster,
) *gin.Engine {
	r := gin.New()
	h := handlers.New(store, bc)
	rg := r.Group("/v0")
	ws := rg.Group("/projects/:projectId/repos/:repoId/workspaces/:wsId")
	ws.GET("/threads", h.List)
	ws.POST("/threads", h.OpenThread)
	ws.GET("/threads/:threadId", h.Detail)
	ws.PATCH("/threads/:threadId", h.SetResolved)
	ws.POST("/threads/:threadId/replies", h.Reply)
	return r
}

func do(
	r *gin.Engine,
	method string,
	path string,
	body any,
) *httptest.ResponseRecorder {
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// doRaw issues a request with a verbatim (possibly malformed) JSON body so the
// bind-error paths can be exercised.
func doRaw(
	r *gin.Engine,
	method string,
	path string,
	body string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

const base = "/v0/projects/p1/repos/r1/workspaces/w1/threads"

func sampleThread() domain.ReviewThread {
	return domain.ReviewThread{
		ID:         "t1",
		WsID:       "w1",
		FilePath:   "main.go",
		LineNumber: 10,
		Side:       domain.ReviewSideRight,
		Status:     domain.ReviewThreadStatusOpen,
		CreatedAt:  time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
		Messages: []domain.ReviewMessage{
			{ID: "m1", Author: "a", Body: "comment", CreatedAt: time.Now()},
		},
	}
}

// TestOpenThread_BroadcastsAndReturns asserts opening a thread persists via the
// store, returns 201 with the scoped DTO, and broadcasts the same ThreadDTO so
// the workspace-scoped stream converges (sync-mutate → broadcast).
func TestOpenThread_BroadcastsAndReturns(t *testing.T) {
	store := &fakeStore{thread: sampleThread()}
	bc := &fakeBroadcaster{}
	r := newRouter(store, bc)

	rec := do(r, http.MethodPost, base, map[string]any{
		"filePath": "main.go", "line": 10, "side": "right", "body": "comment",
	})

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "w1", store.openIn.WsID)
	assert.Equal(t, "main.go", store.openIn.FilePath)
	assert.Equal(t, 10, store.openIn.LineNumber)
	assert.Equal(t, "comment", store.openIn.Body)
	assert.NotEmpty(t, store.openIn.ID)
	assert.NotEmpty(t, store.openIn.MessageID)

	require.Len(t, bc.pushed, 1)
	assert.Equal(t, "t1", bc.pushed[0].ID)
	assert.Equal(t, "p1", bc.pushed[0].ProjectID)
	assert.Equal(t, "r1", bc.pushed[0].RepoID)
	assert.Equal(t, "w1", bc.pushed[0].WorkspaceID)

	var env struct {
		Data dto.ThreadDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.Equal(t, "t1", env.Data.ID)
	assert.Equal(t, "p1", env.Data.ProjectID)
}

// TestReply_BroadcastsAndReturns asserts replying appends through the store,
// returns 200 with the DTO, and broadcasts it.
func TestReply_BroadcastsAndReturns(t *testing.T) {
	store := &fakeStore{thread: sampleThread()}
	bc := &fakeBroadcaster{}
	r := newRouter(store, bc)

	rec := do(r, http.MethodPost, base+"/t1/replies", map[string]any{"body": "a reply"})

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "t1", store.replyID)
	assert.Equal(t, "a reply", store.replyBody)
	require.Len(t, bc.pushed, 1)
	assert.Equal(t, "t1", bc.pushed[0].ID)
}

// TestSetResolved_Resolve asserts a PATCH with isResolved=true resolves through
// the store and broadcasts the resulting DTO.
func TestSetResolved_Resolve(t *testing.T) {
	resolved := sampleThread()
	resolved.Status = domain.ReviewThreadStatusResolved
	store := &fakeStore{thread: resolved}
	bc := &fakeBroadcaster{}
	r := newRouter(store, bc)

	rec := do(r, http.MethodPatch, base+"/t1", map[string]any{"isResolved": true})

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "t1", store.resolveID)
	assert.Empty(t, store.reopenID)
	require.Len(t, bc.pushed, 1)
	assert.True(t, bc.pushed[0].Resolved)
}

// TestSetResolved_Reopen asserts a PATCH with isResolved=false reopens through
// the store and broadcasts the resulting DTO.
func TestSetResolved_Reopen(t *testing.T) {
	store := &fakeStore{thread: sampleThread()}
	bc := &fakeBroadcaster{}
	r := newRouter(store, bc)

	rec := do(r, http.MethodPatch, base+"/t1", map[string]any{"isResolved": false})

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "t1", store.reopenID)
	assert.Empty(t, store.resolveID)
	require.Len(t, bc.pushed, 1)
}

// TestDetail_ReturnsThreadWithReplies asserts GET .../threads/:threadId returns
// the single thread as a DTO with its anchor + replies and does not broadcast.
func TestDetail_ReturnsThreadWithReplies(t *testing.T) {
	thread := sampleThread()
	thread.Messages = append(thread.Messages, domain.ReviewMessage{
		ID: "m2", Author: "b", Body: "reply body", CreatedAt: time.Now(),
	})
	store := &fakeStore{thread: thread}
	bc := &fakeBroadcaster{}
	r := newRouter(store, bc)

	rec := do(r, http.MethodGet, base+"/t1", nil)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "t1", store.getID)
	assert.Empty(t, bc.pushed)

	var env struct {
		Data dto.ThreadDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.Equal(t, "comment", env.Data.Body)
	require.Len(t, env.Data.Replies, 1)
	assert.Equal(t, "reply body", env.Data.Replies[0].Body)
}

// TestList_ReturnsWorkspaceThreads asserts GET .../threads lists the workspace's
// threads as scoped DTOs and does not broadcast.
func TestList_ReturnsWorkspaceThreads(t *testing.T) {
	store := &fakeStore{listResult: []domain.ReviewThread{sampleThread()}}
	bc := &fakeBroadcaster{}
	r := newRouter(store, bc)

	rec := do(r, http.MethodGet, base, nil)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "w1", store.listWsID)
	assert.Empty(t, bc.pushed)

	var env struct {
		Data []dto.ThreadDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Len(t, env.Data, 1)
	assert.Equal(t, "p1", env.Data[0].ProjectID)
	assert.Equal(t, "r1", env.Data[0].RepoID)
}
