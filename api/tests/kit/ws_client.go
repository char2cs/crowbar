//go:build integration || manual

package kit

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/gorilla/websocket"
)

// wsEnvelope mirrors the server-side envelope used for domain broadcaster frames.
type wsEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// WSClient is a WebSocket client that routes connections over a Unix socket.
type WSClient struct {
	t        *testing.T
	sockPath string
}

func newWSClient(
	t *testing.T,
	sockPath string,
) *WSClient {
	return &WSClient{t: t, sockPath: sockPath}
}

func (c *WSClient) dialer() *websocket.Dialer {
	return &websocket.Dialer{
		NetDialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", c.sockPath)
		},
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 30 * time.Second,
	}
}

// SubscribeTasks connects to the task WS endpoint and returns a channel of
// Task events and a close function. The initial sync frame is delivered on the
// channel like any other update.
func (c *WSClient) SubscribeTasks(
	taskID string,
) (<-chan domain.Task, func()) {
	c.t.Helper()
	url := fmt.Sprintf("ws://unix/api/v0/tasks/%s", taskID)
	conn, _, err := c.dialer().Dial(url, nil)
	if err != nil {
		c.t.Fatalf("WSClient.SubscribeTasks: dial: %v", err)
	}

	ch := make(chan domain.Task, 64)
	go func() {
		defer close(ch)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var env wsEnvelope
			if json.Unmarshal(msg, &env) != nil {
				continue
			}
			if env.Type != "task" {
				continue
			}
			var t domain.Task
			if json.Unmarshal(env.Data, &t) != nil {
				continue
			}
			select {
			case ch <- t:
			default:
			}
		}
	}()

	return ch, func() { conn.Close() }
}

// SubscribeAgentRuns connects to the agent-runs WS endpoint and returns a
// channel of AgentRun events and a close function.
func (c *WSClient) SubscribeAgentRuns(
	taskID string,
) (<-chan domain.AgentRun, func()) {
	c.t.Helper()
	url := fmt.Sprintf("ws://unix/api/v0/tasks/%s/agent-runs", taskID)
	conn, _, err := c.dialer().Dial(url, nil)
	if err != nil {
		c.t.Fatalf("WSClient.SubscribeAgentRuns: dial: %v", err)
	}

	ch := make(chan domain.AgentRun, 64)
	go func() {
		defer close(ch)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var env wsEnvelope
			if json.Unmarshal(msg, &env) != nil {
				continue
			}
			if env.Type != "agent_run" {
				continue
			}
			var r domain.AgentRun
			if json.Unmarshal(env.Data, &r) != nil {
				continue
			}
			select {
			case ch <- r:
			default:
			}
		}
	}()

	return ch, func() { conn.Close() }
}

// SubscribeKanbanItems connects to the kanban-items WS endpoint and returns a
// channel of KanbanItem events and a close function.
func (c *WSClient) SubscribeKanbanItems(
	taskID string,
) (<-chan domain.KanbanItem, func()) {
	c.t.Helper()
	url := fmt.Sprintf("ws://unix/api/v0/tasks/%s/kanban-items", taskID)
	conn, _, err := c.dialer().Dial(url, nil)
	if err != nil {
		c.t.Fatalf("WSClient.SubscribeKanbanItems: dial: %v", err)
	}

	ch := make(chan domain.KanbanItem, 64)
	go func() {
		defer close(ch)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var env wsEnvelope
			if json.Unmarshal(msg, &env) != nil {
				continue
			}
			if env.Type != "kanban_item" {
				continue
			}
			var item domain.KanbanItem
			if json.Unmarshal(env.Data, &item) != nil {
				continue
			}
			select {
			case ch <- item:
			default:
			}
		}
	}()

	return ch, func() { conn.Close() }
}

// SubscribeThreads connects to the threads WS endpoint and returns a channel
// of ReviewThread events and a close function.
func (c *WSClient) SubscribeThreads(
	taskID string,
) (<-chan domain.ReviewThread, func()) {
	c.t.Helper()
	url := fmt.Sprintf("ws://unix/api/v0/tasks/%s/threads", taskID)
	conn, _, err := c.dialer().Dial(url, nil)
	if err != nil {
		c.t.Fatalf("WSClient.SubscribeThreads: dial: %v", err)
	}

	ch := make(chan domain.ReviewThread, 64)
	go func() {
		defer close(ch)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var env wsEnvelope
			if json.Unmarshal(msg, &env) != nil {
				continue
			}
			if env.Type != "review_thread" {
				continue
			}
			var t domain.ReviewThread
			if json.Unmarshal(env.Data, &t) != nil {
				continue
			}
			select {
			case ch <- t:
			default:
			}
		}
	}()

	return ch, func() { conn.Close() }
}

// ChatConn is a bidirectional WebSocket connection for the chat endpoint.
type ChatConn struct {
	conn   *websocket.Conn
	frames chan agent.ChatFrame
}

// Send sends a user_message frame to the server.
func (c *ChatConn) Send(
	content string,
) error {
	msg, err := json.Marshal(agent.ChatFrame{
		Type:    agent.ChatFrameTypeUserMessage,
		Content: content,
	})
	if err != nil {
		return fmt.Errorf("ChatConn.Send: marshal: %w", err)
	}
	return c.conn.WriteMessage(websocket.TextMessage, msg)
}

// Frames returns the read-only channel of incoming ChatFrames from the server.
func (c *ChatConn) Frames() <-chan agent.ChatFrame {
	return c.frames
}

// ConnectChat opens a bidirectional WebSocket to the chat endpoint and returns
// a ChatConn and a close function.
func (c *WSClient) ConnectChat(
	taskID string,
) (*ChatConn, func()) {
	c.t.Helper()
	url := fmt.Sprintf("ws://unix/api/v0/tasks/%s/chat", taskID)
	conn, _, err := c.dialer().Dial(url, nil)
	if err != nil {
		c.t.Fatalf("WSClient.ConnectChat: dial: %v", err)
	}

	frames := make(chan agent.ChatFrame, 64)
	go func() {
		defer close(frames)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var f agent.ChatFrame
			if json.Unmarshal(msg, &f) != nil {
				continue
			}
			select {
			case frames <- f:
			default:
			}
		}
	}()

	cc := &ChatConn{conn: conn, frames: frames}
	return cc, func() { conn.Close() }
}
