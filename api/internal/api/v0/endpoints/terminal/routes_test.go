package terminal_test

import (
	"context"
	"image/color"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
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
	return true
}

func (stubEngine) Attach(
	_ context.Context,
	_ string,
	_ engineterminal.WSConn,
) error {
	return nil
}

func (stubEngine) ListSessionsForChat(
	_ string,
) []string {
	return nil
}

func (stubEngine) SetHostTheme(_, _ color.Color) {}

func (stubEngine) HostTheme() (bg, fg color.Color) { return nil, nil }

func (stubEngine) StateOf(_ string) (string, bool) {
	return "active", true
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

func stubWorktreeScope(
	c *gin.Context,
) {
	reqscope.SetWorkspace(c, domain.Workspace{ID: "ws1"})
	c.Next()
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	rg := r.Group("/v0")
	chatScoped := rg.Group("/chats/:chatId")
	chatScoped.Use(stubWorktreeScope)
	terminal.Register(
		chatScoped,
		rg,
		stubEngine{},
		stubProfiles{},
	)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v0/chats/chat1/terminals"},
		{http.MethodPost, "/v0/chats/chat1/terminals"},
		{http.MethodDelete, "/v0/chats/chat1/terminals/sess1"},
		{http.MethodGet, "/v0/chats/chat1/terminals/sess1/ws"},
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
