package handler

import (
	"context"
	"encoding/json"
	"fmt"
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

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agent"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// fakeHub is an in-process hubber for unit tests.
type fakeHub struct {
	mu       sync.Mutex
	subs     map[string][]chan agent.ChatFrame
	forwards []forwardCall
}

type forwardCall struct {
	taskID  string
	content string
}

func newFakeHub() *fakeHub {
	return &fakeHub{
		subs: make(map[string][]chan agent.ChatFrame),
	}
}

func (f *fakeHub) Subscribe(
	taskID string,
) (frames <-chan agent.ChatFrame, unsubscribe func()) {
	ch := make(chan agent.ChatFrame, 64)
	f.mu.Lock()
	f.subs[taskID] = append(f.subs[taskID], ch)
	f.mu.Unlock()

	unsub := func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		list := f.subs[taskID]
		next := make([]chan agent.ChatFrame, 0, len(list))
		for _, c := range list {
			if c != ch {
				next = append(next, c)
			}
		}
		f.subs[taskID] = next
	}

	return ch, unsub
}

func (f *fakeHub) Forward(
	taskID string,
	content string,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forwards = append(f.forwards, forwardCall{taskID: taskID, content: content})
}

// Publish pushes a frame to all subscribers for taskID.
func (f *fakeHub) Publish(
	taskID string,
	frame agent.ChatFrame,
) {
	f.mu.Lock()
	chs := make([]chan agent.ChatFrame, len(f.subs[taskID]))
	copy(chs, f.subs[taskID])
	f.mu.Unlock()

	for _, ch := range chs {
		select {
		case ch <- frame:
		default:
		}
	}
}

// CloseSubs closes all subscriber channels for taskID so writePump receives
// the !ok signal.
func (f *fakeHub) CloseSubs(taskID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ch := range f.subs[taskID] {
		close(ch)
	}
	f.subs[taskID] = nil
}

// closingHub wraps fakeHub and closes the subscription channel immediately so
// writePump sees the !ok branch.
type closingHub struct {
	*fakeHub
}

func (c *closingHub) Subscribe(
	taskID string,
) (frames <-chan agent.ChatFrame, unsubscribe func()) {
	ch := make(chan agent.ChatFrame)
	close(ch) // immediately closed → writePump sees !ok
	return ch, func() {}
}

// fakeConversation records Create calls for assertion.
type fakeConversation struct {
	mu      sync.Mutex
	created []createCall
}

type createCall struct {
	taskID  string
	role    domain.ConversationMessageRole
	msgType domain.ConversationMessageType
	content string
}

func (f *fakeConversation) Create(
	ctx context.Context,
	taskID string,
	role domain.ConversationMessageRole,
	msgType domain.ConversationMessageType,
	content string,
) (domain.ConversationMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, createCall{
		taskID:  taskID,
		role:    role,
		msgType: msgType,
		content: content,
	})
	return domain.ConversationMessage{
		ID:      "fake-id",
		TaskID:  taskID,
		Role:    role,
		Type:    msgType,
		Content: content,
	}, nil
}

func (f *fakeConversation) getCreated() []createCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]createCall, len(f.created))
	copy(out, f.created)
	return out
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestServer spins up an httptest.Server that calls handler.Handle for
// every WebSocket upgrade request. The server URL and a cleanup func are
// returned.
func newTestServer(
	t *testing.T,
	h ChatHandler,
	taskID string,
) (wsURL string, cleanup func()) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/ws", func(c *gin.Context) {
		h.Handle(context.Background(), c, taskID)
	})

	srv := httptest.NewServer(router)
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	return url, srv.Close
}

// dialWS dials the test server and returns a *websocket.Conn.
func dialWS(
	t *testing.T,
	url string,
) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	return conn
}

// sendJSON marshals v and writes it as a text message.
func sendJSON(
	t *testing.T,
	conn *websocket.Conn,
	v any,
) {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, b))
}

// readFrame reads one JSON frame from the connection with a timeout.
func readFrame(
	t *testing.T,
	conn *websocket.Conn,
) agent.ChatFrame {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, b, err := conn.ReadMessage()
	require.NoError(t, err)
	var f agent.ChatFrame
	require.NoError(t, json.Unmarshal(b, &f))
	return f
}

// ---------------------------------------------------------------------------
// New
// ---------------------------------------------------------------------------

func TestNew_returnsNonNil(t *testing.T) {
	h := New(newFakeHub(), &fakeConversation{})
	assert.NotNil(t, h)
}

// ---------------------------------------------------------------------------
// Handle — upgrade failure
// ---------------------------------------------------------------------------

func TestHandle_nonWebSocketRequest_returnsWithoutPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	hub := newFakeHub()
	conv := &fakeConversation{}
	h := New(hub, conv)

	router := gin.New()
	router.GET("/ws", func(c *gin.Context) {
		h.Handle(context.Background(), c, "task-1")
	})

	// Plain HTTP GET — not a WS upgrade; Handle must return cleanly.
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
}

// ---------------------------------------------------------------------------
// readPump
// ---------------------------------------------------------------------------

