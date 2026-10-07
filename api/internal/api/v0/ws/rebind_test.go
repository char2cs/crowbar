package ws_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ws "github.com/char2cs/crowbar/api/internal/api/v0/ws"
)

type wsEvent struct {
	WsID string `json:"wsId"`
	Seq  int    `json:"seq"`
}

// placements is the chat -> workspace table a bound filter resolves against.
// Every Resolve call is recorded on calls; a non-nil gate holds the call open
// AFTER it has read the table, so a test can mutate the table under it.
type placements struct {
	mu    sync.Mutex
	chats map[string]string
	gate  chan struct{}
	calls chan []string
	// down makes resolve answer nil, the way a failed store read does.
	down bool
}

func newPlacements(chats map[string]string) *placements {
	return &placements{chats: chats, calls: make(chan []string, 64)}
}

func (p *placements) move(chatID, wsID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.chats[chatID] = wsID
}

func (p *placements) fail(down bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.down = down
}

func (p *placements) hold(gate chan struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gate = gate
}

func (p *placements) lookup(chatID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.chats[chatID]
}

func (p *placements) resolve(chatIDs []string) map[string]string {
	p.mu.Lock()
	out := make(map[string]string, len(chatIDs))
	for _, id := range chatIDs {
		out[id] = p.chats[id]
	}
	if p.down {
		out = nil
	}
	gate := p.gate
	p.mu.Unlock()
	p.calls <- slices.Clone(chatIDs)
	if gate != nil {
		<-gate
	}
	return out
}

func boundDef(p *placements) ws.StreamDef[wsEvent] {
	return ws.StreamDef[wsEvent]{
		Namespace:     func(e wsEvent) string { return e.WsID },
		Serialize:     func(e wsEvent) ([]byte, error) { return json.Marshal(e) },
		FlatNamespace: true,
		Filters: []ws.FilterDef[wsEvent]{{
			Param:    "chatId",
			Extract:  func(e wsEvent) string { return e.WsID },
			Match:    ws.ExactMatch,
			Required: true,
			Resolve:  p.resolve,
		}},
	}
}

