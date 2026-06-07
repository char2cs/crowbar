package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	terminalhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var errStore = errors.New("store boom")

// stubProfiles is an in-memory ProfileStore for the REST handler tests. Setting
// failAll makes every method return errStore so the 500 paths are exercised.
type stubProfiles struct {
	items   map[string]domain.TerminalProfile
	failAll bool
}

func newStubProfiles() *stubProfiles {
	return &stubProfiles{items: map[string]domain.TerminalProfile{}}
}

func (s *stubProfiles) Save(
	_ context.Context,
	item domain.TerminalProfile,
) error {
	if s.failAll {
		return errStore
	}
	s.items[item.ID] = item
	return nil
}

func (s *stubProfiles) Delete(
	_ context.Context,
	id string,
) error {
	if s.failAll {
		return errStore
	}
	delete(s.items, id)
	return nil
}

func (s *stubProfiles) FindByKey(
	_ context.Context,
	id string,
) (*domain.TerminalProfile, error) {
	if s.failAll {
		return nil, errStore
	}
	item, ok := s.items[id]
	if !ok {
		return nil, nil
	}
	return &item, nil
}

func (s *stubProfiles) FindAll(
	_ context.Context,
) ([]domain.TerminalProfile, error) {
	if s.failAll {
		return nil, errStore
	}
	out := make([]domain.TerminalProfile, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	return out, nil
}

// stubWorkspace is a WorkspaceReader returning a fixed worktree path, or an error
// when fail is set so the error path is exercised. By default fail returns
// apperr.ErrNotFound (the 404 path); set err to override with a generic error
// (the 500 path).
type stubWorkspace struct {
	path string
	fail bool
	err  error
}

func (s stubWorkspace) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	if s.err != nil {
		return domain.Workspace{}, s.err
	}
	if s.fail {
		return domain.Workspace{}, apperr.ErrNotFound
	}
	return domain.Workspace{WorktreePath: s.path}, nil
}

// stubEngine is a TerminalEngine returning canned create/kill results.
type stubEngine struct {
	sid       string
	createErr error
	killErr   error
}

func (s stubEngine) Create(
	_ context.Context,
	_ string,
	_ string,
	_ *domain.TerminalProfile,
) (string, error) {
	return s.sid, s.createErr
}

func (s stubEngine) Kill(
	_ context.Context,
	_ string,
) error {
	return s.killErr
}

// saveFailStore wraps stubProfiles but always fails Save, exercising the
// update-save error path where the prior lookup succeeded.
type saveFailStore struct {
	*stubProfiles
}

func (s *saveFailStore) Save(
	_ context.Context,
	_ domain.TerminalProfile,
) error {
	return errStore
}

// deleteFailStore wraps stubProfiles but always fails Delete, exercising the
// delete error path where the prior lookup succeeded.
type deleteFailStore struct {
	*stubProfiles
}

func (s *deleteFailStore) Delete(
	_ context.Context,
	_ string,
) error {
	return errStore
}

type envelope struct {
	Success bool            `json:"success"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
}

func decodeEnvelope(
	t *testing.T,
	body *bytes.Buffer,
) envelope {
	t.Helper()
	var env envelope
	require.NoError(t, json.Unmarshal(body.Bytes(), &env))
	return env
}

func newHandlers(
	eng terminalhandlers.TerminalEngine,
	profiles terminalhandlers.ProfileStore,
	ws terminalhandlers.WorkspaceReader,
) *terminalhandlers.Handlers {
	return terminalhandlers.New(eng, profiles, ws)
}

func doReq(
	t *testing.T,
	h gin.HandlerFunc,
	method string,
	target string,
	params gin.Params,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	_, engine := gin.CreateTestContext(w)
	engine.Handle(method, routePattern(target, params), h)
	var reader *bytes.Buffer
	if body == "" {
		reader = &bytes.Buffer{}
	} else {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)
	return w
}

// routePattern rebuilds the gin route pattern for target by replacing each
// concrete param value with its :name placeholder, so the bare handler runs
// through a real engine that flushes 204 status writes correctly.
func routePattern(
	target string,
	params gin.Params,
) string {
	pattern := target
	for _, p := range params {
		pattern = strings.Replace(pattern, "/"+p.Value, "/:"+p.Key, 1)
	}
	return pattern
}

func TestCreateSession_NoEngine503(t *testing.T) {
	h := newHandlers(nil, newStubProfiles(), stubWorkspace{path: "/tmp"})
	w := doReq(t, h.CreateSession, http.MethodPost, "/v0/workspaces/w1/terminals", gin.Params{{Key: "wsId", Value: "w1"}}, "{}")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	env := decodeEnvelope(t, w.Body)
	assert.False(t, env.Success)
	assert.NotEmpty(t, env.Error)
}

func TestKillSession_NoEngine503(t *testing.T) {
	h := newHandlers(nil, newStubProfiles(), stubWorkspace{})
	w := doReq(t, h.KillSession, http.MethodDelete, "/v0/terminals/s1", gin.Params{{Key: "sessionId", Value: "s1"}}, "")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
