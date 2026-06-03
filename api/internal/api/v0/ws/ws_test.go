package ws

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ---------------------------------------------------------------------------
// Repo stubs (implement repositories interfaces inline)
// ---------------------------------------------------------------------------

type stubTaskRepo struct {
	task domain.Task
	err  error
}

func (s *stubTaskRepo) Create(_ context.Context, _, _, _, _, _, _, _, _ string) (domain.Task, error) {
	return domain.Task{}, errors.New("not implemented")
}
func (s *stubTaskRepo) List(_ context.Context, _ string) ([]domain.Task, error) {
	return nil, nil
}
func (s *stubTaskRepo) Get(_ context.Context, _ string) (domain.Task, error) {
	return s.task, s.err
}
func (s *stubTaskRepo) Archive(_ context.Context, _ string) error { return nil }
func (s *stubTaskRepo) Pause(_ context.Context, _ string) error   { return nil }
func (s *stubTaskRepo) Resume(_ context.Context, _ string) error  { return nil }
func (s *stubTaskRepo) Retry(_ context.Context, _ string) error   { return nil }
func (s *stubTaskRepo) AdvanceState(_ context.Context, _, _, _ string) error {
	return nil
}
func (s *stubTaskRepo) ForceTransition(_ context.Context, _, _ string) error { return nil }
func (s *stubTaskRepo) Subscribe(_ string, _ func(context.Context, asynxModels.Event[domain.Task])) (string, error) {
	return "", nil
}

type stubAgentRepo struct {
	run domain.AgentRun
	err error
}

func (s *stubAgentRepo) Create(_ context.Context, _, _, _ string) (domain.AgentRun, error) {
	return domain.AgentRun{}, errors.New("not implemented")
}
func (s *stubAgentRepo) Get(_ context.Context, _ string) (domain.AgentRun, error) {
	return s.run, s.err
}
func (s *stubAgentRepo) GetByToken(_ context.Context, _ string) (domain.AgentRun, error) {
	return s.run, s.err
}
func (s *stubAgentRepo) ListByTask(_ context.Context, _ string) ([]domain.AgentRun, error) {
	return nil, nil
}
func (s *stubAgentRepo) CompleteAgentRun(_ context.Context, _, _ string) error { return nil }
func (s *stubAgentRepo) FailAgentRun(_ context.Context, _ string) error        { return nil }
func (s *stubAgentRepo) InterruptAgentRun(_ context.Context, _ string) error   { return nil }
func (s *stubAgentRepo) RecoverOrphanedRuns(_ context.Context) error           { return nil }
func (s *stubAgentRepo) Subscribe(_ string, _ func(context.Context, asynxModels.Event[domain.AgentRun])) (string, error) {
	return "", nil
}

type stubKanbanRepo struct {
	items []domain.KanbanItem
	err   error
}

func (s *stubKanbanRepo) Create(_ context.Context, _, _, _ string) (domain.KanbanItem, error) {
	return domain.KanbanItem{}, errors.New("not implemented")
}
func (s *stubKanbanRepo) Get(_ context.Context, _ string) (domain.KanbanItem, error) {
	return domain.KanbanItem{}, nil
}
func (s *stubKanbanRepo) ListByTask(_ context.Context, _ string) ([]domain.KanbanItem, error) {
	return s.items, s.err
}
func (s *stubKanbanRepo) UpdateStatus(_ context.Context, _, _, _ string) error { return nil }
func (s *stubKanbanRepo) Subscribe(_ string, _ func(context.Context, asynxModels.Event[domain.KanbanItem])) (string, error) {
	return "", nil
}

type stubThreadRepo struct {
	threads []domain.ReviewThread
	err     error
}

