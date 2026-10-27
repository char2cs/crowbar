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

// wsSessionPath is chatPath's per-session WebSocket upgrade path.
const wsSessionPath = chatPath + "/sess1/ws"

// attachSpyEngine embeds stubEngine but signals every Attach call on a
// channel, giving a test a deterministic barrier for "the handler reached the
// terminal engine" instead of a sleep or a poll — the WS upgrade handshake
// completing on the client is not the same event as the server's handler
// goroutine reaching its own Attach call, so this is the only race-free way to
// observe it.
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

// missingSessionEngine reports every session as absent, exercising WS's 404
// guard before any upgrade is attempted.
type missingSessionEngine struct {
	stubEngine
}

func (missingSessionEngine) SessionExists(
	_ context.Context,
	_ string,
) bool {
	return false
}

// TestWS_UpgradesAndAttaches proves the happy path end to end: a session that
// exists gets a successful WebSocket upgrade, and the handler hands the
// connection to the terminal engine's Attach rather than doing anything with
// it itself.
func TestWS_UpgradesAndAttaches(t *testing.T) {
	r := gin.New()
	spy := newAttachSpyEngine()
	h := handlers.New(spy, stubProfiles{}, &spyBroadcaster{})
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

	// Attach's channel send is the genuine completion signal for "the server
	// reached the terminal engine", distinct from the client merely having
	// received the 101 response — a real barrier, not a guessed wait.
	sessionID := <-spy.attached
	assert.Equal(t, "sess1", sessionID)
}

// TestWS_404OnUnknownSession proves the handler checks session existence
// BEFORE attempting the WebSocket upgrade, so a stale/unknown session id gets
// a plain 404 rather than an upgraded socket that immediately has nothing to
// attach to.
func TestWS_404OnUnknownSession(t *testing.T) {
	r := gin.New()
	h := handlers.New(missingSessionEngine{}, stubProfiles{}, &spyBroadcaster{})
	mountSessions(r, h)

	rec := doTerminal(r, http.MethodGet, chatPath+"/ghost/ws", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestWS_NilEngine proves WS shares requireTerminalEngine's 503 guard with
// every other handler in this package.
func TestWS_NilEngine(t *testing.T) {
	r := newNilEngineRouter()

	rec := doTerminal(r, http.MethodGet, wsSessionPath, nil)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// TestWS_RejectsCrossOriginUpgrade proves the terminal socket's Origin check is
// actually wired: a disallowed cross-origin request must never be upgraded, or
// any website the user has open could open a live PTY through their browser.
func TestWS_RejectsCrossOriginUpgrade(t *testing.T) {
	r := gin.New()
	spy := newAttachSpyEngine()
	h := handlers.New(spy, stubProfiles{}, &spyBroadcaster{})
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
	// Attach is only reachable past a successful upgrade, and the response the
	// dial just read was written by the same handler goroutine that would have
	// made the call — so an empty channel here is a settled fact, not a race.
	select {
	case sessionID := <-spy.attached:
		t.Fatalf("the engine must never see a connection that failed to upgrade, got Attach(%q)", sessionID)
	default:
	}
}

// failingAttachEngine refuses every attach, as a session whose restore fails does.
type failingAttachEngine struct{ stubEngine }

func (failingAttachEngine) Attach(context.Context, string, engineterminal.WSConn) error {
	return errors.New("restore failed")
}

// recordsChan is a slog handler that hands each record to a test.
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
	mountSessions(r, handlers.New(failingAttachEngine{}, stubProfiles{}, &spyBroadcaster{}))
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