func boundSetup(
	t *testing.T,
	def ws.StreamDef[wsEvent],
) (*ws.Broadcaster[wsEvent], *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	b := ws.NewBroadcaster(def)
	t.Cleanup(b.Close)
	r := gin.New()
	r.GET("/chats/:chatId/ws", b.Handle)
	r.GET("/files/ws", b.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return b, srv
}

func readEvent(
	t *testing.T,
	conn *websocket.Conn,
) wsEvent {
	t.Helper()
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var got wsEvent
	require.NoError(t, json.Unmarshal(msg, &got))
	return got
}

func TestBroadcaster_BoundClientFollowsItsChatAcrossRebind(t *testing.T) {
	p := newPlacements(map[string]string{"chat-1": "ws-a"})
	b, srv := boundSetup(t, boundDef(p))
	conn := dial(t, srv, "/chats/chat-1/ws")

	b.Push(wsEvent{WsID: "ws-b", Seq: 1})
	b.Push(wsEvent{WsID: "ws-a", Seq: 2})
	assert.Equal(t, wsEvent{WsID: "ws-a", Seq: 2}, readEvent(t, conn))

	p.move("chat-1", "ws-b")
	assert.Equal(t, 1, b.Rebind())

	b.Push(wsEvent{WsID: "ws-a", Seq: 3})
	b.Push(wsEvent{WsID: "ws-b", Seq: 4})
	assert.Equal(t, wsEvent{WsID: "ws-b", Seq: 4}, readEvent(t, conn))
}

// TestBroadcaster_AFailedResolveKeepsTheLastBinding: a chat the resolver could
// not answer keeps streaming its current workspace instead of going silent
// until the next placement change.
func TestBroadcaster_AFailedResolveKeepsTheLastBinding(t *testing.T) {
	p := newPlacements(map[string]string{"chat-1": "ws-a"})
	b, srv := boundSetup(t, boundDef(p))
	conn := dial(t, srv, "/chats/chat-1/ws")

	p.move("chat-1", "ws-b")
	p.fail(true)
	assert.Equal(t, 0, b.Rebind())
	b.Push(wsEvent{WsID: "ws-a", Seq: 1})
	assert.Equal(t, wsEvent{WsID: "ws-a", Seq: 1}, readEvent(t, conn))

	p.fail(false)
	assert.Equal(t, 1, b.Rebind())
	b.Push(wsEvent{WsID: "ws-a", Seq: 2})
	b.Push(wsEvent{WsID: "ws-b", Seq: 3})
	assert.Equal(t, wsEvent{WsID: "ws-b", Seq: 3}, readEvent(t, conn))
}

// TestBroadcaster_UnplacedChatReceivesNothing: a chat resolving to no workspace
// matches nothing, not even an event that itself carries no workspace.
func TestBroadcaster_UnplacedChatReceivesNothing(t *testing.T) {
	p := newPlacements(map[string]string{})
	b, srv := boundSetup(t, boundDef(p))
	conn := dial(t, srv, "/chats/chat-orphan/ws")

	b.Push(wsEvent{WsID: "ws-a", Seq: 1})
	b.Push(wsEvent{WsID: "", Seq: 2})

	// Frames are FIFO per client, so the first frame read proves the two
	// above were never queued.
	p.move("chat-orphan", "ws-z")
	require.Equal(t, 1, b.Rebind())
	b.Push(wsEvent{WsID: "ws-z", Seq: 3})
	assert.Equal(t, wsEvent{WsID: "ws-z", Seq: 3}, readEvent(t, conn))
}

func TestBroadcaster_RebindResolvesEveryClientInOneCall(t *testing.T) {
	chats := map[string]string{}
	for i := range 4 {
		chats[fmt.Sprintf("chat-%d", i)] = fmt.Sprintf("ws-%d", i%2)
	}
	p := newPlacements(chats)
	b, srv := boundSetup(t, boundDef(p))
	paths := []string{"chat-0", "chat-1", "chat-2", "chat-3", "chat-0"}
	for _, chatID := range paths {
		dial(t, srv, "/chats/"+chatID+"/ws")
		assert.Equal(t, []string{chatID}, <-p.calls, "connect resolves the one chat")
	}

	assert.Equal(t, 0, b.Rebind(), "nothing moved")

	require.Len(t, p.calls, 1)
	assert.Equal(t, []string{"chat-0", "chat-1", "chat-2", "chat-3"}, <-p.calls)
}

// TestBroadcaster_RequestRebindCoalescesAndTheLastStateWins holds the worker's
// first rebind open while the chat moves ten times, each move requesting a
// rebind; exactly one more rebind follows and it sees the final placement.
func TestBroadcaster_RequestRebindCoalescesAndTheLastStateWins(t *testing.T) {
	p := newPlacements(map[string]string{"chat-1": "ws-a"})
	b, srv := boundSetup(t, boundDef(p))
	conn := dial(t, srv, "/chats/chat-1/ws")
	<-p.calls

	gate := make(chan struct{})
	p.hold(gate)
	b.RequestRebind()
	<-p.calls
	for i := range 10 {
		p.move("chat-1", fmt.Sprintf("ws-%d", i))
		b.RequestRebind()
	}
	p.move("chat-1", "ws-final")
	close(gate)
	<-p.calls

	b.Close()
	assert.Empty(t, p.calls, "ten requests during one rebind coalesce into one")
	b.Push(wsEvent{WsID: "ws-9", Seq: 1})
	b.Push(wsEvent{WsID: "ws-final", Seq: 2})
	assert.Equal(t, wsEvent{WsID: "ws-final", Seq: 2}, readEvent(t, conn))
}

// TestBroadcaster_RebindResendsTheSnapshotOncePerBindingChange: the snapshot
// stands in for a full-state stream (git status) that must replace the old
// workspace's state with the new one's when the binding moves.
func TestBroadcaster_RebindResendsTheSnapshotOncePerBindingChange(t *testing.T) {
	p := newPlacements(map[string]string{"chat-1": "ws-a"})
	snaps := make(chan string, 16)
	def := boundDef(p)
	def.Snapshot = func(scope string) []wsEvent {
		snaps <- scope
		return []wsEvent{{WsID: p.lookup(scope), Seq: -1}}
	}
	b, srv := boundSetup(t, def)
	conn := dial(t, srv, "/chats/chat-1/ws")
	assert.Equal(t, wsEvent{WsID: "ws-a", Seq: -1}, readEvent(t, conn))
	assert.Equal(t, "chat-1", <-snaps)

	p.move("chat-1", "ws-b")
	require.Equal(t, 1, b.Rebind())
	assert.Equal(t, wsEvent{WsID: "ws-b", Seq: -1}, readEvent(t, conn))

	require.Equal(t, 0, b.Rebind())
	b.Push(wsEvent{WsID: "ws-b", Seq: 5})
	assert.Equal(t, wsEvent{WsID: "ws-b", Seq: 5}, readEvent(t, conn))

	p.move("chat-1", "ws-c")
	require.Equal(t, 1, b.Rebind())
	assert.Equal(t, wsEvent{WsID: "ws-c", Seq: -1}, readEvent(t, conn))

	assert.Equal(t, []string{"chat-1", "chat-1"}, []string{<-snaps, <-snaps})
	assert.Empty(t, snaps, "no snapshot for a rebind that moved nothing")
}

// TestBroadcaster_BoundFilterIsInactiveForAClientWithoutTheParam keeps a
// non-Required bound filter out of the way of a mount that scopes by another
// filter, the way the files stream serves both wsId and chatId routes.
func TestBroadcaster_BoundFilterIsInactiveForAClientWithoutTheParam(t *testing.T) {
	p := newPlacements(map[string]string{})
	def := boundDef(p)
	def.Filters[0].Required = false
	def.Filters = append(def.Filters, ws.FilterDef[wsEvent]{
		Param: "wsId", Extract: func(e wsEvent) string { return e.WsID }, Match: ws.ExactMatch,
	})
	b, srv := boundSetup(t, def)
	conn := dial(t, srv, "/files/ws?wsId=ws-q")

	b.Push(wsEvent{WsID: "ws-a", Seq: 1})
	b.Push(wsEvent{WsID: "ws-q", Seq: 2})
	assert.Equal(t, wsEvent{WsID: "ws-q", Seq: 2}, readEvent(t, conn))
	assert.Empty(t, p.calls, "a client carrying no chat id is never resolved")
}

func TestBroadcaster_CloseWithoutBoundFiltersIsANoOp(t *testing.T) {
	b := ws.NewBroadcaster(itemDef())
	b.RequestRebind()
	b.Close()
	b.Close()
}

// scopeLedger refcounts the lifecycle hooks the way the watcher manager does,
// and records every hook call on ops and every scope that fell to zero.
type scopeLedger struct {
	mu      sync.Mutex
	refs    map[string]int
	dropped []string
	ops     chan string
}

func newScopeLedger() *scopeLedger {
	return &scopeLedger{refs: map[string]int{}, ops: make(chan string, 64)}
}

func (l *scopeLedger) acquire(scope string) {
	l.mu.Lock()
	l.refs[scope]++
	l.mu.Unlock()
	l.ops <- "+" + scope
}

func (l *scopeLedger) release(scope string) {
	l.mu.Lock()
	l.refs[scope]--
	if l.refs[scope] == 0 {
		delete(l.refs, scope)
		l.dropped = append(l.dropped, scope)
	}
	l.mu.Unlock()
	l.ops <- "-" + scope
}

func (l *scopeLedger) held() map[string]int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return maps.Clone(l.refs)
}