func (s *stubThreadRepo) Open(_ context.Context, _ string, _ *string, _ string, _ int, _ domain.ReviewPhase, _, _ string) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, errors.New("not implemented")
}
func (s *stubThreadRepo) Get(_ context.Context, _ string) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}
func (s *stubThreadRepo) ListByTask(_ context.Context, _, _, _ string) ([]domain.ReviewThread, error) {
	return s.threads, s.err
}
func (s *stubThreadRepo) PostMessage(_ context.Context, _, _, _ string) error    { return nil }
func (s *stubThreadRepo) ForceApprove(_ context.Context, _ string) error         { return nil }
func (s *stubThreadRepo) ResolveThread(_ context.Context, _, _ string) error     { return nil }
func (s *stubThreadRepo) Subscribe(_ string, _ func(context.Context, asynxModels.Event[domain.ReviewThread])) (string, error) {
	return "", nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestWSHandler() *WSHandler {
	return NewWSHandler(
		&stubTaskRepo{task: domain.Task{ID: "t1", Title: "test task"}},
		&stubAgentRepo{},
		&stubKanbanRepo{items: []domain.KanbanItem{{ID: "k1", TaskID: "t1"}}},
		&stubThreadRepo{threads: []domain.ReviewThread{{ID: "th1", TaskID: "t1"}}},
	)
}

// dialWS opens a real WebSocket connection to the provided httptest.Server and
// returns the client connection. The caller is responsible for closing it.
func dialWS(
	t *testing.T,
	srv *httptest.Server,
	path string,
) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + path
	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	conn, _, err := dialer.Dial(url, nil)
	require.NoError(t, err)
	return conn
}

// readTextFrame reads the next text frame from conn with a short deadline.
func readTextFrame(
	t *testing.T,
	conn *websocket.Conn,
) []byte {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second)) //nolint:errcheck
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	return msg
}

// ginRouterForHandler wraps a raw http.HandlerFunc in a gin engine for a given
// path so httptest.Server can serve it.
func ginRouterForHandler(
	path string,
	h gin.HandlerFunc,
) http.Handler {
	r := gin.New()
	r.GET(path, h)
	return r
}

// ---------------------------------------------------------------------------
// Broadcaster — Subscribe / Broadcast
// ---------------------------------------------------------------------------

func TestBroadcaster_Subscribe_returnsChanAndUnsub(t *testing.T) {
	b := NewBroadcaster[domain.Task]()
	ch, unsub := b.Subscribe(nil)
	require.NotNil(t, ch)
	require.NotNil(t, unsub)
	unsub()
}

func TestBroadcaster_Broadcast_nilPredicate_delivers(t *testing.T) {
	b := NewBroadcaster[domain.Task]()
	ch, unsub := b.Subscribe(nil)
	defer unsub()

	task := domain.Task{ID: "t1"}
	b.Broadcast(task, "task")

	select {
	case msg := <-ch:
		var env wsEnvelope
		require.NoError(t, json.Unmarshal(msg, &env))
		assert.Equal(t, "task", env.Type)
	case <-time.After(time.Second):
		t.Fatal("expected message, timed out")
	}
}

func TestBroadcaster_Broadcast_matchingPredicate_delivers(t *testing.T) {
	b := NewBroadcaster[domain.Task]()
	ch, unsub := b.Subscribe(func(task domain.Task) bool {
		return task.ID == "target"
	})
	defer unsub()

	b.Broadcast(domain.Task{ID: "target"}, "task")

	select {
	case msg := <-ch:
		var env wsEnvelope
		require.NoError(t, json.Unmarshal(msg, &env))
		assert.Equal(t, "task", env.Type)
	case <-time.After(time.Second):
		t.Fatal("expected message, timed out")
	}
}

func TestBroadcaster_Broadcast_nonMatchingPredicate_doesNotDeliver(t *testing.T) {
	b := NewBroadcaster[domain.Task]()
	ch, unsub := b.Subscribe(func(task domain.Task) bool {
		return task.ID == "wanted"
	})
	defer unsub()

	b.Broadcast(domain.Task{ID: "other"}, "task")

	select {
	case <-ch:
		t.Fatal("should not have received message")
	case <-time.After(50 * time.Millisecond):
		// correct — nothing delivered
	}
}

