// Package browserbridge routes browser tool calls from agents to the desktop
// app, which owns the tabs, and keeps the one authoritative answer to "which
// chat does this tab belong to" that every call is checked against.
package browserbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// callCap bounds any single request no matter what the caller's context says.
const callCap = 60 * time.Second

var (
	// ErrNoDesktop means no desktop app is attached to carry the call out.
	ErrNoDesktop = errors.New(
		"browser: the Crowbar desktop app is not connected; browser tools only work while it is running",
	)
	// ErrUnknownTab is returned for a tab that does not exist AND for one that
	// belongs to another chat, so the answer reveals nothing about either.
	ErrUnknownTab = errors.New("browser: no such tab in this chat; list the tabs with browser_tabs")
)

// Peer is one attached desktop window: the bridge only needs to hand it frames.
type Peer interface {
	Send(frame []byte) error
}

// Bridge is the registry of attached peers, the tabs they report, and the
// requests still waiting for an answer.
type Bridge struct {
	mu      sync.Mutex
	peers   []*Session
	pending map[string]pendingCall
	nextID  atomic.Uint64
}

type pendingCall struct {
	session *Session
	reply   chan reply
}

// deliver never blocks: the first answer wins and a duplicate is dropped.
func (c pendingCall) deliver(r reply) {
	select {
	case c.reply <- r:
	default:
	}
}

type reply struct {
	result json.RawMessage
	err    error
}

// New returns an empty bridge.
func New() *Bridge {
	return &Bridge{pending: map[string]pendingCall{}}
}

// Attach registers a peer. The returned Session is fed the peer's frames and
// must be closed when the peer goes away.
func (b *Bridge) Attach(p Peer) *Session {
	s := &Session{bridge: b, peer: p}
	b.mu.Lock()
	b.peers = append(b.peers, s)
	b.mu.Unlock()
	return s
}

// Tabs returns, as the peer reported them, only the tabs of chat chatID.
func (b *Bridge) Tabs(chatID string) json.RawMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []json.RawMessage{}
	for _, s := range b.peers {
		for _, t := range s.tabs {
			if t.chatID == chatID {
				out = append(out, t.raw)
			}
		}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return json.RawMessage("[]")
	}
	return raw
}

// Do runs op for chat chatID on the desktop. A non-empty tabID must be a tab
// of that same chat: the check is here, not in the peer, so no caller can
// reach a tab outside its own scope however it phrases the id.
func (b *Bridge) Do(
	ctx context.Context,
	chatID, op, tabID string,
	args json.RawMessage,
) (json.RawMessage, error) {
	session, err := b.route(chatID, tabID)
	if err != nil {
		return nil, err
	}
	id := strconv.FormatUint(b.nextID.Add(1), 10)
	frame, err := json.Marshal(map[string]any{
		"type": "request", "id": id, "op": op, "chatId": chatID, "tab": tabID, "args": args,
	})
	if err != nil {
		return nil, fmt.Errorf("browser: encode %s: %w", op, err)
	}
	call := pendingCall{session: session, reply: make(chan reply, 1)}
	b.mu.Lock()
	b.pending[id] = call
	b.mu.Unlock()
	defer b.forget(id)

	if err := session.peer.Send(frame); err != nil {
		return nil, fmt.Errorf("browser: send %s: %w: %w", op, err, ErrNoDesktop)
	}
	ctx, cancel := context.WithTimeout(ctx, callCap)
	defer cancel()
	select {
	case r := <-call.reply:
		return r.result, r.err
	case <-ctx.Done():
		return nil, fmt.Errorf("browser: %s: no answer from the desktop app: %w", op, ctx.Err())
	}
}

func (b *Bridge) route(chatID, tabID string) (*Session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if tabID != "" {
		for _, s := range b.peers {
			if t, ok := s.tabs[tabID]; ok && t.chatID == chatID {
				return s, nil
			}
		}
		return nil, ErrUnknownTab
	}
	if len(b.peers) == 0 {
		return nil, ErrNoDesktop
	}
	return b.peers[len(b.peers)-1], nil
}

func (b *Bridge) forget(id string) {
	b.mu.Lock()
	delete(b.pending, id)
	b.mu.Unlock()
}

func (b *Bridge) settle(s *Session, id string, r reply) {
	b.mu.Lock()
	call, ok := b.pending[id]
	b.mu.Unlock()
	if ok && call.session == s {
		call.deliver(r)
	}
}

type tab struct {
	chatID string
	raw    json.RawMessage
}

// Session is one attached peer's side of the bridge.
type Session struct {
	bridge *Bridge
	peer   Peer
	tabs   map[string]tab // guarded by bridge.mu
}

type frame struct {
	Type   string            `json:"type"`
	ID     string            `json:"id"`
	OK     bool              `json:"ok"`
	Result json.RawMessage   `json:"result"`
	Error  string            `json:"error"`
	Tabs   []json.RawMessage `json:"tabs"`
}

// Receive handles one frame from the peer: a full tab inventory, or the answer
// to a request. Malformed frames are dropped; the peer is not trusted to be
// well-formed and a bad frame must not take the bridge down.
func (s *Session) Receive(raw []byte) {
	var f frame
	if json.Unmarshal(raw, &f) != nil {
		return
	}
	switch f.Type {
	case "tabs":
		s.replaceTabs(f.Tabs)
	case "response":
		if f.OK {
			s.bridge.settle(s, f.ID, reply{result: f.Result})
			return
		}
		s.bridge.settle(s, f.ID, reply{err: fmt.Errorf("browser: %s", f.Error)})
	}
}

func (s *Session) replaceTabs(items []json.RawMessage) {
	next := make(map[string]tab, len(items))
	for _, raw := range items {
		var head struct {
			ID   string `json:"id"`
			WsID string `json:"chatId"`
		}
		if json.Unmarshal(raw, &head) != nil || head.ID == "" || head.WsID == "" {
			continue
		}
		next[head.ID] = tab{chatID: head.WsID, raw: raw}
	}
	s.bridge.mu.Lock()
	s.tabs = next
	s.bridge.mu.Unlock()
}

// Close detaches the peer, forgets its tabs, and fails the requests it still
// owed an answer to.
func (s *Session) Close() {
	b := s.bridge
	b.mu.Lock()
	for i, p := range b.peers {
		if p == s {
			b.peers = append(b.peers[:i], b.peers[i+1:]...)
			break
		}
	}
	s.tabs = nil
	var orphaned []pendingCall
	for _, call := range b.pending {
		if call.session == s {
			orphaned = append(orphaned, call)
		}
	}
	b.mu.Unlock()
	for _, call := range orphaned {
		call.deliver(reply{err: ErrNoDesktop})
	}
}
