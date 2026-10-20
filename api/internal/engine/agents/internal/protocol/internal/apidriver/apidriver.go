// Package apidriver drives one provider's API transport connection: handshake,
// receive loop, and reply — the pieces protocol/protocol.go exposes so the
// runner usecase (which already holds the Turns/answerdesk references this
// needs to feed) never has to know jsonrpc or dispatch exist.
package apidriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
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

type Driver struct {
	conn   *jsonrpc.Conn
	d      *spec.Descriptor
	out    chan Event
	origin SessionOrigin

	// mu guards established and remembered.
	//
	// established is set once this connection has itself run an event's Fresh
	// or Resume steps successfully, so a second message on an already-live
	// connection goes straight to Action. Nothing ON THE WIRE clears it; only
	// reopen does, and only when a lost session is being recovered.
	//
	// remembered holds every field ANY step's capture: has ever pulled out of a
	// response on this connection — session_id from a session-creating call,
	// turn_id from a turn call, whatever a descriptor names — so a LATER Send
	// (which has no step of its own to capture from) can still reference it. An
	// interrupt needs the turn id of a call it never made itself; this is how
	// it gets it without Crowbar's code knowing turns exist.
	//
	// birth is the value set the FIRST establish on this connection ran with.
	// A re-entry (reestablish) re-runs the Resume steps long after that call
	// returned, from a plain message push carrying only session_id/cwd/text, so
	// it is re-entered with the settings the session was opened with.
	mu          sync.Mutex
	established bool
	remembered  map[string]string
	birth       map[string]string
}

// Start speaks over serve's stdio, runs the descriptor's declared handshake
// call, sends `initialized`, and starts translating inbound frames into
// canonical Events. The answered handshake is serve being ready; on any
// failure stdio is closed.
//
// origin may be nil for a caller that does not care which sessions this
// connection produced itself — see SessionOrigin for who does.
func Start(
	ctx context.Context, d *spec.Descriptor, stdio jsonrpc.Stdio, origin SessionOrigin,
) (*Driver, error) {
	call := d.Runtime.API.Handshake["call"]
	if call == "" {
		return nil, errors.Join(fmt.Errorf("apidriver: descriptor %s declares no handshake call", d.ID), stdio.Close())
	}
	conn := jsonrpc.New(stdio)
	if _, err := conn.Call(ctx, call, map[string]any{
		"clientInfo": map[string]string{"name": "crowbar", "title": "Crowbar", "version": "0.0.0"},
	}); err != nil {
		return nil, errors.Join(fmt.Errorf("apidriver: %s handshake: %w", d.ID, err), conn.Close())
	}
	if err := conn.Notify("initialized", map[string]any{}); err != nil {
		return nil, errors.Join(fmt.Errorf("apidriver: %s initialized: %w", d.ID, err), conn.Close())
	}

	drv := &Driver{
		conn: conn, d: d, out: make(chan Event, 64), origin: origin,
		remembered: map[string]string{},
	}
	go drv.translateLoop()
	return drv, nil
}

// originate opens a claim that this connection is about to produce a session of
// its own, and returns the func that closes it with whatever id came of the
// attempt. Claims nest: a recovery holds one across both of its attempts, so
// the handover from one to the other leaves no unclaimed instant for an
// announcement to arrive in.
func (drv *Driver) originate() func(string) {
	if drv.origin == nil {
		return func(string) {}
	}
	return drv.origin()
}

func (drv *Driver) translateLoop() {
	defer close(drv.out)
	for frame := range drv.conn.Frames() {
		var params map[string]any
		if err := json.Unmarshal(frame.Params, &params); err != nil {
			continue // a malformed params object is dropped, never fatal
		}
		canonical, ok := dispatch.Resolve(drv.d, frame.Method, params)
		if !ok {
			continue // this provider's descriptor does not map this wire method
		}
		raw := frame.Params
		// by_wire: (design spec F1) — a literal field the matched wire method
		// itself names, merged in BEFORE the event reaches map: resolution.
		// This is the only place the wire method that actually matched is
		// known at all; downstream (inbound.Parse) only ever sees the
		// resulting bytes, no different from a field the provider sent itself.
		if overlay := byWireOverlay(drv.d, canonical, frame.Method); len(overlay) > 0 {
			if merged, err := withOverlay(params, overlay); err == nil {
				raw = merged
			}
		}
		drv.out <- Event{Canonical: canonical, Raw: raw, AskID: frame.ID}
	}
}