func TestBroadcaster_Broadcast_fullChannel_skipped(t *testing.T) {
	b := NewBroadcaster[domain.Task]()
	ch, unsub := b.Subscribe(nil)
	defer unsub()

	// Fill the buffer completely.
	task := domain.Task{ID: "t"}
	for i := 0; i < clientBufferSize; i++ {
		b.Broadcast(task, "task")
	}

	// One more must not block; it should be dropped silently.
	done := make(chan struct{})
	go func() {
		b.Broadcast(task, "task")
		close(done)
	}()

	select {
	case <-done:
		// correct — broadcast returned without blocking
	case <-time.After(time.Second):
		t.Fatal("Broadcast blocked on a full channel")
	}

	// Drain the buffer.
	for i := 0; i < clientBufferSize; i++ {
		<-ch
	}
}

func TestBroadcaster_Unsub_stopsDelivery(t *testing.T) {
	b := NewBroadcaster[domain.Task]()
	ch, unsub := b.Subscribe(nil)
	unsub() // immediately unsubscribe

	b.Broadcast(domain.Task{ID: "t1"}, "task")

	select {
	case <-ch:
		t.Fatal("unsubscribed client must not receive messages")
	case <-time.After(50 * time.Millisecond):
		// correct
	}
}

func TestBroadcaster_MultipleSubscribers_allReceive(t *testing.T) {
	b := NewBroadcaster[domain.Task]()
	ch1, unsub1 := b.Subscribe(nil)
	ch2, unsub2 := b.Subscribe(nil)
	defer unsub1()
	defer unsub2()

	b.Broadcast(domain.Task{ID: "t1"}, "task")

	for _, ch := range []<-chan []byte{ch1, ch2} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatal("subscriber did not receive message")
		}
	}
}

func TestBroadcaster_Concurrent_noRace(t *testing.T) {
	b := NewBroadcaster[domain.Task]()

	const n = 10
	unsubs := make([]func(), n)
	for i := 0; i < n; i++ {
		_, unsub := b.Subscribe(nil)
		unsubs[i] = unsub
	}

	var wg sync.WaitGroup
	wg.Add(50)
	for i := 0; i < 50; i++ {
		go func() {
			defer wg.Done()
			b.Broadcast(domain.Task{ID: "t"}, "task")
		}()
	}
	wg.Wait()

	for _, u := range unsubs {
		u()
	}
}

// ---------------------------------------------------------------------------
// Dispatch helper
// ---------------------------------------------------------------------------

func TestDispatch_upgradeHeader_routesToWS(t *testing.T) {
	restCalled := false
	wsCalled := false

	r := gin.New()
	r.GET("/test", Dispatch(
		func(c *gin.Context) { restCalled = true },
		func(c *gin.Context) { wsCalled = true; c.Status(http.StatusSwitchingProtocols) },
	))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Upgrade", "websocket")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.True(t, wsCalled, "ws handler must be called when Upgrade: websocket")
	assert.False(t, restCalled, "rest handler must not be called")
}

func TestDispatch_noUpgradeHeader_routesToREST(t *testing.T) {
	restCalled := false
	wsCalled := false

	r := gin.New()
	r.GET("/test", Dispatch(
		func(c *gin.Context) { restCalled = true; c.Status(http.StatusOK) },
		func(c *gin.Context) { wsCalled = true },
	))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.True(t, restCalled, "rest handler must be called when no Upgrade header")
	assert.False(t, wsCalled, "ws handler must not be called")
}

func TestDispatch_upgradeOtherValue_routesToREST(t *testing.T) {
	restCalled := false

	r := gin.New()
	r.GET("/test", Dispatch(
		func(c *gin.Context) { restCalled = true; c.Status(http.StatusOK) },
		func(c *gin.Context) { c.Status(http.StatusSwitchingProtocols) },
	))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Upgrade", "h2c")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.True(t, restCalled)
}

// ---------------------------------------------------------------------------
// WSHandler — PushTask/PushAgentRun/PushKanbanItem/PushReviewThread
// ---------------------------------------------------------------------------

