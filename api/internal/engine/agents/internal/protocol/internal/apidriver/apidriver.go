// Package apidriver drives one provider's API transport connection (handshake,
// receive loop, reply, session steps) so callers never see jsonrpc or dispatch.
package apidriver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"sync"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/dispatch"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/jsonrpc"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/outbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Event is one canonical event resolved from the wire. AskID is non-nil exactly
// when this is an ask: the caller may Reply to.
type Event struct {
	Canonical string
	Raw       []byte
	AskID     json.RawMessage
}

// SessionOrigin is told about every session this connection opens or enters.
// It is called before the call that does so, and the func it returns takes the
// id once there is one, or "" when nothing came of the attempt.
type SessionOrigin func() func(sessionID string)

// ErrSessionLost is the provider refusing a session as the descriptor's
// session.refused declares: the caller loses it, never this driver replacing
// it with another.
var ErrSessionLost = errors.New("apidriver: the provider no longer has the session")

// wire is a connection as the driver uses it: a JSON-RPC conn, or an http client
// adapted to the same calls (httpwire.go).
type wire interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
	Notify(method string, params any) error
	Reply(id, result json.RawMessage) error
	ReplyError(id json.RawMessage, code int, message string) error
	Frames() <-chan jsonrpc.Frame
	Overflowed() bool
	Close() error
}

type Driver struct {
	d      *spec.Descriptor
	out    chan Event
	origin SessionOrigin

	// conn is replaced by a reconnect; cmu guards the swap against its readers.
	cmu  sync.RWMutex
	conn wire

	// dial is nil for a stdio driver, which never reconnects.
	dial   func(context.Context) (jsonrpc.Transport, error)
	rc     Reconnect
	ctx    context.Context
	cancel context.CancelFunc

	// mu guards the rest. established: this connection ran an event's Fresh or
	// Resume steps (only reopen clears it). remembered: every value a step's
	// capture: pulled out, for a later Send; literals names those that were a JSON
	// number or bool, which a template sends back as such. birth: the first establish's values.
	// inflight: a turn was started and neither stopped nor failed yet; lastEvent
	// is the event a reconnect resumes the session through.
	mu          sync.Mutex
	established bool
	remembered  map[string]string
	literals    map[string]bool
	birth       map[string]string
	inflight    bool
	lastEvent   string
}

// Start runs the descriptor's handshake over serve's stdio and starts
// translating inbound frames; the answered handshake is serve being ready. On
// any failure stdio is closed. origin may be nil (see SessionOrigin).
func Start(
	ctx context.Context, d *spec.Descriptor, stdio jsonrpc.Stdio, origin SessionOrigin,
) (*Driver, error) {
	return newDriver(ctx, d, jsonrpc.New(stdio), nil, Reconnect{}, origin)
}

func newDriver(
	ctx context.Context, d *spec.Descriptor, conn wire,
	dial func(context.Context) (jsonrpc.Transport, error), rc Reconnect, origin SessionOrigin,
) (*Driver, error) {
	if err := handshake(ctx, d, conn); err != nil {
		return nil, errors.Join(err, conn.Close())
	}
	drv := &Driver{
		conn: conn, d: d, out: make(chan Event, 64), origin: origin,
		remembered: map[string]string{}, literals: map[string]bool{}, dial: dial, rc: rc,
	}
	// Detached from ctx: a caller's start deadline must not end a long-lived link.
	drv.ctx, drv.cancel = context.WithCancel(context.Background())
	go drv.translateLoop(conn)
	return drv, nil
}

// handshake runs the descriptor's opening call on a fresh connection.
func handshake(ctx context.Context, d *spec.Descriptor, conn wire) error {
	if d.Runtime.API.Delivery == spec.APIDeliveryHTTP {
		return nil // requests carry no connection to open
	}
	hs := d.Runtime.API.Handshake
	if hs.Call == "" {
		return fmt.Errorf("apidriver: descriptor %s declares no handshake call", d.ID)
	}
	params := hs.Params
	if params == nil {
		params = map[string]any{}
	}
	if _, err := conn.Call(ctx, hs.Call, params); err != nil {
		return fmt.Errorf("apidriver: %s handshake: %w", d.ID, err)
	}
	if hs.Notify != "" {
		if err := conn.Notify(hs.Notify, map[string]any{}); err != nil {
			return fmt.Errorf("apidriver: %s %s: %w", d.ID, hs.Notify, err)
		}
	}
	return nil
}

func (drv *Driver) current() wire {
	drv.cmu.RLock()
	defer drv.cmu.RUnlock()
	return drv.conn
}

// originate claims that this connection is about to produce a session and returns
// the func closing the claim with the resulting id. Claims nest, so a recovery's two
// attempts leave no unclaimed instant for an announcement to slip into.
func (drv *Driver) originate() func(string) {
	if drv.origin == nil {
		return func(string) {}
	}
	return drv.origin()
}