func TestReadPump_userMessage_persistedAndForwarded(t *testing.T) {
	hub := newFakeHub()
	conv := &fakeConversation{}
	h := New(hub, conv)

	url, cleanup := newTestServer(t, h, "task-read")
	defer cleanup()

	conn := dialWS(t, url)
	defer conn.Close()

	msg := incomingMsg{
		Type:    agent.ChatFrameTypeUserMessage,
		Content: "hello world",
	}
	sendJSON(t, conn, msg)

	require.Eventually(t, func() bool {
		created := conv.getCreated()
		for _, c := range created {
			if c.content == "hello world" && c.role == domain.ConversationMessageRoleUser {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "user message not persisted")

	require.Eventually(t, func() bool {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		for _, fw := range hub.forwards {
			if fw.taskID == "task-read" && fw.content == "hello world" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "user message not forwarded to hub")
}

func TestReadPump_nonUserMessage_ignoredNoForward(t *testing.T) {
	hub := newFakeHub()
	conv := &fakeConversation{}
	h := New(hub, conv)

	url, cleanup := newTestServer(t, h, "task-ignore")
	defer cleanup()

	conn := dialWS(t, url)
	defer conn.Close()

	msg := incomingMsg{
		Type:    agent.ChatFrameTypeAgentChunk,
		Content: "should be ignored",
	}
	sendJSON(t, conn, msg)

	time.Sleep(100 * time.Millisecond)
	assert.Empty(t, conv.getCreated(), "non-user message should not be persisted")
	hub.mu.Lock()
	assert.Empty(t, hub.forwards, "non-user message should not be forwarded")
	hub.mu.Unlock()
}

func TestReadPump_malformedJSON_skipped(t *testing.T) {
	hub := newFakeHub()
	conv := &fakeConversation{}
	h := New(hub, conv)

	url, cleanup := newTestServer(t, h, "task-bad-json")
	defer cleanup()

	conn := dialWS(t, url)
	defer conn.Close()

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("not json {{{")))

	msg := incomingMsg{
		Type:    agent.ChatFrameTypeUserMessage,
		Content: "after bad json",
	}
	sendJSON(t, conn, msg)

	require.Eventually(t, func() bool {
		created := conv.getCreated()
		for _, c := range created {
			if c.content == "after bad json" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "pump did not recover after malformed JSON")
}

func TestReadPump_persistError_stillForwards(t *testing.T) {
	hub := newFakeHub()
	conv := &errConversation{}
	h := New(hub, conv)

	url, cleanup := newTestServer(t, h, "task-err")
	defer cleanup()

	conn := dialWS(t, url)
	defer conn.Close()

	sendJSON(t, conn, incomingMsg{
		Type:    agent.ChatFrameTypeUserMessage,
		Content: "msg-with-err",
	})

	require.Eventually(t, func() bool {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		for _, fw := range hub.forwards {
			if fw.content == "msg-with-err" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "Forward not called despite persist error")
}

// errConversation always returns an error from Create.
type errConversation struct{}

func (e *errConversation) Create(
	ctx context.Context,
	taskID string,
	role domain.ConversationMessageRole,
	msgType domain.ConversationMessageType,
	content string,
) (domain.ConversationMessage, error) {
	return domain.ConversationMessage{}, errFakeConv
}

var errFakeConv = fmt.Errorf("fake conversation error")

// ---------------------------------------------------------------------------
// writePump
// ---------------------------------------------------------------------------

func TestWritePump_agentChunk_sentToWebSocket(t *testing.T) {
	hub := newFakeHub()
	conv := &fakeConversation{}
	h := New(hub, conv)

	url, cleanup := newTestServer(t, h, "task-write")
	defer cleanup()

	conn := dialWS(t, url)
	defer conn.Close()

	hub.Publish("task-write", agent.ChatFrame{
		Type:  agent.ChatFrameTypeAgentChunk,
		Delta: "chunk1",
	})

	f := readFrame(t, conn)
	assert.Equal(t, agent.ChatFrameTypeAgentChunk, f.Type)
	assert.Equal(t, "chunk1", f.Delta)
}

func TestWritePump_agentTurnEnd_assemblesAndPersists(t *testing.T) {
	hub := newFakeHub()
	conv := &fakeConversation{}
	h := New(hub, conv)

	url, cleanup := newTestServer(t, h, "task-turn")
	defer cleanup()

	conn := dialWS(t, url)
	defer conn.Close()

	hub.Publish("task-turn", agent.ChatFrame{
		Type:  agent.ChatFrameTypeAgentChunk,
		Delta: "hello ",
	})
	hub.Publish("task-turn", agent.ChatFrame{
		Type:  agent.ChatFrameTypeAgentChunk,
		Delta: "world",
	})
	hub.Publish("task-turn", agent.ChatFrame{
		Type: agent.ChatFrameTypeAgentTurnEnd,
	})

	_ = readFrame(t, conn)
	_ = readFrame(t, conn)
	_ = readFrame(t, conn)

	require.Eventually(t, func() bool {
		created := conv.getCreated()
		for _, c := range created {
			if c.role == domain.ConversationMessageRoleAgent && c.content == "hello world" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "agent turn not persisted with assembled content")
}

func TestWritePump_agentTurnEnd_emptyBuffer_notPersisted(t *testing.T) {
	hub := newFakeHub()
	conv := &fakeConversation{}
	h := New(hub, conv)

	url, cleanup := newTestServer(t, h, "task-empty-turn")
	defer cleanup()

	conn := dialWS(t, url)
	defer conn.Close()

	hub.Publish("task-empty-turn", agent.ChatFrame{
		Type: agent.ChatFrameTypeAgentTurnEnd,
	})

	_ = readFrame(t, conn)

	time.Sleep(100 * time.Millisecond)
	assert.Empty(t, conv.getCreated(), "empty turn should not be persisted")
}

func TestWritePump_persistTurnError_doesNotCrash(t *testing.T) {
	hub := newFakeHub()
	conv := &errConversation{}
	h := New(hub, conv)

	url, cleanup := newTestServer(t, h, "task-turn-err")
	defer cleanup()

	conn := dialWS(t, url)
	defer conn.Close()

	hub.Publish("task-turn-err", agent.ChatFrame{
		Type:  agent.ChatFrameTypeAgentChunk,
		Delta: "data",
	})
	hub.Publish("task-turn-err", agent.ChatFrame{
		Type: agent.ChatFrameTypeAgentTurnEnd,
	})

	_ = readFrame(t, conn)
	_ = readFrame(t, conn)
}

func TestWritePump_contextCancelled_exits(t *testing.T) {
	hub := newFakeHub()
	conv := &fakeConversation{}

	gin.SetMode(gin.TestMode)
	router := gin.New()

	ctx, cancel := context.WithCancel(context.Background())
	h := New(hub, conv)

	router.GET("/ws", func(c *gin.Context) {
		h.Handle(ctx, c, "task-cancel")
	})

	srv := httptest.NewServer(router)
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	conn := dialWS(t, url)
	defer conn.Close()

	cancel()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := conn.ReadMessage()
	_ = err
}

func TestWritePump_multipleFrameTypes_delivered(t *testing.T) {
	hub := newFakeHub()
	conv := &fakeConversation{}
	h := New(hub, conv)

	url, cleanup := newTestServer(t, h, "task-multi")
	defer cleanup()

	conn := dialWS(t, url)
	defer conn.Close()

	types := []agent.ChatFrameType{
		agent.ChatFrameTypeToolCall,
		agent.ChatFrameTypeToolResult,
		agent.ChatFrameTypeStateTransition,
	}

	for _, ft := range types {
		hub.Publish("task-multi", agent.ChatFrame{Type: ft})
	}

	for _, ft := range types {
		f := readFrame(t, conn)
		assert.Equal(t, ft, f.Type)
	}
}

func TestWritePump_pingFired_sentToWebSocket(t *testing.T) {
	orig := pingInterval
	pingInterval = 50 * time.Millisecond
	t.Cleanup(func() { pingInterval = orig })

	hub := newFakeHub()
	conv := &fakeConversation{}
	h := New(hub, conv)

	url, cleanup := newTestServer(t, h, "task-ping")
	defer cleanup()

	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	pongReceived := make(chan struct{}, 1)
	conn.SetPingHandler(func(appData string) error {
		select {
		case pongReceived <- struct{}{}:
		default:
		}
		return conn.WriteMessage(websocket.PongMessage, []byte(appData))
	})

	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	select {
	case <-pongReceived:
	case <-time.After(2 * time.Second):
		t.Fatal("ping never fired")
	}
}

func TestWritePump_pingWriteError_exits(t *testing.T) {
	orig := pingInterval
	pingInterval = 50 * time.Millisecond
	t.Cleanup(func() { pingInterval = orig })

	hub := newFakeHub()
	conv := &fakeConversation{}
	h := New(hub, conv)

	url, cleanup := newTestServer(t, h, "task-ping-err")
	defer cleanup()

	conn := dialWS(t, url)

	conn.Close()

	time.Sleep(300 * time.Millisecond)
}

func TestWritePump_closedChannel_exits(t *testing.T) {
	hub := &closingHub{fakeHub: newFakeHub()}
	conv := &fakeConversation{}
	h := New(hub, conv)

	url, cleanup := newTestServer(t, h, "task-closed-ch")
	defer cleanup()

	conn := dialWS(t, url)
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := conn.ReadMessage()
	_ = err
}

func TestWritePump_writeError_exits(t *testing.T) {
	hub := newFakeHub()
	conv := &fakeConversation{}
	h := New(hub, conv)

	url, cleanup := newTestServer(t, h, "task-write-err")
	defer cleanup()

	conn := dialWS(t, url)

	conn.Close()

	hub.Publish("task-write-err", agent.ChatFrame{
		Type:  agent.ChatFrameTypeAgentChunk,
		Delta: "x",
	})

	time.Sleep(200 * time.Millisecond)
}
