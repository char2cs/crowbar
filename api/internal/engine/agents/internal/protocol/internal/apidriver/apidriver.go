// Package apidriver drives one provider's API transport connection: dial,
// handshake, receive loop, and reply — the pieces protocol/protocol.go exposes so
// the runner usecase (which already holds the Turns/answerdesk references this
// needs to feed) never has to know wsrpc or dispatch exist.
package apidriver

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/dispatch"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/outbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/wsrpc"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Event is one canonical event resolved from the wire. AskID is non-nil exactly
// when this is an ask: the caller may Reply to.
type Event struct {
	Canonical string
	Raw       []byte
	AskID     json.RawMessage
}

type Driver struct {
	conn *wsrpc.Conn
	d    *spec.Descriptor
	out  chan Event

	// inject carries synthesized frames — SendPrompt's own echo of the message
	// it just sent, and the thread/read pull-based turn_stop fallback below —
	// back through the SAME handleFrame path a real wire frame takes, so
	// dispatch.Resolve's declarative mapping is the one and only place a
	// canonical event's shape is ever decided. Buffered: a send must not block
	// on translateLoop happening to be parked in select at that exact instant.
	inject chan wsrpc.Frame

	// mu guards turnSettled, touched by translateLoop's goroutine (on a native
	// turn/completed or after a successful thread/read pull) and by
	// pullTurnStop's own goroutine (spawned per idle transition) alike.
	mu          sync.Mutex
	turnSettled map[string]bool
}

// Start dials socketPath, runs the descriptor's declared handshake call, sends
// `initialized` (mirroring the sequence codex's own fixture-capture script uses),
// and starts translating inbound frames into canonical Events.
func Start(ctx context.Context, d *spec.Descriptor, socketPath string) (*Driver, error) {
	conn, err := wsrpc.Dial(ctx, socketPath)
	if err != nil {
		return nil, err
	}
	call := d.Runtime.API.Handshake["call"]
	if call == "" {
		_ = conn.Close()
		return nil, fmt.Errorf("apidriver: descriptor %s declares no handshake call", d.ID)
	}
	if _, err := conn.Call(ctx, call, map[string]any{
		"clientInfo": map[string]string{"name": "crowbar", "title": "Crowbar", "version": "0.0.0"},
	}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("apidriver: %s handshake: %w", d.ID, err)
	}
	if err := conn.Notify("initialized", map[string]any{}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("apidriver: %s initialized: %w", d.ID, err)
	}

	drv := &Driver{
		conn:        conn,
		d:           d,
		out:         make(chan Event, 64),
		inject:      make(chan wsrpc.Frame, 4),
		turnSettled: make(map[string]bool),
	}
	go drv.translateLoop()
	return drv, nil
}

// translateLoop drains both the wire and this driver's own synthesized frames
// through the identical handleFrame path — a synthetic frame is never a
// separate code path, only a second SOURCE of the same Frame shape.
func (drv *Driver) translateLoop() {
	defer close(drv.out)
	frames := drv.conn.Frames()
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				return
			}
			drv.handleFrame(frame)
		case frame := <-drv.inject:
			drv.handleFrame(frame)
		}
	}
}

func (drv *Driver) handleFrame(frame wsrpc.Frame) {
	var params map[string]any
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return // a malformed params object is dropped, never fatal
	}

	// codex's app-server sometimes reports a thread's whole life through this
	// coarse pair alone — item/started, item/agentMessage/delta and
	// turn/completed never arrive at all for it — a variant confirmed live
	// against real codex-cli 0.149.1 traffic and visible on the THREAD ITSELF
	// as historyMode:"paginated" (vs "legacy"). Nothing here declares which
	// variant a given thread will be: it is decided server-side per thread,
	// unrelated to any capability Crowbar negotiates at handshake, so both
	// must always be handled rather than picked between.
	switch frame.Method {
	case "turn/completed":
		drv.markSettled(threadIDOf(params))
	case "thread/status/changed":
		drv.maybePullTurnStop(params)
	}

	canonical, ok := dispatch.Resolve(drv.d, frame.Method, params)
	if !ok {
		return // this provider's descriptor does not map this wire method
	}
	drv.out <- Event{Canonical: canonical, Raw: frame.Params, AskID: frame.ID}
}

