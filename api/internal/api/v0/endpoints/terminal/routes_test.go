package terminal_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubEngine struct{}

func (stubEngine) Create(
	_ context.Context,
	_ string,
	_ string,
	_ *domain.TerminalProfile,
) (string, error) {
	return "sess1", nil
}

func (stubEngine) Kill(
	_ context.Context,
	_ string,
) error {
	return nil
}

func (stubEngine) SessionExists(
	_ context.Context,
	_ string,
) bool {
	return false
}

func (stubEngine) Attach(
	_ context.Context,
	_ string,
	_ engineterminal.WSConn,
) error {
	return nil
}

type stubProfiles struct{}

func (stubProfiles) FindAll(
	_ context.Context,
) ([]domain.TerminalProfile, error) {
	return nil, nil
}

func (stubProfiles) FindByKey(
	_ context.Context,
	id string,
) (*domain.TerminalProfile, error) {
	return &domain.TerminalProfile{ID: id}, nil
}

func (stubProfiles) Save(
	_ context.Context,
	_ domain.TerminalProfile,
) error {
	return nil
}

func (stubProfiles) Delete(
	_ context.Context,
	_ string,
) error {
	return nil
}

type stubReader struct{}

func (stubReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	terminal.Register(r.Group("/v0"), stubEngine{}, stubProfiles{}, stubReader{})

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v0/workspaces/ws1/terminals"},
		{http.MethodDelete, "/v0/terminals/sess1"},
		{http.MethodGet, "/v0/settings/terminal/profiles"},
		{http.MethodGet, "/v0/settings/terminal/profiles/p1"},
		{http.MethodPost, "/v0/settings/terminal/profiles"},
		{http.MethodPut, "/v0/settings/terminal/profiles/p1"},
		{http.MethodDelete, "/v0/settings/terminal/profiles/p1"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