// byWireOverlay is the literal synthetic fields this event's api: block
// declares for wireMethod (spec.ChannelBlock.ByWire) — see that field's own
// doc comment. Nil for a legacy event, or a channel-scoped one that declares
// none for this wire method: the ordinary case, where map: reads the
// provider's own payload unchanged.
func byWireOverlay(d *spec.Descriptor, canonical, wireMethod string) map[string]string {
	ev, ok := d.Events[canonical]
	if !ok || ev.API == nil {
		return nil
	}
	return ev.API.ByWire[wireMethod]
}

// withOverlay merges overlay's literal fields into decoded (already-parsed
// params) and re-marshals. decoded is copied, never mutated in place — a
// concurrent when: match against the original params map must keep seeing
// only what the provider actually sent.
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

// Reply answers an ask (an Event whose AskID is non-nil) with already-rendered
// bytes — rendering is the caller's job via the existing, transport-agnostic
// engineagents.Agent.RenderAnswer, so this package stays free of any per-event
// rendering knowledge.
func (drv *Driver) Reply(askID json.RawMessage, rendered []byte) error {
	return drv.conn.Reply(askID, json.RawMessage(rendered))
}

// Send drives an outbound canonical event (interrupt, compact_start) by
// resolving it through the SAME outbound.Resolve the hooks transport's
// injection-based sends use, merged over every value this connection has ever
// remembered (so interrupt's turn id is there without the caller supplying it),
// then calling it over the connection and waiting for the reply — an
// interrupt is a request needing a real response, not a fire-and-forget
// notification: a malformed one comes back a JSON-RPC error, not silence.
// For an event that must first establish a session before acting, see
// Dispatch instead — Send's flat {field: "{value}"} template cannot express a
// typed/nested payload or a response value feeding a later call.
func (drv *Driver) Send(ctx context.Context, canonical string, values map[string]string) error {
	drv.mu.Lock()
	merged := cloneValues(drv.remembered)
	drv.mu.Unlock()
	for k, v := range values {
		merged[k] = v
	}

	wire, payload, ok := outbound.Resolve(drv.d, canonical, merged)
	if !ok {
		return fmt.Errorf("apidriver: %s does not declare outbound event %q", drv.d.ID, canonical)
	}
	params := make(map[string]any, len(payload))
	for k, v := range payload {
		params[k] = v
	}
	_, err := drv.conn.Call(ctx, wire, params)
	if err != nil {
		return fmt.Errorf("apidriver: %s: %s: %w", drv.d.ID, wire, err)
	}
	return nil
}

// EstablishSession runs canonical's Fresh steps (no session id yet) or Resume
// steps, once per connection, and returns values merged with what each step
// captured. It is apart from Dispatch so an attach argv can name the session
// before anything is said on it.
func (drv *Driver) EstablishSession(
	ctx context.Context, canonical string, values map[string]string,
) (map[string]string, error) {
	ev, declared := drv.d.Events[canonical]
	if !declared {
		return nil, fmt.Errorf("apidriver: %s does not declare event %q", drv.d.ID, canonical)
	}
	out := cloneValues(values)

	drv.mu.Lock()
	already := drv.established
	remembered := cloneValues(drv.remembered)
	drv.mu.Unlock()
	if already {
		// A later caller may pass blank values (a pure api resume announces no
		// session), so blanks are filled from what this connection established;
		// its own session wins over whatever the caller named.
		out = fillBlanksFromRemembered(out, remembered)
		if id := remembered["session_id"]; id != "" {
			out["session_id"] = id
		}
		return out, nil
	}

	drv.mu.Lock()
	if drv.birth == nil {
		drv.birth = cloneValues(values)
	}
	drv.mu.Unlock()

	resuming := out["session_id"] != ""
	steps := ev.Resume
	if !resuming {
		steps = ev.Fresh
	}
	// Every session this connection opens is reported, so the chat binds the
	// one its ingress filters against.
	settle := drv.originate()
	if err := drv.runSteps(ctx, steps, out); err != nil {
		settle("")
		if resuming && drv.sessionLost(err) {
			return nil, fmt.Errorf("%w: %s: %w", ErrSessionLost, out["session_id"], err)
		}
		return nil, err
	}
	drv.markEstablished(out)
	settle(out["session_id"])
	return out, nil
}

