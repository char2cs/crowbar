package ws_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ws "github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

// Both hooks record via buffered channels so tests block on the hook actually
// firing (a real signal) instead of racing the async OnSubscribe against a slice
// read — that read-before-fire race passed plain -race but failed under the
// slower -race -coverpkg instrumentation.
type lifecycleRecorder struct {
	subscribe   chan string
	unsubscribe chan string
}

func newLifecycleRecorder() *lifecycleRecorder {
	return &lifecycleRecorder{
		subscribe:   make(chan string, 4),
		unsubscribe: make(chan string, 4),
	}
}

func (r *lifecycleRecorder) onSubscribe(scope string)   { r.subscribe <- scope }
func (r *lifecycleRecorder) onUnsubscribe(scope string) { r.unsubscribe <- scope }

func lifecycleDef(
	rec *lifecycleRecorder,
) ws.StreamDef[item] {
	def := itemDef()
	def.ScopeKey = func(c *gin.Context) string {
		if id := c.Param("ns"); id != "" {
			return id
		}
		return c.Query("wsId")
	}
	def.OnSubscribe = rec.onSubscribe
	def.OnUnsubscribe = rec.onUnsubscribe
	return def
}

func TestBroadcaster_LifecycleHooks_FireOnSubAndUnsub(t *testing.T) {
	rec := newLifecycleRecorder()
	b, srv := setup(t, lifecycleDef(rec))
	conn := dial(t, srv, "/items?wsId=w7")
	b.WaitRegistered()

	// Block on the OnSubscribe hook actually firing, not a racy slice read.
	assert.Equal(t, "w7", <-rec.subscribe)

	_ = conn.Close()
	assert.Equal(t, "w7", <-rec.unsubscribe)
}

func TestBroadcaster_LifecycleHooks_ScopeFromPath(t *testing.T) {
	rec := newLifecycleRecorder()
	b, srv := setup(t, lifecycleDef(rec))
	_ = dial(t, srv, "/items/wpath")
	b.WaitRegistered()

	// Block on the OnSubscribe hook actually firing, not a racy slice read.
	assert.Equal(t, "wpath", <-rec.subscribe)
}

func TestBroadcaster_NoHooks_NoRegression(t *testing.T) {
	b, srv := setup(t, itemDef())
	conn := dial(t, srv, "/items")
	b.WaitRegistered()
	b.Push(item{Name: "alpha", Kind: "fruit"})

	assert.Equal(t, "alpha", readItem(t, conn).Name)
}

// hookBound fails a test whose hook never fires instead of stalling it.
const hookBound = 10 * time.Second

// hooked is the next scope a hook recorded, failing the test past the bound.
func hooked(
	t *testing.T,
	ch <-chan string,
) string {
	t.Helper()
	select {
	case scope := <-ch:
		return scope
	case <-time.After(hookBound):
		require.FailNow(t, "the hook never fired")
		return ""
	}
}

// admitDef holds each client's scope through Admit, refusing the scope "refused".
func admitDef(
	rec *lifecycleRecorder,
) ws.StreamDef[item] {
	def := lifecycleDef(rec)
	def.OnSubscribe = nil
	def.Admit = func(scope string) error {
		rec.subscribe <- scope
		switch scope {
		case "refused":
			return errors.New("no watch")
		case "full":
			return &ws.AdmitRefusal{Status: http.StatusInsufficientStorage, Err: errors.New("no watches left")}
		}
		return nil
	}
	return def
}

func TestBroadcaster_Admit_HoldsTheScopeBeforeTheUpgradeAndReleasesItOnLeave(t *testing.T) {
	rec := newLifecycleRecorder()
	_, srv := setup(t, admitDef(rec))

	conn := dial(t, srv, "/items?wsId=w7")

	select {
	case scope := <-rec.subscribe:
		assert.Equal(t, "w7", scope)
	default:
		assert.Fail(t, "the upgrade answered before Admit held the scope")
	}
	_ = conn.Close()
	assert.Equal(t, "w7", hooked(t, rec.unsubscribe))
}

func TestBroadcaster_Admit_ErrorRefusesTheUpgradeAndHoldsNothing(t *testing.T) {
	rec := newLifecycleRecorder()
	_, srv := setup(t, admitDef(rec))

	_, resp, err := websocket.DefaultDialer.Dial("ws"+srv.URL[len("http"):]+"/items?wsId=refused", nil)

	require.ErrorIs(t, err, websocket.ErrBadHandshake)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, "refused", hooked(t, rec.subscribe))
	_ = dial(t, srv, "/items?wsId=w8").Close()
	assert.Equal(t, "w8", hooked(t, rec.subscribe))
	assert.Equal(t, "w8", hooked(t, rec.unsubscribe), "the refused client released nothing")
}

func TestBroadcaster_Admit_RefusalNamesItsStatus(t *testing.T) {
	rec := newLifecycleRecorder()
	_, srv := setup(t, admitDef(rec))

	_, resp, err := websocket.DefaultDialer.Dial("ws"+srv.URL[len("http"):]+"/items?wsId=full", nil)

	require.ErrorIs(t, err, websocket.ErrBadHandshake)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusInsufficientStorage, resp.StatusCode)
}
