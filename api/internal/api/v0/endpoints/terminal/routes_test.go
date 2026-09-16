package terminal_test

import (
	"context"
	"image/color"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
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
	// True so the PTY /ws route's handler proceeds past the existence guard;
	// the non-Upgrade request then 400s, proving the route is mounted (not a
	// router 404).
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

// SetHostTheme is a no-op here: the host theme's behaviour is covered where it has an
// observable effect (a session's OSC 11 answer at birth, in the terminal engine's own
// tests) and at the endpoint, by the recording engine in hosttheme_test.go.
func (stubEngine) SetHostTheme(_, _ color.Color) {}

func (stubEngine) StateOf(_ string) (string, bool) {
	return "active", true
}

type stubBroadcaster struct{}

func (stubBroadcaster) Push(
	_ dto.TerminalSessionDTO,
) {
}

// passthroughDispatch mirrors ws.DualServe's signature for the route-mount test:
// it returns the REST handler so a plain GET is served (the WS branch is
// exercised by the container integration tests).
func passthroughDispatch(
	rest gin.HandlerFunc,
	_ gin.HandlerFunc,
) gin.HandlerFunc {
	return rest
}

func noopWSHandle(
	_ *gin.Context,
) {
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

// stubWorktreeScope stands in for v0's resolveChatWorktree middleware: the
// chat-scoped group resolves the chat to a workspace and stashes it before any
// terminal handler runs, so the mount test exercises the same wiring the router
// builds.
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
	// Session lifecycle + PTY routes mount on the flat chat-scoped group; the
	// profile CRUD mounts on the top-level /v0 group (mirroring the router).
	chatScoped := rg.Group("/chats/:chatId")
	chatScoped.Use(stubWorktreeScope)
	terminal.Register(
		chatScoped,
		rg,
		stubEngine{},
		stubProfiles{},
		stubBroadcaster{},
		noopWSHandle,
		passthroughDispatch,
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