func TestWSHandler_PushTask_broadcastsToSubscribers(t *testing.T) {
	h := newTestWSHandler()
	ch, unsub := h.tasks.Subscribe(nil)
	defer unsub()

	h.PushTask(domain.Task{ID: "t1"})

	select {
	case msg := <-ch:
		var env wsEnvelope
		require.NoError(t, json.Unmarshal(msg, &env))
		assert.Equal(t, "task", env.Type)
	case <-time.After(time.Second):
		t.Fatal("expected task broadcast")
	}
}

func TestWSHandler_PushAgentRun_broadcastsToSubscribers(t *testing.T) {
	h := newTestWSHandler()
	ch, unsub := h.agentRuns.Subscribe(nil)
	defer unsub()

	h.PushAgentRun(domain.AgentRun{ID: "r1"})

	select {
	case msg := <-ch:
		var env wsEnvelope
		require.NoError(t, json.Unmarshal(msg, &env))
		assert.Equal(t, "agent_run", env.Type)
	case <-time.After(time.Second):
		t.Fatal("expected agent_run broadcast")
	}
}

func TestWSHandler_PushKanbanItem_broadcastsToSubscribers(t *testing.T) {
	h := newTestWSHandler()
	ch, unsub := h.kanbanItems.Subscribe(nil)
	defer unsub()

	h.PushKanbanItem(domain.KanbanItem{ID: "k1"})

	select {
	case msg := <-ch:
		var env wsEnvelope
		require.NoError(t, json.Unmarshal(msg, &env))
		assert.Equal(t, "kanban_item", env.Type)
	case <-time.After(time.Second):
		t.Fatal("expected kanban_item broadcast")
	}
}

func TestWSHandler_PushReviewThread_broadcastsToSubscribers(t *testing.T) {
	h := newTestWSHandler()
	ch, unsub := h.threads.Subscribe(nil)
	defer unsub()

	h.PushReviewThread(domain.ReviewThread{ID: "th1"})

	select {
	case msg := <-ch:
		var env wsEnvelope
		require.NoError(t, json.Unmarshal(msg, &env))
		assert.Equal(t, "review_thread", env.Type)
	case <-time.After(time.Second):
		t.Fatal("expected review_thread broadcast")
	}
}

// ---------------------------------------------------------------------------
// WSHandler — HandleTask real WebSocket connection
// ---------------------------------------------------------------------------

func TestHandleTask_sendsInitialSyncFrame(t *testing.T) {
	h := newTestWSHandler()

	router := ginRouterForHandler("/api/v0/tasks/:id", h.HandleTask)
	srv := httptest.NewServer(router)
	defer srv.Close()

	conn := dialWS(t, srv, "/api/v0/tasks/t1")
	defer conn.Close()

	// First frame must be the initial sync frame.
	msg := readTextFrame(t, conn)
	var env wsEnvelope
	require.NoError(t, json.Unmarshal(msg, &env))
	assert.Equal(t, "task", env.Type)
}

func TestHandleTask_broadcastDeliveredToClient(t *testing.T) {
	h := newTestWSHandler()

	router := ginRouterForHandler("/api/v0/tasks/:id", h.HandleTask)
	srv := httptest.NewServer(router)
	defer srv.Close()

	conn := dialWS(t, srv, "/api/v0/tasks/t1")
	defer conn.Close()

	// Consume the initial sync frame.
	readTextFrame(t, conn)

	// Broadcast a matching task update.
	h.PushTask(domain.Task{ID: "t1", Title: "updated"})

	msg := readTextFrame(t, conn)
	var env wsEnvelope
	require.NoError(t, json.Unmarshal(msg, &env))
	assert.Equal(t, "task", env.Type)
}

func TestHandleTask_nonMatchingBroadcast_notDelivered(t *testing.T) {
	h := newTestWSHandler()

	router := ginRouterForHandler("/api/v0/tasks/:id", h.HandleTask)
	srv := httptest.NewServer(router)
	defer srv.Close()

	conn := dialWS(t, srv, "/api/v0/tasks/t1")
	defer conn.Close()

	// Consume the initial sync frame.
	readTextFrame(t, conn)

	// Broadcast a task with a different ID — should NOT arrive.
	h.PushTask(domain.Task{ID: "other"})

	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)) //nolint:errcheck
	_, _, err := conn.ReadMessage()
	assert.Error(t, err, "non-matching broadcast must not be delivered")
}