// translateLoop pumps conn until it ends, then redials when this driver may.
func (drv *Driver) translateLoop(conn wire) {
	defer close(drv.out)
	for {
		drv.pump(conn)
		var ok bool
		if conn, ok = drv.reconnect(conn); !ok {
			return
		}
	}
}

func (drv *Driver) pump(conn wire) {
	for frame := range conn.Frames() {
		var params map[string]any
		if err := json.Unmarshal(frame.Params, &params); err != nil {
			continue // a malformed params object is dropped, never fatal
		}
		primary, ok := dispatch.Resolve(drv.d, frame.Method, params)
		if !ok {
			drv.refuseUnmapped(conn, frame)
			continue
		}
		canonicals := append([]string{primary}, dispatch.Riders(drv.d, frame.Method, params)...)
		// An ask is answered once, so only its own event rides the reply id.
		if frame.ID != nil && len(canonicals) > 1 {
			canonicals = canonicals[:1]
		}
		for _, canonical := range canonicals {
			drv.deliver(frame, params, canonical)
		}
	}
}

func (drv *Driver) deliver(frame jsonrpc.Frame, params map[string]any, canonical string) {
	if canonical == spec.HookTurnStop || canonical == spec.HookTurnFailed {
		drv.setInflight(false)
	}
	raw := frame.Params
	// by_wire: literals are merged here because this is the only place the
	// matched wire method is known; downstream sees them as provider fields.
	if overlay := byWireOverlay(drv.d, canonical, frame.Method); len(overlay) > 0 {
		if merged, err := withOverlay(params, overlay); err == nil {
			raw = merged
		}
	}
	askID := frame.ID
	if askID == nil {
		askID = drv.httpAskID(canonical, params)
	}
	drv.out <- Event{Canonical: canonical, Raw: raw, AskID: askID}
}

// byWireOverlay is the literal fields this event's api: block declares for
// wireMethod (spec.ChannelBlock.ByWire); nil in the ordinary case of none.
func byWireOverlay(d *spec.Descriptor, canonical, wireMethod string) map[string]string {
	ev, ok := d.Events[canonical]
	if !ok || ev.API == nil {
		return nil
	}
	return ev.API.ByWire[wireMethod]
}

// withOverlay merges overlay into a copy of decoded and re-marshals; decoded is
// never mutated, so when: matching keeps seeing only what the provider sent.
func withOverlay(decoded map[string]any, overlay map[string]string) (json.RawMessage, error) {
	merged := make(map[string]any, len(decoded)+len(overlay))
	for k, v := range decoded {
		merged[k] = v
	}
	for k, v := range overlay {
		merged[k] = v
	}
	return json.Marshal(merged)
}

// Events delivers every canonical event this driver has resolved, in arrival
// order. Closed when the underlying connection closes.
func (drv *Driver) Events() <-chan Event { return drv.out }

// Reply answers an ask (an Event whose AskID is non-nil) with bytes the caller
// already rendered, keeping per-event rendering out of this package.
func (drv *Driver) Reply(askID json.RawMessage, rendered []byte) error {
	return drv.current().Reply(askID, json.RawMessage(rendered))
}

// Send calls an outbound canonical event (interrupt, compact_start) resolved via
// outbound.Resolve over remembered values, and waits for the reply so a malformed call
// surfaces as an error. Events needing a session or nested payload use Dispatch.
func (drv *Driver) Send(ctx context.Context, canonical string, values map[string]string) error {
	drv.mu.Lock()
	merged := cloneValues(drv.remembered)
	merged[callIDKey] = newCallID()
	literals := maps.Clone(drv.literals)
	drv.mu.Unlock()
	for k, v := range values {
		merged[k] = v
		delete(literals, k) // a caller's value is text, whatever an earlier capture was
	}

	wire, params, ok := outbound.ResolveTyped(drv.d, canonical, merged, literals)
	if !ok {
		return fmt.Errorf("apidriver: %s does not declare outbound event %q", drv.d.ID, canonical)
	}
	_, err := drv.current().Call(ctx, wire, params)
	if err != nil {
		return fmt.Errorf("apidriver: %s: %s: %w", drv.d.ID, wire, err)
	}
	return nil
}

const callIDKey = "call_id"

// newCallID is a fresh 32-hex id per call, so a re-sent call is never mistaken for a repeat.
func newCallID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand never fails on supported platforms
	return hex.EncodeToString(b[:])
}

// Overflowed reports whether the connection closed because its consumer fell
// too far behind.
func (drv *Driver) Overflowed() bool { return drv.current().Overflowed() }

func (drv *Driver) Close() error {
	drv.cancel()
	return drv.current().Close()
}
