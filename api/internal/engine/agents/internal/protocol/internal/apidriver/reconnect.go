package apidriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/jsonrpc"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Reconnect is how a dialed driver recovers a dropped transport: up to Max
// consecutive redials, waiting Backoff*attempt before each. Sleep is the wait
// (nil: a real timer), so a test can run on its own clock.
type Reconnect struct {
	Max     int
	Backoff time.Duration
	Sleep   func(ctx context.Context, d time.Duration) error
}

// StartDial is Start over a transport dial opens, which it calls again, per rc,
// after the transport drops.
func StartDial(
	ctx context.Context, d *spec.Descriptor, dial func(context.Context) (jsonrpc.Transport, error),
	rc Reconnect, origin SessionOrigin,
) (*Driver, error) {
	t, err := dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("apidriver: %s dial: %w", d.ID, err)
	}
	return newDriver(ctx, d, jsonrpc.NewTransport(t), dial, rc, origin)
}

func (drv *Driver) setInflight(v bool) {
	drv.mu.Lock()
	drv.inflight = v
	drv.mu.Unlock()
}

// reconnect replaces a dropped conn. A turn in flight is failed visibly first
// and never replayed; false ends the driver (no dial, Close, or retries spent).
func (drv *Driver) reconnect(dead wire) (wire, bool) {
	if drv.dial == nil || dead.Overflowed() || drv.ctx.Err() != nil {
		return nil, false
	}
	if err := dead.Close(); err != nil {
		slog.WarnContext(drv.ctx, "apidriver: close dropped transport", "provider", drv.d.ID, "err", err)
	}
	drv.mu.Lock()
	id, lastEvent, interrupted := drv.remembered["session_id"], drv.lastEvent, drv.inflight
	drv.inflight = false
	drv.mu.Unlock()
	slog.WarnContext(drv.ctx, "apidriver: transport dropped; redialing", "provider", drv.d.ID, "turn_interrupted", interrupted)
	if interrupted && !drv.emit(drv.failure(id)) {
		return nil, false
	}
	for attempt := 1; attempt <= drv.rc.Max; attempt++ {
		if err := drv.sleep(time.Duration(attempt) * drv.rc.Backoff); err != nil {
			return nil, false
		}
		conn, err := drv.redial(id, lastEvent)
		if drv.ctx.Err() != nil {
			return nil, false
		}
		if err == nil {
			return conn, true
		}
		slog.WarnContext(drv.ctx, "apidriver: redial failed", "provider", drv.d.ID, "attempt", attempt, "err", err)
	}
	return nil, false
}

func (drv *Driver) sleep(d time.Duration) error {
	if drv.rc.Sleep != nil {
		return drv.rc.Sleep(drv.ctx, d)
	}
	wait, cancel := context.WithTimeout(drv.ctx, d)
	defer cancel()
	<-wait.Done()
	return drv.ctx.Err()
}

// redial dials, handshakes and installs a new conn, then re-enters the session
// the dead one had. A conn that cannot be fully set up is closed again.
func (drv *Driver) redial(sessionID, lastEvent string) (wire, error) {
	t, err := drv.dial(drv.ctx)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	conn := jsonrpc.NewTransport(t)
	if err := handshake(drv.ctx, drv.d, conn); err != nil {
		return nil, fmt.Errorf("%w; close: %w", err, conn.Close())
	}
	drv.cmu.Lock()
	if drv.ctx.Err() != nil {
		drv.cmu.Unlock()
		return nil, fmt.Errorf("driver closed: %w", conn.Close())
	}
	drv.conn = conn
	drv.cmu.Unlock()
	if sessionID == "" {
		return conn, nil
	}
	if err := drv.resumeSession(sessionID, lastEvent); err != nil {
		return nil, errors.Join(err, conn.Close())
	}
	return conn, nil
}

func (drv *Driver) resumeSession(sessionID, lastEvent string) error {
	ev := drv.d.Events[lastEvent]
	if len(ev.Resume) == 0 {
		return fmt.Errorf("apidriver: %s: event %q declares no resume path", drv.d.ID, lastEvent)
	}
	_, err := drv.reestablish(drv.ctx, ev, nil, sessionID)
	return err
}

// emit delivers ev unless the driver closes first.
func (drv *Driver) emit(ev Event) bool {
	select {
	case drv.out <- ev:
		return true
	case <-drv.ctx.Done():
		return false
	}
}

// failure is the turn_failed the dropped turn is reported as. Its payload is
// laid out along the descriptor's own map paths, so the ordinary mapping reads it.
func (drv *Driver) failure(sessionID string) Event {
	values := map[string]string{"reason": "connection lost before the turn finished", "session_id": sessionID}
	doc := map[string]any{}
	if api := drv.d.Events[spec.HookTurnFailed].API; api != nil {
		for field, expr := range api.Map {
			setPath(doc, expr.Literal(), values[field])
		}
	}
	if len(doc) == 0 {
		for k, v := range values {
			doc[k] = v
		}
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		slog.WarnContext(drv.ctx, "apidriver: encode failure event", "provider", drv.d.ID, "err", err)
	}
	return Event{Canonical: spec.HookTurnFailed, Raw: raw}
}

// setPath writes v at a plain dotted path; a path using selector syntax, or a
// blank value, is left out.
func setPath(doc map[string]any, path, v string) {
	if path == "" || v == "" || strings.ContainsAny(path, "[]{}()<>=*|:@#") {
		return
	}
	keys := strings.Split(path, ".")
	for _, k := range keys[:len(keys)-1] {
		next, ok := doc[k].(map[string]any)
		if !ok {
			next = map[string]any{}
			doc[k] = next
		}
		doc = next
	}
	doc[keys[len(keys)-1]] = v
}