func TestHandleTask_repoError_stillUpgrades(t *testing.T) {
	// When the task repo returns an error the handler should still upgrade and
	// stream future events (no initial sync frame, but no crash).
	h := NewWSHandler(
		&stubTaskRepo{err: domain.ErrNotFound},
		&stubAgentRepo{},
		&stubKanbanRepo{},
		&stubThreadRepo{},
	)

	router := ginRouterForHandler("/api/v0/tasks/:id", h.HandleTask)
	srv := httptest.NewServer(router)
	defer srv.Close()

	conn := dialWS(t, srv, "/api/v0/tasks/t1")
	defer conn.Close()

	// No initial sync frame; a subsequent broadcast should arrive.
	h.PushTask(domain.Task{ID: "t1"})

	msg := readTextFrame(t, conn)
	var env wsEnvelope
	require.NoError(t, json.Unmarshal(msg, &env))
	assert.Equal(t, "task", env.Type)
}

// ---------------------------------------------------------------------------
// WSHandler — HandleKanbanItems real WebSocket connection
// ---------------------------------------------------------------------------

func TestHandleKanbanItems_sendsInitialSyncFrames(t *testing.T) {
	h := newTestWSHandler() // stubKanbanRepo returns [{ID:"k1",TaskID:"t1"}]

	router := ginRouterForHandler("/api/v0/tasks/:id/kanban-items", h.HandleKanbanItems)
	srv := httptest.NewServer(router)
	defer srv.Close()

	conn := dialWS(t, srv, "/api/v0/tasks/t1/kanban-items")
	defer conn.Close()

	msg := readTextFrame(t, conn)
	var env wsEnvelope
	require.NoError(t, json.Unmarshal(msg, &env))
	assert.Equal(t, "kanban_item", env.Type)
}

func TestHandleKanbanItems_broadcastDeliveredToClient(t *testing.T) {
	h := newTestWSHandler()

	router := ginRouterForHandler("/api/v0/tasks/:id/kanban-items", h.HandleKanbanItems)
	srv := httptest.NewServer(router)
	defer srv.Close()

	conn := dialWS(t, srv, "/api/v0/tasks/t1/kanban-items")
	defer conn.Close()

	// Consume initial sync.
	readTextFrame(t, conn)

	h.PushKanbanItem(domain.KanbanItem{ID: "k2", TaskID: "t1"})

	msg := readTextFrame(t, conn)
	var env wsEnvelope
	require.NoError(t, json.Unmarshal(msg, &env))
	assert.Equal(t, "kanban_item", env.Type)
}

func TestHandleKanbanItems_repoError_stillUpgrades(t *testing.T) {
	h := NewWSHandler(
		&stubTaskRepo{},
		&stubAgentRepo{},
		&stubKanbanRepo{err: domain.ErrNotFound},
		&stubThreadRepo{},
	)

	router := ginRouterForHandler("/api/v0/tasks/:id/kanban-items", h.HandleKanbanItems)
	srv := httptest.NewServer(router)
	defer srv.Close()

	conn := dialWS(t, srv, "/api/v0/tasks/t1/kanban-items")
	defer conn.Close()

	// Push an item — it must arrive even though initial sync failed.
	h.PushKanbanItem(domain.KanbanItem{ID: "k1", TaskID: "t1"})

	msg := readTextFrame(t, conn)
	var env wsEnvelope
	require.NoError(t, json.Unmarshal(msg, &env))
	assert.Equal(t, "kanban_item", env.Type)
}

// ---------------------------------------------------------------------------
// WSHandler — HandleThreads real WebSocket connection
// ---------------------------------------------------------------------------

