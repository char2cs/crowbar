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
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

// fakePlacer fakes tree.Usecase's PlaceWorkspace surface (handlers.Placer),
// recording the call it received and returning a canned result.
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

// folderFrame is one frame a placement handler pushed on the chats WS.
type folderFrame struct {
	folderID    string
	workspaceID string
	kind        string
}

func newRouter(
	placer handlers.Placer,
) *gin.Engine {
	r, _ := newRouterWithFrames(placer)
	return r
}

// newRouterWithFrames is newRouter plus the broadcast frames it captures,
// for the tests that assert on what a placement announces.
func newRouterWithFrames(
	placer handlers.Placer,
) (*gin.Engine, *[]folderFrame) {
	var frames []folderFrame
	r := gin.New()
	h := handlers.New(placer, func(folderID, workspaceID, kind string) {
		frames = append(frames, folderFrame{folderID: folderID, workspaceID: workspaceID, kind: kind})
	})
	rg := r.Group("/v0")
	ws := rg.Group("/projects/:projectId/repos/:repoId/workspaces/:wsId")
	ws.PATCH("/placement", h.PlaceWorkspace)
	return r, &frames
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

const base = "/v0/projects/p1/repos/r1/workspaces/branch-1/placement"

// A placement request addresses the workspace by the :wsId path param alone
// — never a body field, and never resolved through any owning chat — and
// forwards parentId/order straight through as PlaceInput.
func TestPlaceWorkspace_ForwardsWorkspaceIDAndBody(t *testing.T) {
	placer := &fakePlacer{placed: domain.Chat{ID: "branch-1", ParentID: "folder-a", Order: 2}}
	r := newRouter(placer)

	rec := do(r, http.MethodPatch, base, map[string]any{"parentId": "folder-a", "order": 2})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "branch-1", placer.wsID)
	require.NotNil(t, placer.in.ParentID)
	assert.Equal(t, "folder-a", *placer.in.ParentID)
	require.NotNil(t, placer.in.Order)
	assert.Equal(t, 2, *placer.in.Order)

	// The uniform v0 envelope (libs.Envelope): the actual payload rides under
	// "data", not at the response's top level.
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

// A field the caller omits is left nil, so a reorder-only or a reparent-only
// request are the same endpoint — the identical partial-update contract
// placeChatRequest already carries.
func TestPlaceWorkspace_OmittedFieldsStayNil(t *testing.T) {
	placer := &fakePlacer{}
	r := newRouter(placer)

	rec := do(r, http.MethodPatch, base, map[string]any{"order": 0})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Nil(t, placer.in.ParentID)
	require.NotNil(t, placer.in.Order)
	assert.Equal(t, 0, *placer.in.Order)
}

// A malformed body never reaches the usecase.
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

// A PlaceWorkspace ErrNotFound refusal (a workspaceID with no real repo
// scope — see its own doc) maps to 404, the same status libs.StatusAndMessage
// already gives the identical refusal on the chat placement route.
func TestPlaceWorkspace_NotFoundMapsTo404(t *testing.T) {
	placer := &fakePlacer{err: apperr.ErrNotFound}
	r := newRouter(placer)

	rec := do(r, http.MethodPatch, base, map[string]any{"order": 0})

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestRegression_PlaceWorkspace_AnnouncesTheMovedBranch pins the exact live
// bug: PlaceWorkspace's write rides Node (placeWorkspaceResponse's own doc),
// which carries no aggregate-command hub projection, and this handler used to
// call no broadcast at all — not even for its own subject. A locked branch
// dragged past a sibling PATCHed 200 and never moved on a live client's
// screen despite the write succeeding.
func TestRegression_PlaceWorkspace_AnnouncesTheMovedBranch(t *testing.T) {
	placer := &fakePlacer{placed: domain.Chat{ID: "branch-1", ParentID: "", Order: 1}}
	r, frames := newRouterWithFrames(placer)

	rec := do(r, http.MethodPatch, base, map[string]any{"order": 1})

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, *frames, 1, "the moved branch must be announced even with no folder siblings shifted")
	assert.Equal(t, folderFrame{folderID: "branch-1", workspaceID: "branch-1", kind: "placement_set"}, (*frames)[0])
}

// TestRegression_PlaceWorkspace_AnnouncesShiftedFolderSiblingsToo proves the
// densify's collateral folder rows are announced alongside the branch that
// actually moved, mirroring PlaceChat's identical announceFolders call.
func TestRegression_PlaceWorkspace_AnnouncesShiftedFolderSiblingsToo(t *testing.T) {
	placer := &fakePlacer{
		placed:  domain.Chat{ID: "branch-1", Order: 0},
		shifted: []domain.Chat{{ID: "f0", Type: domain.ChatTypeFolder, Order: 1}},
	}
	r, frames := newRouterWithFrames(placer)

	rec := do(r, http.MethodPatch, base, map[string]any{"order": 0})

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, *frames, 2)
	assert.Equal(t, folderFrame{folderID: "f0", workspaceID: "branch-1", kind: "folder_updated"}, (*frames)[0])
	assert.Equal(t, folderFrame{folderID: "branch-1", workspaceID: "branch-1", kind: "placement_set"}, (*frames)[1])
}
