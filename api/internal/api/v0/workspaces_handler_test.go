package v0_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/fixtures"
)

func newTestStore() *fixtures.Store {
	s := fixtures.NewStore()
	s.Flows = []fixtures.FlowDefinition{
		{
			Name: "feature-development",
			States: []fixtures.FlowStateDefinition{
				{Name: "brainstorming", Label: "Brainstorm", UI: "chat"},
			},
		},
	}
	s.AddWorkspace(fixtures.WorkspacePayload{
		ID: "ws1", RepoID: "crowbar", Branch: "main",
		FlowName: "feature-development", CurrentState: "brainstorming",
		Flow: s.Flows[0],
	})
	return s
}

func TestWorkspacesHandler_Get(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore()
	r := gin.New()
	h := v0.NewWorkspacesHandler(store)
	r.GET("/workspaces/:id", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/workspaces/ws1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp fixtures.WorkspacePayload
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.ID != "ws1" {
		t.Fatalf("expected id ws1, got %s", resp.ID)
	}
}

func TestWorkspacesHandler_Get_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := fixtures.NewStore()
	r := gin.New()
	h := v0.NewWorkspacesHandler(store)
	r.GET("/workspaces/:id", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/workspaces/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestWorkspacesHandler_Create(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore()
	r := gin.New()
	h := v0.NewWorkspacesHandler(store)
	r.POST("/workspaces", h.Create)

	body := `{"repoId":"crowbar","branch":"feature/new","flowName":"feature-development"}`
	req := httptest.NewRequest(http.MethodPost, "/workspaces", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d — body: %s", w.Code, w.Body.String())
	}
	var resp fixtures.WorkspacePayload
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ID == "" {
		t.Fatal("expected non-empty id")
	}
	if resp.Branch != "feature/new" {
		t.Fatalf("expected branch feature/new, got %s", resp.Branch)
	}
}