func (l *scopeLedger) drops() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.dropped)
}

// scopedBoundDef answers "from-request" as the request's scope, which a bound
// client must never be refcounted by: its scope is its binding.
func scopedBoundDef(p *placements, l *scopeLedger) ws.StreamDef[wsEvent] {
	def := boundDef(p)
	def.ScopeKey = func(*gin.Context) string { return "from-request" }
	def.OnSubscribe = l.acquire
	def.OnUnsubscribe = l.release
	return def
}

func TestBroadcaster_ABoundClientsScopeFollowsItsBinding(t *testing.T) {
	p := newPlacements(map[string]string{"chat-1": "ws-a", "chat-2": "ws-a"})
	l := newScopeLedger()
	b, srv := boundSetup(t, scopedBoundDef(p, l))

	mover := dial(t, srv, "/chats/chat-1/ws")
	assert.Equal(t, "+ws-a", <-l.ops)
	stayer := dial(t, srv, "/chats/chat-2/ws")
	assert.Equal(t, "+ws-a", <-l.ops)

	p.move("chat-1", "ws-b")
	require.Equal(t, 1, b.Rebind())
	assert.Equal(t, []string{"+ws-b", "-ws-a"}, []string{<-l.ops, <-l.ops},
		"the new scope is held before the old one is let go")
	assert.Equal(t, map[string]int{"ws-a": 1, "ws-b": 1}, l.held())

	require.NoError(t, mover.Close())
	assert.Equal(t, "-ws-b", <-l.ops, "a disconnect releases the CURRENT scope")
	require.NoError(t, stayer.Close())
	assert.Equal(t, "-ws-a", <-l.ops)

	assert.Empty(t, l.held(), "every acquired scope is released")
	assert.Empty(t, l.ops, "no scope is acquired or released twice")
	assert.Equal(t, []string{"ws-b", "ws-a"}, l.drops())
}