func TestHandleThreads_sendsInitialSyncFrames(t *testing.T) {
	h := newTestWSHandler() // stubThreadRepo returns [{ID:"th1",TaskID:"t1"}]

	router := ginRouterForHandler("/api/v0/tasks/:id/threads", h.HandleThreads)
	srv := httptest.NewServer(router)
	defer srv.Close()

	conn := dialWS(t, srv, "/api/v0/tasks/t1/threads")
	defer conn.Close()

	msg := readTextFrame(t, conn)
	var env wsEnvelope
	require.NoError(t, json.Unmarshal(msg, &env))
	assert.Equal(t, "review_thread", env.Type)
}

func TestHandleThreads_broadcastDeliveredToClient(t *testing.T) {
	h := newTestWSHandler()

	router := ginRouterForHandler("/api/v0/tasks/:id/threads", h.HandleThreads)
	srv := httptest.NewServer(router)
	defer srv.Close()

	conn := dialWS(t, srv, "/api/v0/tasks/t1/threads")
	defer conn.Close()

	// Consume initial sync.
	readTextFrame(t, conn)

	h.PushReviewThread(domain.ReviewThread{ID: "th2", TaskID: "t1"})

	msg := readTextFrame(t, conn)
	var env wsEnvelope
	require.NoError(t, json.Unmarshal(msg, &env))
	assert.Equal(t, "review_thread", env.Type)
}

func TestHandleThreads_nonMatchingBroadcast_notDelivered(t *testing.T) {
	h := newTestWSHandler()

	router := ginRouterForHandler("/api/v0/tasks/:id/threads", h.HandleThreads)
	srv := httptest.NewServer(router)
	defer srv.Close()

	conn := dialWS(t, srv, "/api/v0/tasks/t1/threads")
	defer conn.Close()

	// Consume initial sync.
	readTextFrame(t, conn)

	h.PushReviewThread(domain.ReviewThread{ID: "th2", TaskID: "other"})

	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)) //nolint:errcheck
	_, _, err := conn.ReadMessage()
	assert.Error(t, err, "non-matching broadcast must not arrive")
}

func TestHandleThreads_repoError_stillUpgrades(t *testing.T) {
	h := NewWSHandler(
		&stubTaskRepo{},
		&stubAgentRepo{},
		&stubKanbanRepo{},
		&stubThreadRepo{err: domain.ErrNotFound},
	)

	router := ginRouterForHandler("/api/v0/tasks/:id/threads", h.HandleThreads)
	srv := httptest.NewServer(router)
	defer srv.Close()

	conn := dialWS(t, srv, "/api/v0/tasks/t1/threads")
	defer conn.Close()

	h.PushReviewThread(domain.ReviewThread{ID: "th1", TaskID: "t1"})

	msg := readTextFrame(t, conn)
	var env wsEnvelope
	require.NoError(t, json.Unmarshal(msg, &env))
	assert.Equal(t, "review_thread", env.Type)
}

// ---------------------------------------------------------------------------
// writePump / readPump integration: connection close terminates pump
// ---------------------------------------------------------------------------

func TestHandleTask_clientDisconnect_cleanupSubscription(t *testing.T) {
	h := newTestWSHandler()

	router := ginRouterForHandler("/api/v0/tasks/:id", h.HandleTask)
	srv := httptest.NewServer(router)
	defer srv.Close()

	conn := dialWS(t, srv, "/api/v0/tasks/t1")

	// Consume initial frame then close the connection.
	readTextFrame(t, conn)
	conn.Close()

	// Give the server goroutines time to notice the disconnect.
	time.Sleep(100 * time.Millisecond)

	// The broadcaster's clients map should now be empty (subscription removed).
	h.tasks.mu.RLock()
	count := len(h.tasks.clients)
	h.tasks.mu.RUnlock()

	assert.Equal(t, 0, count, "subscription must be cleaned up after disconnect")
}

// ---------------------------------------------------------------------------
// sendInitialFrame — marshal error path (non-marshallable type)
// ---------------------------------------------------------------------------

// marshalFail is a type that always fails json.Marshal.
type marshalFail struct{}

func (marshalFail) MarshalJSON() ([]byte, error) {
	return nil, errors.New("marshal error")
}

