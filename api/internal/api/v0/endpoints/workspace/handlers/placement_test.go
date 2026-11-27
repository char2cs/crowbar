package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/workspace/handlers"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type fakePlacer struct {
	wsID    string
	in      agentusecase.PlaceInput
	placed  domain.Chat
	shifted []domain.Chat
	err     error
}

func (f *fakePlacer) PlaceWorkspace(
	_ context.Context,
	workspaceID string,
	in agentusecase.PlaceInput,
) (domain.Chat, []domain.Chat, error) {
	f.wsID = workspaceID
	f.in = in
	return f.placed, f.shifted, f.err
}

func newRouter(
	placer handlers.Placer,
) *gin.Engine {
	r := gin.New()
	h := handlers.New(placer)
	rg := r.Group("/v0")
	ws := rg.Group("/projects/:projectId/repos/:repoId/workspaces/:wsId")
	ws.PATCH("/placement", h.PlaceWorkspace)
	return r
}

func place(
	r *gin.Engine,
	body any,
) *httptest.ResponseRecorder {
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPatch, base, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

const base = "/v0/projects/p1/repos/r1/workspaces/branch-1/placement"

func TestPlaceWorkspace_ForwardsWorkspaceIDAndBody(t *testing.T) {
	placer := &fakePlacer{placed: domain.Chat{ID: "branch-1", ParentID: "folder-a", Order: 2}}
	r := newRouter(placer)

	rec := place(r, map[string]any{"parentId": "folder-a", "order": 2})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "branch-1", placer.wsID)
	require.NotNil(t, placer.in.ParentID)
	assert.Equal(t, "folder-a", *placer.in.ParentID)
	require.NotNil(t, placer.in.Order)
	assert.Equal(t, 2, *placer.in.Order)

	var body struct {
		Data struct {
			Workspace struct {
				ID       string `json:"id"`
				ParentID string `json:"parentId"`
				Order    int    `json:"order"`
			} `json:"workspace"`
			Shifted []any `json:"shifted"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "branch-1", body.Data.Workspace.ID)
	assert.Equal(t, "folder-a", body.Data.Workspace.ParentID)
	assert.Equal(t, 2, body.Data.Workspace.Order)
	assert.Empty(t, body.Data.Shifted)
}

func TestPlaceWorkspace_OmittedFieldsStayNil(t *testing.T) {
	placer := &fakePlacer{}
	r := newRouter(placer)

	rec := place(r, map[string]any{"order": 0})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Nil(t, placer.in.ParentID)
	require.NotNil(t, placer.in.Order)
	assert.Equal(t, 0, *placer.in.Order)
}

func TestPlaceWorkspace_RejectsMalformedBody(t *testing.T) {
	placer := &fakePlacer{}
	r := newRouter(placer)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, base, bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, placer.wsID, "the usecase must never be called for a malformed body")
}

func TestPlaceWorkspace_NotFoundMapsTo404(t *testing.T) {
	placer := &fakePlacer{err: problem.ErrNotFound}
	r := newRouter(placer)

	rec := place(r, map[string]any{"order": 0})

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRegression_PlaceWorkspace_AnswersShiftedAnchorSiblingsToo(t *testing.T) {
	placer := &fakePlacer{
		placed:  domain.Chat{ID: "branch-1", Order: 0},
		shifted: []domain.Chat{{ID: "ws-branch-2", Type: domain.ChatTypeBranch, Order: 1}},
	}
	r := newRouter(placer)

	rec := place(r, map[string]any{"order": 0})

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data struct {
			Shifted []struct {
				ID    string `json:"id"`
				Order int    `json:"order"`
			} `json:"shifted"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data.Shifted, 1)
	assert.Equal(t, "ws-branch-2", body.Data.Shifted[0].ID)
	assert.Equal(t, 1, body.Data.Shifted[0].Order)
}
