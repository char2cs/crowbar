package ws_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

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