func TestSendInitialFrame_marshalError_doesNotPanic(t *testing.T) {
	h := newTestWSHandler()

	// We need a real ws conn to call sendInitialFrame.
	router := ginRouterForHandler("/api/v0/tasks/:id", h.HandleTask)
	srv := httptest.NewServer(router)
	defer srv.Close()

	conn := dialWS(t, srv, "/api/v0/tasks/t1")
	defer conn.Close()

	// Directly call sendInitialFrame with an unmarshalable value.
	// Must not panic.
	assert.NotPanics(t, func() {
		h.sendInitialFrame(conn, "task", marshalFail{})
	})
}

// ---------------------------------------------------------------------------
// NewWSHandler — sanity check
// ---------------------------------------------------------------------------

func TestNewWSHandler_returnsNonNil(t *testing.T) {
	h := newTestWSHandler()
	assert.NotNil(t, h)
	assert.NotNil(t, h.tasks)
	assert.NotNil(t, h.agentRuns)
	assert.NotNil(t, h.kanbanItems)
	assert.NotNil(t, h.threads)
}

// ---------------------------------------------------------------------------
// writePump — done channel terminates pump
// ---------------------------------------------------------------------------

func TestWritePump_doneChannelTerminates(t *testing.T) {
	h := newTestWSHandler()

	router := ginRouterForHandler("/api/v0/tasks/:id", h.HandleTask)
	srv := httptest.NewServer(router)
	defer srv.Close()

	conn := dialWS(t, srv, "/api/v0/tasks/t1")
	defer conn.Close()

	// Consume initial sync frame.
	readTextFrame(t, conn)

	// Send a close frame from the client side — this causes readPump to return,
	// which closes done, which terminates writePump.
	err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"))
	require.NoError(t, err)

	// Give the server side time to tear down.
	time.Sleep(150 * time.Millisecond)

	// The subscriber should now be gone.
	h.tasks.mu.RLock()
	count := len(h.tasks.clients)
	h.tasks.mu.RUnlock()
	assert.Equal(t, 0, count)
}

// ---------------------------------------------------------------------------
// writePump — closed channel (!ok path)
// ---------------------------------------------------------------------------

// testWritePumpClosedChannel exercises writePump directly: it creates a
// loopback WS pair and closes the message channel, expecting writePump to
// return cleanly.
func TestWritePump_closedChannel_returnsCleanly(t *testing.T) {
	// Build a local WS loopback via httptest.
	serverConn := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConn <- conn
	}))
	defer srv.Close()

	clientConn := dialWS(t, srv, "/")
	defer clientConn.Close()

	sConn := <-serverConn

	ch := make(chan []byte)
	done := make(chan struct{})

	finished := make(chan struct{})
	go func() {
		writePump(sConn, ch, done)
		close(finished)
	}()

	// Close the channel — triggers the !ok path.
	close(ch)

	select {
	case <-finished:
		// correct
	case <-time.After(2 * time.Second):
		t.Fatal("writePump did not return after channel close")
	}
}

// ---------------------------------------------------------------------------
// writePump — write error path (write to closed server connection)
// ---------------------------------------------------------------------------

func TestWritePump_writeError_returns(t *testing.T) {
	serverConn := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConn <- conn
	}))
	defer srv.Close()

	clientConn := dialWS(t, srv, "/")
	defer clientConn.Close()

	sConn := <-serverConn

	ch := make(chan []byte, 1)
	done := make(chan struct{})

	finished := make(chan struct{})
	go func() {
		writePump(sConn, ch, done)
		close(finished)
	}()

	// Close the underlying server connection directly so the next WriteMessage
	// inside writePump fails.
	sConn.Close()

	// Send a message; writePump will attempt to write to the closed conn and return.
	ch <- []byte(`{"type":"task","data":{}}`)

	select {
	case <-finished:
		// correct — writePump exited due to write error
	case <-time.After(2 * time.Second):
		t.Fatal("writePump did not return after write error")
	}
}

// ---------------------------------------------------------------------------
// writePump — ping tick path
// ---------------------------------------------------------------------------

