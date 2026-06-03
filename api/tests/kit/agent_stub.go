//go:build integration || manual

package kit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
	"github.com/google/uuid"
)

// stubSession represents a single controllable agent run session.
type stubSession struct {
	token   string
	publish func(agent.ChatFrame)
	done    chan struct{}
	fail    chan struct{}
}

// AgentStub is a controllable implementation of engine/agent.AgentRuntime.
// It blocks every Run call until the test explicitly fires a signal, streams
// chunks, or calls Fail. WaitForRun returns the session token so the test can
// use it to call MCP tools via MCPClient.WithToken.
type AgentStub struct {
	mu       sync.Mutex
	sessions map[string]*stubSession // keyed by token
	active   *stubSession            // most recently started session
	waiting  chan string              // sends token when a session starts
	chatHub  hub.ChatHub             // optional; wired via SetChatHub after construction
}

// NewAgentStub returns a ready-to-use AgentStub.
func NewAgentStub() *AgentStub {
	return &AgentStub{
		sessions: make(map[string]*stubSession),
		waiting:  make(chan string, 1),
	}
}

// SetChatHub wires the chat hub into the stub so that StreamChunks delivers
// frames to subscribed WebSocket clients. Call this after the app container is
// created. If SetChatHub is not called, StreamChunks is a no-op.
func (s *AgentStub) SetChatHub(h hub.ChatHub) {
	s.mu.Lock()
	s.chatHub = h
	s.mu.Unlock()
}

// Run implements agent.AgentRuntime. It registers the session under the token
// from the provided AgentRun (so the test can use it with MCPClient.WithToken),
// signals the waiting channel, and blocks until the session is told to complete,
// fail, or the context is cancelled.
func (s *AgentStub) Run(
	ctx context.Context,
	run domain.AgentRun,
	task domain.Task,
	state flow.StateDefinition,
) error {
	token := run.Token
	if token == "" {
		token = uuid.NewString()
	}

	// Wire the publish function from the chat hub when available so that
	// StreamChunks delivers frames to subscribed WebSocket clients.
	s.mu.Lock()
	h := s.chatHub
	s.mu.Unlock()

	var pub func(agent.ChatFrame)
	if h != nil {
		_, publish, unregister := h.RegisterSession(task.ID)
		defer unregister()
		pub = publish
	} else {
		pub = func(agent.ChatFrame) {}
	}

	sess := &stubSession{
		token:   token,
		publish: pub,
		done:    make(chan struct{}),
		fail:    make(chan struct{}),
	}

	s.mu.Lock()
	s.sessions[token] = sess
	s.active = sess
	s.mu.Unlock()

	// Notify any WaitForRun caller.
	select {
	case s.waiting <- token:
	default:
	}

	// Block until done, fail, or ctx cancelled.
	select {
	case <-sess.done:
		return nil
	case <-sess.fail:
		return errors.New("agent stub: simulated failure")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WaitForRun blocks until an agent run session starts and returns its token.
// It calls t.Fatal if no session starts within 2 seconds.
func (s *AgentStub) WaitForRun(
	ctx context.Context,
) string {
	select {
	case token := <-s.waiting:
		return token
	case <-time.After(2 * time.Second):
		panic("AgentStub.WaitForRun: timeout waiting for agent run to start")
	case <-ctx.Done():
		panic(fmt.Sprintf("AgentStub.WaitForRun: context cancelled: %v", ctx.Err()))
	}
}

// StreamChunks sends a series of agent_chunk frames followed by
// agent_turn_end to the active session's publish function.
func (s *AgentStub) StreamChunks(
	chunks ...string,
) {
	s.mu.Lock()
	sess := s.active
	s.mu.Unlock()
	if sess == nil {
		return
	}
	for _, chunk := range chunks {
		sess.publish(agent.ChatFrame{
			Type:  agent.ChatFrameTypeAgentChunk,
			Delta: chunk,
		})
	}
	sess.publish(agent.ChatFrame{
		Type: agent.ChatFrameTypeAgentTurnEnd,
	})
}

// Complete signals the active session to exit successfully (simulates agent
// completing its work without firing a crowbar_signal).
func (s *AgentStub) Complete() {
	s.mu.Lock()
	sess := s.active
	s.mu.Unlock()
	if sess == nil {
		return
	}
	close(sess.done)
}

// Fail signals the active session to exit with an error, simulating a crash.
func (s *AgentStub) Fail() {
	s.mu.Lock()
	sess := s.active
	s.mu.Unlock()
	if sess == nil {
		return
	}
	select {
	case <-sess.fail:
	default:
		close(sess.fail)
	}
}

// SetPublish replaces the publish function of the session identified by token.
// Used by the chat hub integration to wire real frame delivery.
func (s *AgentStub) SetPublish(
	token string,
	publish func(agent.ChatFrame),
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[token]; ok {
		sess.publish = publish
	}
}

// Ensure AgentStub implements agent.AgentRuntime at compile time.
var _ agent.AgentRuntime = (*AgentStub)(nil)

// testingFatal is a small helper so WaitForRun can call t.Fatal without
// importing testing directly (the *testing.T is not stored on the stub).
func testingFatal(t interface{ Helper(); Fatal(...any) }, args ...any) {
	t.Helper()
	t.Fatal(args...)
}

// WaitForRunT is a convenience wrapper that calls t.Fatal on timeout instead
// of panicking. Use this variant when a *testing.T is available.
func (s *AgentStub) WaitForRunT(
	t *testing.T,
	ctx context.Context,
) string {
	t.Helper()
	select {
	case token := <-s.waiting:
		return token
	case <-time.After(2 * time.Second):
		t.Fatal("AgentStub.WaitForRunT: timeout waiting for agent run to start")
		return ""
	case <-ctx.Done():
		t.Fatalf("AgentStub.WaitForRunT: context cancelled: %v", ctx.Err())
		return ""
	}
}