// TestBroadcaster_SwappedBindingsNeverDropAHeldScope: two clients trade
// workspaces in one Rebind; both stay subscribed throughout, so neither
// scope's refcount may touch zero (that would stop its watcher).
func TestBroadcaster_SwappedBindingsNeverDropAHeldScope(t *testing.T) {
	p := newPlacements(map[string]string{"chat-1": "ws-a", "chat-2": "ws-b"})
	l := newScopeLedger()
	b, srv := boundSetup(t, scopedBoundDef(p, l))
	dial(t, srv, "/chats/chat-1/ws")
	dial(t, srv, "/chats/chat-2/ws")
	<-l.ops
	<-l.ops

	p.move("chat-1", "ws-b")
	p.move("chat-2", "ws-a")
	require.Equal(t, 2, b.Rebind())

	assert.Empty(t, l.drops())
	assert.Equal(t, map[string]int{"ws-a": 1, "ws-b": 1}, l.held())
}

// TestBroadcaster_AnUnplacedOrUnresolvedClientHoldsNoNewScope: a binding of
// "" holds nothing, and a failed resolve keeps the last binding's scope.
func TestBroadcaster_AnUnplacedOrUnresolvedClientHoldsNoNewScope(t *testing.T) {
	p := newPlacements(map[string]string{})
	l := newScopeLedger()
	b, srv := boundSetup(t, scopedBoundDef(p, l))
	conn := dial(t, srv, "/chats/chat-orphan/ws")
	assert.Empty(t, l.held(), "an unplaced chat holds no scope")

	p.move("chat-orphan", "ws-z")
	require.Equal(t, 1, b.Rebind())
	assert.Equal(t, "+ws-z", <-l.ops)

	p.move("chat-orphan", "ws-y")
	p.fail(true)
	require.Equal(t, 0, b.Rebind())
	assert.Equal(t, map[string]int{"ws-z": 1}, l.held(), "a failed resolve keeps the scope")

	require.NoError(t, conn.Close())
	assert.Equal(t, "-ws-z", <-l.ops)
	assert.Empty(t, l.ops)
}