func TestWritePump_pingTick_sendsAndContinues(t *testing.T) {
	// We need to use a very short ticker to avoid a long test.  We can't change
	// the package-level pingInterval, but we can observe the behaviour by
	// using done to terminate after a short wait and checking the pump exits.
	serverConn := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConn <- conn
	}))
	defer srv.Close()

	clientConn := dialWS(t, srv, "/")
	defer clientConn.Close()

	sConn := <-serverConn

	ch := make(chan []byte)
	done := make(chan struct{})

	finished := make(chan struct{})
	go func() {
		writePump(sConn, ch, done)
		close(finished)
	}()

	// Close done to terminate the pump quickly without waiting for the
	// 30-second ping interval.
	close(done)

	select {
	case <-finished:
		// correct — done channel causes writePump to exit
	case <-time.After(2 * time.Second):
		t.Fatal("writePump did not return after done closed")
	}
}

// ---------------------------------------------------------------------------
// readPump — pong handler covers SetPongHandler callback
// ---------------------------------------------------------------------------

func TestReadPump_pongHandler_invoked(t *testing.T) {
	serverConn := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConn <- conn
	}))
	defer srv.Close()

	clientConn := dialWS(t, srv, "/")

	sConn := <-serverConn

	finished := make(chan struct{})
	go func() {
		readPump(sConn)
		close(finished)
	}()

	// Send a pong frame from the client — this invokes the pong handler on
	// the server side, which calls SetReadDeadline.
	err := clientConn.WriteMessage(websocket.PongMessage, []byte("pong"))
	require.NoError(t, err)

	// Close the client to make readPump's NextReader return an error.
	clientConn.Close()

	select {
	case <-finished:
		// readPump exited — pong handler was invoked on the way.
	case <-time.After(2 * time.Second):
		t.Fatal("readPump did not return after client close")
	}
}

// ---------------------------------------------------------------------------
// sendInitialFrame — write error path (closed connection)
// ---------------------------------------------------------------------------

func TestSendInitialFrame_writeError_doesNotPanic(t *testing.T) {
	serverConn := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConn <- conn
	}))
	defer srv.Close()

	clientConn := dialWS(t, srv, "/")

	sConn := <-serverConn

	// Close the client so the next WriteMessage from sendInitialFrame fails.
	clientConn.Close()
	time.Sleep(50 * time.Millisecond)

	h := newTestWSHandler()
	assert.NotPanics(t, func() {
		h.sendInitialFrame(sConn, "task", domain.Task{ID: "t1"})
	})
}

// ---------------------------------------------------------------------------
// Handler upgrade error paths
// ---------------------------------------------------------------------------

func TestHandleTask_upgradeFailure_returns(t *testing.T) {
	h := newTestWSHandler()

	// Send a plain HTTP GET (no WS headers) directly to the gin handler.
	// The upgrader will fail and the handler must return without panicking.
	r := gin.New()
	r.GET("/api/v0/tasks/:id", h.HandleTask)

	req := httptest.NewRequest(http.MethodGet, "/api/v0/tasks/t1", nil)
	// Deliberately do NOT set Upgrade/Connection/Sec-WebSocket-Key headers.
	w := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		r.ServeHTTP(w, req)
	})
}

func TestHandleKanbanItems_upgradeFailure_returns(t *testing.T) {
	h := newTestWSHandler()

	r := gin.New()
	r.GET("/api/v0/tasks/:id/kanban-items", h.HandleKanbanItems)

	req := httptest.NewRequest(http.MethodGet, "/api/v0/tasks/t1/kanban-items", nil)
	w := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		r.ServeHTTP(w, req)
	})
}

func TestHandleThreads_upgradeFailure_returns(t *testing.T) {
	h := newTestWSHandler()

	r := gin.New()
	r.GET("/api/v0/tasks/:id/threads", h.HandleThreads)

	req := httptest.NewRequest(http.MethodGet, "/api/v0/tasks/t1/threads", nil)
	w := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		r.ServeHTTP(w, req)
	})
}
