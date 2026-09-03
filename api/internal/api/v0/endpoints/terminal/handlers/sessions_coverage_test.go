package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// createErrEngine fails Create, exercising CreateSession's own error branch
// (distinct from the workspace-lookup 404 errReader already covers).
type createErrEngine struct {
	stubEngine
}

func (createErrEngine) Create(
	_ context.Context,
	_ string,
	_ string,
	_ *domain.TerminalProfile,
) (string, error) {
	return "", errors.New("spawn failed")
}

// killErrEngine fails Kill with a generic error, distinct from
// engineterminal.ErrSessionNotFound (already covered by notFoundEngine).
type killErrEngine struct {
	stubEngine
}

func (killErrEngine) Kill(
	_ context.Context,
	_ string,
) error {
	return errors.New("pty wedged")
}

// vanishedStateEngine reports one session from ListSessionsForWorkspace whose
// StateOf then reports it gone — the race ListSessions' own comment describes
// ("session vanished between List and StateOf").
type vanishedStateEngine struct {
	stubEngine
}

func (vanishedStateEngine) ListSessionsForWorkspace(
	_ string,
) []string {
	return []string{"sess1", "sess2"}
}

func (vanishedStateEngine) StateOf(
	id string,
) (string, bool) {
	if id == "sess1" {
		return "active", true
	}
	return "", false
}

func TestCreateSession_400OnMalformedBody(t *testing.T) {
	r := gin.New()
	mountSessions(r, newHandlers(&spyBroadcaster{}))

	rec := doRaw(r, http.MethodPost, wsPath, []byte("{not json"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateSession_500WhenTheEngineFailsToSpawn(t *testing.T) {
	r := gin.New()
	spy := &spyBroadcaster{}
	h := handlers.New(createErrEngine{}, stubProfiles{}, stubReader{}, spy)
	mountSessions(r, h)

	rec := do(r, http.MethodPost, wsPath, nil)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Empty(t, spy.pushed, "a failed spawn must not broadcast an active session")
}

func TestKillSession_500OnAGenericEngineError(t *testing.T) {
	r := gin.New()
	spy := &spyBroadcaster{}
	h := handlers.New(killErrEngine{}, stubProfiles{}, stubReader{}, spy)
	mountSessions(r, h)

	rec := do(r, http.MethodDelete, wsPath+"/sess1", nil)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Empty(t, spy.pushed, "a kill that never actually succeeded must not broadcast 'ended'")
}

func TestListSessions_SkipsASessionThatVanishedBetweenListAndStateOf(t *testing.T) {
	r := gin.New()
	h := handlers.New(vanishedStateEngine{}, stubProfiles{}, stubReader{}, &spyBroadcaster{})
	mountSessions(r, h)

	rec := do(r, http.MethodGet, wsPath, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "sess1")
	assert.NotContains(t, rec.Body.String(), "sess2",
		"a session reported by List but gone by the time StateOf runs must be skipped, not raced")
}

// TestCreateSession_UnresolvableProfileIsIgnoredNotFatal proves resolveProfile
// swallows a profile-store lookup failure into "no profile" rather than
// failing the whole session create over an optional lookup.
func TestCreateSession_UnresolvableProfileIsIgnoredNotFatal(t *testing.T) {
	r := gin.New()
	h := handlers.New(stubEngine{}, errProfiles{}, stubReader{}, &spyBroadcaster{})
	mountSessions(r, h)

	rec := do(r, http.MethodPost, wsPath, map[string]any{"profileId": "p1"})

	assert.Equal(t, http.StatusCreated, rec.Code,
		"an unresolvable requested profile must not block session creation")
}

// TestPushSession_NilBroadcaster_NoPanic proves pushSession degrades cleanly
// when no broadcaster is wired (mirrors the production wiring for a daemon
// mode with no WS fan-out), rather than requiring every caller to nil-check.
func TestPushSession_NilBroadcaster_NoPanic(t *testing.T) {
	r := gin.New()
	h := handlers.New(stubEngine{}, stubProfiles{}, stubReader{}, nil)
	mountSessions(r, h)

	rec := do(r, http.MethodPost, wsPath, nil)
	assert.Equal(t, http.StatusCreated, rec.Code)

	rec = do(r, http.MethodDelete, wsPath+"/sess1", nil)
	assert.Equal(t, http.StatusAccepted, rec.Code)
}
