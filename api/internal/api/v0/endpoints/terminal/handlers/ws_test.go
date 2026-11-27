package handlers_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
)

const wsSessionPath = chatPath + "/sess1/ws"

type attachSpyEngine struct {
	stubEngine
	attached chan string
}

func newAttachSpyEngine() *attachSpyEngine {
	return &attachSpyEngine{attached: make(chan string, 1)}
}

func (e *attachSpyEngine) Attach(
	_ context.Context,
	sessionID string,
	_ engineterminal.WSConn,
) error {
	e.attached <- sessionID
	return nil
}

type missingSessionEngine struct {
	stubEngine
}

func (missingSessionEngine) SessionExists(
	_ context.Context,
	_ string,
) bool {
	return false
}

func TestWS_UpgradesAndAttaches(t *testing.T) {
	r := gin.New()
	spy := newAttachSpyEngine()
	h := handlers.New(spy, stubProfiles{})
	mountSessions(r, h)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + wsSessionPath
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	sessionID := <-spy.attached
	assert.Equal(t, "sess1", sessionID)
}

func TestWS_404OnUnknownSession(t *testing.T) {
	r := gin.New()
	h := handlers.New(missingSessionEngine{}, stubProfiles{})
	mountSessions(r, h)

	rec := doTerminal(r, http.MethodGet, chatPath+"/ghost/ws", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestWS_NilEngine(t *testing.T) {
	r := newNilEngineRouter()

	rec := doTerminal(r, http.MethodGet, wsSessionPath, nil)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestWS_RejectsCrossOriginUpgrade(t *testing.T) {
	r := gin.New()
	spy := newAttachSpyEngine()
	h := handlers.New(spy, stubProfiles{})
	mountSessions(r, h)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + wsSessionPath
	header := http.Header{"Origin": {"http://evil.example.com"}}
	conn, resp, err := websocket.DefaultDialer.Dial(url, header)
	if conn != nil {
		_ = conn.Close()
	}

	require.Error(t, err, "a disallowed cross-origin request must not be upgraded")
	require.NotNil(t, resp)
	defer func() { _ = resp.Body.Close() }()
	assert.NotEqual(t, http.StatusSwitchingProtocols, resp.StatusCode)
	select {
	case sessionID := <-spy.attached:
		t.Fatalf("the engine must never see a connection that failed to upgrade, got Attach(%q)", sessionID)
	default:
	}
}

type failingAttachEngine struct{ stubEngine }

func (failingAttachEngine) Attach(context.Context, string, engineterminal.WSConn) error {
	return errors.New("restore failed")
}

type recordsChan chan slog.Record

func (c recordsChan) Enabled(context.Context, slog.Level) bool { return true }
func (c recordsChan) Handle(_ context.Context, r slog.Record) error {
	c <- r
	return nil
}
func (c recordsChan) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c recordsChan) WithGroup(string) slog.Handler      { return c }

func TestWS_AFailedAttachIsLoggedWithItsSession(t *testing.T) {
	records := make(recordsChan, 16)
	prev := slog.Default()
	slog.SetDefault(slog.New(records))
	t.Cleanup(func() { slog.SetDefault(prev) })
	r := gin.New()
	mountSessions(r, handlers.New(failingAttachEngine{}, stubProfiles{}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+srv.URL[len("http"):]+wsSessionPath, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	var rec slog.Record
	select {
	case rec = <-records:
	case <-time.After(5 * time.Second):
		t.Fatal("a failed attach logged nothing")
	}
	assert.Equal(t, slog.LevelWarn, rec.Level)
	attrs := map[string]string{}
	rec.Attrs(func(a slog.Attr) bool { attrs[a.Key] = a.Value.String(); return true })
	assert.Equal(t, "sess1", attrs["session_id"])
	assert.Contains(t, attrs["err"], "restore failed")
}