// markEstablished latches this connection as established and remembers its
// session id. Resume captures none, so the id is kept from its input, or a later
// blank caller (EstablishSession's established branch) has nothing to fall back to.
func (drv *Driver) markEstablished(out map[string]string) {
	if out["session_id"] == "" {
		return
	}
	drv.mu.Lock()
	defer drv.mu.Unlock()
	drv.established = true
	drv.remembered["session_id"] = out["session_id"]
}

// sessionLost reports whether err is the provider saying it does not have the
// session we named: the descriptor's session.refused code AND one of its lines,
// since a code alone is often the protocol's generic "invalid request".
func (drv *Driver) sessionLost(err error) bool {
	var callErr *jsonrpc.CallError
	r := drv.d.Session.Refused
	if !errors.As(err, &callErr) || r == nil || r.RPCCode == 0 || callErr.Code != r.RPCCode {
		return false
	}
	return slices.ContainsFunc(r.Needles, func(needle string) bool {
		return needle != "" && strings.Contains(callErr.Message, needle)
	})
}

// reopen is the value set a re-establish runs with: the session's birth values
// overlaid with the caller's. It clears established and all of remembered (a
// turn_id among them), which belong to the session being left.
func (drv *Driver) reopen(values map[string]string) map[string]string {
	drv.mu.Lock()
	out := cloneValues(drv.birth)
	drv.established = false
	drv.remembered = map[string]string{}
	drv.mu.Unlock()

	for k, v := range values {
		out[k] = v
	}
	return out
}

// reestablish re-enters a session the provider says it no longer has through
// the event's declared Resume steps: a provider pages an idle session out of
// memory, which is what a resume is for.
func (drv *Driver) reestablish(
	ctx context.Context, ev spec.EventSpec, values map[string]string, sessionID string,
) (map[string]string, error) {
	settle := drv.originate()
	out := drv.reopen(values)
	out["session_id"] = sessionID
	if err := drv.runSteps(ctx, ev.Resume, out); err != nil {
		settle("")
		return nil, fmt.Errorf("apidriver: %s: re-enter session: %w", drv.d.ID, err)
	}
	drv.markEstablished(out)
	settle(out["session_id"])
	return out, nil
}

// recoverLostSession answers an Action the provider refused because it does not
// have the session: re-enter it and re-run the Action, so the message still
// lands. A session the provider refuses to resume as well is lost.
func (drv *Driver) recoverLostSession(
	ctx context.Context, ev spec.EventSpec, values map[string]string, lostSessionID string,
) (map[string]string, error) {
	if lostSessionID == "" || len(ev.Resume) == 0 {
		return nil, fmt.Errorf("%w: %s: no declared resume path", ErrSessionLost, lostSessionID)
	}
	out, err := drv.reestablish(ctx, ev, values, lostSessionID)
	if err != nil {
		if drv.sessionLost(err) {
			return nil, fmt.Errorf("%w: %s: %w", ErrSessionLost, lostSessionID, err)
		}
		return nil, err
	}
	return out, drv.runSteps(ctx, ev.Action, out)
}

// fillBlanksFromRemembered overwrites every blank value in out with what this
// connection actually remembers from its own establish — see
// EstablishSession's already-established branch for why an empty caller
// value must lose to it, not overwrite it.
func fillBlanksFromRemembered(out, remembered map[string]string) map[string]string {
	for k, v := range out {
		if v != "" {
			continue
		}
		if rv, ok := remembered[k]; ok {
			out[k] = rv
		}
	}
	return out
}

// Dispatch establishes canonical's session if this connection has not, then runs
// its Action steps. The established latch never clears itself, so an Action
// refused because the session is gone is recovered once (recoverLostSession).
func (drv *Driver) Dispatch(
	ctx context.Context, canonical string, values map[string]string,
) (map[string]string, error) {
	out, err := drv.EstablishSession(ctx, canonical, values)
	if err != nil {
		return nil, err
	}
	ev := drv.d.Events[canonical]
	err = drv.runSteps(ctx, ev.Action, out)
	if err == nil {
		return out, nil
	}
	if !drv.sessionLost(err) {
		return nil, err
	}
	slog.WarnContext(ctx, "apidriver: session is gone under an established connection; recovering",
		"provider", drv.d.ID, "event", canonical, "lost_session_id", out["session_id"], "err", err)

	recovered, recoverErr := drv.recoverLostSession(ctx, ev, values, out["session_id"])
	if recoverErr != nil {
		return nil, fmt.Errorf("%w (after %v)", recoverErr, err)
	}
	return recovered, nil
}