func threadIDOf(params map[string]any) string {
	id, _ := params["threadId"].(string)
	return id
}

func (drv *Driver) markSettled(threadID string) {
	if threadID == "" {
		return
	}
	drv.mu.Lock()
	drv.turnSettled[threadID] = true
	drv.mu.Unlock()
}

// maybePullTurnStop reacts to a thread going idle: if turn/completed already
// reported this same turn's end (the "legacy" wire, the common case), the
// native path already forwarded turn_stop and there is nothing to do. If it
// never arrived — the "paginated" wire — this thread's own content is still
// sitting in the rollout codex keeps regardless of which notifications it
// chooses to stream, so a pull fills the gap the push never carried.
func (drv *Driver) maybePullTurnStop(params map[string]any) {
	threadID := threadIDOf(params)
	status, _ := params["status"].(map[string]any)
	statusType, _ := status["type"].(string)
	if threadID == "" || statusType != "idle" {
		return
	}
	drv.mu.Lock()
	settled := drv.turnSettled[threadID]
	drv.mu.Unlock()
	if settled {
		return
	}
	// thread/read is a blocking RPC; running it inline would stall every other
	// frame this connection is delivering (hooks-relayed permission asks
	// included) until it returns.
	go drv.pullTurnStop(threadID)
}

// pullTurnStop fetches threadID's full history and, if turn/completed still
// has not settled it in the meantime (the two paths race harmlessly — only
// one is allowed to win, guarded by the SAME turnSettled map turn/completed
// itself sets), synthesizes a turn_stop-shaped frame from the last recorded
// turn. Its shape — {threadId, turn} — is deliberately identical to
// turn/completed's own params, so codex.yaml's existing turn_stop mapping
// (turn.items[type=agentMessage].text) applies unchanged; this is a second
// SOURCE for that exact shape, not a second mapping to keep in sync.
func (drv *Driver) pullTurnStop(threadID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := drv.conn.Call(ctx, "thread/read", map[string]any{
		"threadId":     threadID,
		"includeTurns": true,
	})
	if err != nil {
		return // best-effort: a truly stuck turn is still caught by the delivery-uncertainty path
	}
	var parsed struct {
		Thread struct {
			Turns []json.RawMessage `json:"turns"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil || len(parsed.Thread.Turns) == 0 {
		return
	}

	drv.mu.Lock()
	already := drv.turnSettled[threadID]
	if !already {
		drv.turnSettled[threadID] = true
	}
	drv.mu.Unlock()
	if already {
		return
	}

	raw, err := json.Marshal(map[string]any{
		"threadId": threadID,
		"turn":     parsed.Thread.Turns[len(parsed.Thread.Turns)-1],
	})
	if err != nil {
		return
	}
	drv.inject <- wsrpc.Frame{Method: "turn/completed", Params: raw}
}

// Events delivers every canonical event this driver has resolved, in arrival
// order. Closed when the underlying connection closes.
func (drv *Driver) Events() <-chan Event { return drv.out }

// Reply answers an ask (an Event whose AskID is non-nil) with already-rendered
// bytes — rendering is the caller's job via the existing, transport-agnostic
// engineagents.Agent.RenderAnswer, so this package stays free of any per-event
// rendering knowledge.
func (drv *Driver) Reply(askID json.RawMessage, rendered []byte) error {
	return drv.conn.Reply(askID, json.RawMessage(rendered))
}

// Send drives an outbound canonical event (prompt, interrupt, compact_start) by
// resolving it through the SAME outbound.Resolve the hooks transport's
// injection-based sends use, then notifying over the socket.
func (drv *Driver) Send(canonical string, values map[string]string) error {
	wire, payload, ok := outbound.Resolve(drv.d, canonical, values)
	if !ok {
		return fmt.Errorf("apidriver: %s does not declare outbound event %q", drv.d.ID, canonical)
	}
	params := make(map[string]any, len(payload))
	for k, v := range payload {
		params[k] = v
	}
	return drv.conn.Notify(wire, params)
}

// SendPrompt delivers one user message over an already-open connection — the
// reason mixed transport needs no restart_tui equivalent at all: unlike every
// hooks-only provider, this socket stays open for the conversation's whole
// life, so a SECOND message reuses the SAME thread rather than spawning
// anything.
//
// codex's app-server has no single "send a message" call, and neither of the
// two it does have fits outbound.Resolve's {field: "{value}"} template: a new
// conversation needs `thread/start` first (its own JSON-RPC REQUEST, answered
// with the new thread's id, which every following `turn/start` must carry —
// there is no implicit "start a thread and a turn together" call), and
// `turn/start`'s `input` is an array of typed content blocks, never a bare
// string. Both are hand-written here, the same way Start's handshake already
// is, rather than descriptor-driven.
//
// threadID empty means "no thread yet" — the conversation's first message —
// and SendPrompt creates one, returning its id so the caller remembers it for
// every later call. A non-empty threadID is reused unchanged, and cwd is used
// only on that first call — an existing thread already has one.
//
// sandbox/approvalPolicy are fixed at workspace-write/on-request, mirroring
// codex.yaml's own `spawn.args` (`--sandbox workspace-write --ask-for-approval
// on-request`) — the PTY-launched settings this connection replaces. Update
// both together if either ever changes.
//
// user_prompt is synthesized here rather than parsed off item/started: in
// "paginated" mode codex never sends that notification at all, and Crowbar
// does not need it to — it is the one party that already knows exactly what
// text was just accepted, the moment this call returns, on EVERY thread
// regardless of which wire variant it turns out to use.
func (drv *Driver) SendPrompt(ctx context.Context, threadID, cwd, text string) (string, error) {
	if threadID == "" {
		result, err := drv.conn.Call(ctx, "thread/start", map[string]any{
			"cwd":            cwd,
			"sandbox":        "workspace-write",
			"approvalPolicy": "on-request",
		})
		if err != nil {
			return "", fmt.Errorf("apidriver: %s: thread/start: %w", drv.d.ID, err)
		}
		var started struct {
			Thread struct {
				ID string `json:"id"`
			} `json:"thread"`
		}
		if err := json.Unmarshal(result, &started); err != nil {
			return "", fmt.Errorf("apidriver: %s: thread/start: parse response: %w", drv.d.ID, err)
		}
		if started.Thread.ID == "" {
			return "", fmt.Errorf("apidriver: %s: thread/start: response carried no thread id", drv.d.ID)
		}
		threadID = started.Thread.ID
	}

	drv.mu.Lock()
	drv.turnSettled[threadID] = false // a new turn is starting; its end has not been observed yet
	drv.mu.Unlock()

	if _, err := drv.conn.Call(ctx, "turn/start", map[string]any{
		"threadId": threadID,
		"input":    []map[string]string{{"type": "text", "text": text}},
	}); err != nil {
		return "", fmt.Errorf("apidriver: %s: turn/start: %w", drv.d.ID, err)
	}

	// Shaped like a real item/started(userMessage) frame so codex.yaml's
	// EXISTING user_prompt mapping (item.content[type=text].text) applies
	// unchanged — this is a second SOURCE for that shape, never a second
	// mapping. A natural item/started(userMessage) may also arrive in legacy
	// mode; ingesting user_prompt twice for one message is a pre-existing,
	// separately-owned concern (durable-dispatch dedup in the runner usecase
	// keys on provider+text, not on frame count), not one this driver invents.
	raw, err := json.Marshal(map[string]any{
		"threadId": threadID,
		"item": map[string]any{
			"type": "userMessage",
			"content": []map[string]any{
				{"type": "text", "text": text},
			},
		},
	})
	if err == nil {
		select {
		case drv.inject <- wsrpc.Frame{Method: "item/started", Params: raw}:
		default:
		}
	}

	return threadID, nil
}

func (drv *Driver) Close() error {
	return drv.conn.Close()
}
