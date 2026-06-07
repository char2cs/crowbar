package ws_test

import (
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	ws "github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

type lifecycleRecorder struct {
	mu          sync.Mutex
	subscribe   []string
	unsubscribe chan string
}

func newLifecycleRecorder() *lifecycleRecorder {
	return &lifecycleRecorder{unsubscribe: make(chan string, 4)}
}

func (r *lifecycleRecorder) onSubscribe(scope string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscribe = append(r.subscribe, scope)
}

func (r *lifecycleRecorder) onUnsubscribe(scope string) {
	r.unsubscribe <- scope
}

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

	rec.mu.Lock()
	assert.Equal(t, []string{"w7"}, rec.subscribe)
	rec.mu.Unlock()

	_ = conn.Close()
	assert.Equal(t, "w7", <-rec.unsubscribe)
}

func TestBroadcaster_LifecycleHooks_ScopeFromPath(t *testing.T) {
	rec := newLifecycleRecorder()
	b, srv := setup(t, lifecycleDef(rec))
	_ = dial(t, srv, "/items/wpath")
	b.WaitRegistered()

	rec.mu.Lock()
	defer rec.mu.Unlock()
	assert.Equal(t, []string{"wpath"}, rec.subscribe)
}

func TestBroadcaster_NoHooks_NoRegression(t *testing.T) {
	b, srv := setup(t, itemDef())
	conn := dial(t, srv, "/items")
	b.WaitRegistered()
	b.Push(item{Name: "alpha", Kind: "fruit"})

	var got item
	read(t, conn, &got)
	assert.Equal(t, "alpha", got.Name)
}
