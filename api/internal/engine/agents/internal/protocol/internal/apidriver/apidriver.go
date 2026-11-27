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

type Event struct {
	Canonical string
	Raw       []byte
	AskID     json.RawMessage
}

type SessionOrigin func() func(sessionID string)

var ErrSessionLost = errors.New("apidriver: the provider no longer has the session")

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

	cmu  sync.RWMutex
	conn wire

	dial   func(context.Context) (jsonrpc.Transport, error)
	rc     Reconnect
	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.Mutex
	established bool
	remembered  map[string]string
	literals    map[string]bool
	frameValues map[string]frameValue
	birth       map[string]string
	inflight    bool
	lastEvent   string
	lead        string
}

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
		remembered: map[string]string{}, literals: map[string]bool{}, frameValues: map[string]frameValue{}, dial: dial, rc: rc,
	}
	drv.ctx, drv.cancel = context.WithCancel(context.Background())
	go drv.translateLoop(conn)
	return drv, nil
}

func handshake(ctx context.Context, d *spec.Descriptor, conn wire) error {
	if d.Runtime.API.Delivery == spec.APIDeliveryHTTP {
		return nil
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

func (drv *Driver) originate() func(string) {
	if drv.origin == nil {
		return func(string) {}
	}
	return drv.origin()
}

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
			continue
		}
		drv.remember(frame.Method, params)
		primary, ok := dispatch.Resolve(drv.d, frame.Method, params)
		if !ok {
			drv.refuseUnmapped(conn, frame)
			continue
		}
		canonicals := append([]string{primary}, dispatch.Riders(drv.d, frame.Method, params)...)
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
	if overlay := drv.overlay(canonical, frame.Method); len(overlay) > 0 {
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

func (drv *Driver) overlay(canonical, wireMethod string) map[string]any {
	out := drv.capturedValues()
	for k, v := range byWireOverlay(drv.d, canonical, wireMethod) {
		if out == nil {
			out = map[string]any{}
		}
		out[k] = v
	}
	return out
}

func byWireOverlay(d *spec.Descriptor, canonical, wireMethod string) map[string]string {
	ev, ok := d.Events[canonical]
	if !ok || ev.API == nil {
		return nil
	}
	return ev.API.ByWire[wireMethod]
}

func withOverlay(decoded, overlay map[string]any) (json.RawMessage, error) {
	merged := make(map[string]any, len(decoded)+len(overlay))
	for k, v := range decoded {
		merged[k] = v
	}
	for k, v := range overlay {
		merged[k] = v
	}
	return json.Marshal(merged)
}

func (drv *Driver) Events() <-chan Event { return drv.out }

func (drv *Driver) Reply(askID json.RawMessage, rendered []byte) error {
	return drv.current().Reply(askID, json.RawMessage(rendered))
}

func (drv *Driver) Send(ctx context.Context, canonical string, values map[string]string) error {
	drv.mu.Lock()
	merged := cloneValues(drv.remembered)
	merged[callIDKey] = newCallID()
	literals := maps.Clone(drv.literals)
	drv.addFrameValues(merged, literals)
	drv.mu.Unlock()
	for k, v := range values {
		merged[k] = v
		delete(literals, k)
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

func newCallID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (drv *Driver) Overflowed() bool { return drv.current().Overflowed() }

func (drv *Driver) Close() error {
	drv.cancel()
	return drv.current().Close()
}