// InjectAt runs the descriptor's inject step declared for lifecycle moment at
// (spec.InjectSpec's "config | mcp | context | resume"), if it has one — a
// descriptor with nothing declared for at is a no-op, not an error, the same
// declarative-capability shape ContextSteps already has (a provider whose
// resume channel is config, not a live connection, has no use for this).
//
// Unlike EstablishSession/Dispatch's Fresh/Resume/Action lists (bound to a
// canonical EVENT, always run in the same place in that event's lifecycle),
// an inject step is bound to a lifecycle MOMENT the caller decides on: a
// context inject (at: context) has to run once, right after a RESUME
// establishes a session that existed before this connection loaded it — never
// on a fresh session, whose creating call already carries {context} in the
// very call that creates it.
func (drv *Driver) InjectAt(ctx context.Context, at string, values map[string]string) error {
	var step *spec.InjectSpec
	for i := range drv.d.Inject {
		if drv.d.Inject[i].At == at {
			step = &drv.d.Inject[i]
			break
		}
	}
	if step == nil || step.Call == "" {
		return nil
	}

	drv.mu.Lock()
	merged := cloneValues(drv.remembered)
	drv.mu.Unlock()
	for k, v := range values {
		merged[k] = v
	}

	payload, _ := expandTree(step.Send, merged).(map[string]any)
	if _, err := drv.conn.Call(ctx, step.Call, payload); err != nil {
		return fmt.Errorf("apidriver: %s: %s: %w", drv.d.ID, step.Call, err)
	}
	return nil
}

// runSteps executes one Fresh/Resume/Action list in order: expand Send's
// template tree against values, call it, and fold any Capture into values so
// a later step in the SAME list (or the caller) can read it.
//
// Any field this step is ABOUT to (re)capture is dropped from remembered
// first, before the call goes out — never left standing until the reply
// lands. Send (interrupt, compact_start) reads remembered concurrently with
// this, on a caller's own goroutine, and turn_id is exactly the field a
// second turn call overwrites: leaving the PREVIOUS turn's id in place for
// the whole round trip means a Stop racing in while this call is still in
// flight would ship an interrupt naming a turn that already ended — not
// loudly rejected (that's the safe, already-handled empty-field case), but
// silently pointed at the wrong turn while the real one keeps generating.
// Clearing here trades that silent miss for the safe one: an empty field
// fails the provider's own validation and StopChat falls back to a full stop.
func (drv *Driver) runSteps(ctx context.Context, steps []spec.CallStep, values map[string]string) error {
	for _, step := range steps {
		if len(step.Capture) > 0 {
			drv.mu.Lock()
			for field := range step.Capture {
				delete(drv.remembered, field)
			}
			drv.mu.Unlock()
		}
		payload, _ := expandTree(step.Send, values).(map[string]any)
		result, err := drv.conn.Call(ctx, step.Call, payload)
		if err != nil {
			return fmt.Errorf("apidriver: %s: %s: %w", drv.d.ID, step.Call, err)
		}
		if len(step.Capture) == 0 {
			continue
		}
		var decoded map[string]any
		if err := json.Unmarshal(result, &decoded); err != nil {
			return fmt.Errorf("apidriver: %s: %s: parse response: %w", drv.d.ID, step.Call, err)
		}
		for field, path := range step.Capture {
			if v, ok := mapping.Scalar(decoded, mapping.Paths(path)); ok {
				values[field] = v
				drv.mu.Lock()
				drv.remembered[field] = v
				drv.mu.Unlock()
			}
		}
	}
	return nil
}

// expandTree walks a Send tree (whatever shape the descriptor's YAML gave it)
// and substitutes {placeholder} at every string leaf via the SAME
// outbound.Substitute the flat Send/Notify path already uses — one
// {placeholder} grammar, applied at every depth instead of only the top one,
// which is the whole of what a typed/nested payload needed over the old flat
// map[string]string.
func expandTree(node any, values map[string]string) any {
	switch v := node.(type) {
	case string:
		return outbound.Substitute(v, values)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			out[k] = expandTree(val, values)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, val := range v {
			out[i] = expandTree(val, values)
		}
		return out
	default:
		return v
	}
}

func cloneValues(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}

// Overflowed reports whether the connection closed because its consumer fell
// too far behind.
func (drv *Driver) Overflowed() bool { return drv.conn.Overflowed() }

func (drv *Driver) Close() error {
	return drv.conn.Close()
}
