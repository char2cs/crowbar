// Package apidriver drives one provider's API transport connection: dial,
// handshake, receive loop, and reply — the pieces protocol/protocol.go exposes so
// the runner usecase (which already holds the Turns/answerdesk references this
// needs to feed) never has to know wsrpc or dispatch exist.
package apidriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
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

	// mu guards established and remembered.
	//
	// established is set once this connection has itself run an event's Fresh
	// or Resume steps successfully — never re-entered on a later Dispatch for
	// the same event, so a second message on an already-live connection goes
	// straight to Action.
	//
	// remembered holds every field ANY step's capture: has ever pulled out of a
	// response on this connection — session_id from thread/start, turn_id from
	// turn/start, whatever a descriptor names — so a LATER Send (which has no
	// step of its own to capture from) can still reference it. A codex
	// interrupt needs the turn/start it never ran itself; this is how it gets
	// it without Crowbar's code knowing turns exist.
	//
	// birth is the value set the FIRST establish on this connection ran with.
	// A rebind (see establishFresh) has to re-run the Fresh steps long after
	// that call returned, and the caller driving it by then is a plain message
	// push carrying only session_id/cwd/text — none of the sandbox, approval
	// policy or handoff context a fresh session's opening call must send. Kept
	// here so the replacement session is born with the SAME settings the
	// original was, instead of a blank-argument thread the provider refuses.
	mu          sync.Mutex
	established bool
	remembered  map[string]string
	birth       map[string]string
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

	drv := &Driver{conn: conn, d: d, out: make(chan Event, 64), remembered: map[string]string{}}
	go drv.translateLoop()
	return drv, nil
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
		drv.out <- Event{Canonical: canonical, Raw: frame.Params, AskID: frame.ID}
	}
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
// remembered (so interrupt's turnId is there without the caller supplying it),
// then calling it over the socket and waiting for the reply — confirmed live
// that codex's own turn/interrupt is a request needing a real response, not a
// fire-and-forget notification: a malformed one comes back a JSON-RPC error,
// not silence. For an event that must first establish a session before
// acting, see Dispatch instead — Send's flat {field: "{value}"} template
// cannot express a typed/nested payload or a response value feeding a later
// call.
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

// EstablishSession runs canonical's Fresh steps (no session id known yet) or
// Resume steps (one is known, e.g. carried over from a prior life of this
// chat, but this connection has never itself loaded it) — whichever applies —
// and is a no-op on every later call once this connection has already
// established a session once. It returns values merged with whatever each
// step's Capture pulled out of its response (a freshly-minted session id,
// most notably), for the caller to keep and to pass to Dispatch.
//
// Split out from Dispatch so a caller can establish a session (and learn its
// id) before anything needs to be SAID on it — attach's argv must name the
// same thread turn/start will act on, and that id has to exist before the
// attach process is even spawned, well before the first message.
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
		// A later caller on an established connection (pushPromptOverAPI, after
		// a switch back to a resumed session) reads session_id from the RUNNER
		// ROW, not this driver — and that row is never rebound on a pure
		// api-transport resume (thread/resume fires no thread/started
		// notification for pumpAPIConn to carry into HandleSessionStart), so it
		// hands back "". Passed straight through before this fix, turn/start
		// got literally session_id="" and codex refused it outright ("invalid
		// thread id: ... found 0"), permanently uncertain-ing every prompt to
		// this runner from then on — confirmed live. Filling any BLANK field
		// from what THIS connection actually remembers (the real thing Fresh or
		// Resume captured at establish) is what Dispatch has always assumed
		// happens here; an empty caller value must lose to it, not overwrite it.
		return fillBlanksFromRemembered(out, remembered), nil
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
	if err := drv.runSteps(ctx, steps, out); err != nil {
		// A resume that fails because the session is GONE is not a fatal error:
		// the id came from a prior life of this chat and the provider has since
		// forgotten it, so no amount of retrying that id can ever work. Left
		// fatal (what this did before), the caller tears the connection down and
		// every later spawn re-runs the same doomed resume against the same dead
		// id forever — the chat can never speak over this transport again.
		// Confirmed live, four times over, in one evening's daemon log.
		if !resuming || !drv.sessionLost(err) {
			return nil, err
		}
		slog.WarnContext(ctx, "apidriver: session is gone; establishing a replacement",
			"provider", drv.d.ID, "event", canonical, "lost_session_id", out["session_id"], "err", err)
		return drv.establishFresh(ctx, ev, values)
	}
	drv.markEstablished(out)
	return out, nil
}

// markEstablished latches this connection as established and remembers the
// session id it settled on.
//
// Resume's own steps declare no capture: (only Fresh's thread/start does) —
// session_id survives Resume's runSteps only because it was already non-empty
// going IN, from the caller's own prior-session value. Remembered explicitly
// here too, or a LATER call on this SAME connection with a blank caller value
// (see EstablishSession's established branch) has nothing to fall back to.
func (drv *Driver) markEstablished(out map[string]string) {
	if out["session_id"] == "" {
		return
	}
	drv.mu.Lock()
	defer drv.mu.Unlock()
	drv.established = true
	drv.remembered["session_id"] = out["session_id"]
}

// sessionLost reports whether err is the provider saying the session id we
// named does not exist any more — matched on the numeric protocol code the
// DESCRIPTOR declares (runtime.api.session_lost_codes), never on the message
// text, which is the provider's own wording and has no business being known
// here. A descriptor that declares no codes never recovers, it just fails.
func (drv *Driver) sessionLost(err error) bool {
	var callErr *wsrpc.CallError
	if !errors.As(err, &callErr) {
		return false
	}
	for _, code := range drv.d.Runtime.API.SessionLostCodes {
		if code == callErr.Code {
			return true
		}
	}
	return false
}

// establishFresh abandons whatever session this connection thought it had and
// mints a genuinely new one by running the SAME Fresh steps a chat's
// first-ever message runs — the one path that cannot depend on the provider
// still remembering anything.
//
// The value set is birth (the settings the original session was born with:
// sandbox, approval policy, handoff context) overlaid with the caller's
// current values (the text actually being sent), minus session_id — blanking
// it is what makes this Fresh rather than another doomed Resume.
//
// remembered is cleared wholesale, not just of session_id: every other field
// in it (turn_id, most of all) was captured from the dead session and would
// otherwise be handed to a Send against the new one, which is a different kind
// of wrong answer than simply not knowing it yet.
func (drv *Driver) establishFresh(
	ctx context.Context, ev spec.EventSpec, values map[string]string,
) (map[string]string, error) {
	drv.mu.Lock()
	out := cloneValues(drv.birth)
	drv.established = false
	drv.remembered = map[string]string{}
	drv.mu.Unlock()

	for k, v := range values {
		out[k] = v
	}
	out["session_id"] = ""
	if err := drv.runSteps(ctx, ev.Fresh, out); err != nil {
		return nil, fmt.Errorf("apidriver: %s: establish replacement session: %w", drv.d.ID, err)
	}
	drv.markEstablished(out)
	return out, nil
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

// Dispatch establishes canonical's session if this connection has not already
// (see EstablishSession), then runs its Action steps — the actual "do the
// thing" call (turn/start), always last, always run.
//
// The established latch is a claim about THIS connection's own history, never
// a fact about the provider: it is set once and nothing on the wire can clear
// it. So a session that dies after it was set leaves every later Action
// naming a thread that is gone, forever — the message is refused, the caller
// records the delivery as uncertain, and the chat wedges with nothing
// streaming and no way back. An Action refused for exactly that reason is
// therefore retried ONCE against a replacement session, which is the only
// outcome that is not either a lie or a dead end.
func (drv *Driver) Dispatch(
	ctx context.Context, canonical string, values map[string]string,
) (map[string]string, error) {
	out, err := drv.EstablishSession(ctx, canonical, values)
	if err != nil {
		return nil, err
	}
	action := drv.d.Events[canonical].Action
	err = drv.runSteps(ctx, action, out)
	if err == nil {
		return out, nil
	}
	if !drv.sessionLost(err) {
		return nil, err
	}
	slog.WarnContext(ctx, "apidriver: session is gone under an established connection; rebinding",
		"provider", drv.d.ID, "event", canonical, "lost_session_id", out["session_id"], "err", err)

	rebound, rebindErr := drv.establishFresh(ctx, drv.d.Events[canonical], values)
	if rebindErr != nil {
		return nil, fmt.Errorf("%w (after %v)", rebindErr, err)
	}
	if err := drv.runSteps(ctx, action, rebound); err != nil {
		return nil, err
	}
	return rebound, nil
}

// InjectAt runs the descriptor's inject step declared for lifecycle moment at
// (spec.InjectSpec's "config | mcp | context | resume"), if it has one — a
// descriptor with nothing declared for at is a no-op, not an error, the same
// declarative-capability shape ContextSteps already has (a provider whose
// resume channel is config, not a live connection, has no use for this).
//
// Unlike EstablishSession/Dispatch's Fresh/Resume/Action lists (bound to a
// canonical EVENT, always run in the same place in that event's lifecycle),
// an inject step is bound to a lifecycle MOMENT the caller decides on: codex's
// own thread/inject_items (at: context) has to run once, right after a RESUME
// establishes a session that existed before this connection loaded it — never
// on a fresh thread/start, which already carries {context} as
// developerInstructions in the very call that creates the thread.
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
func (drv *Driver) runSteps(ctx context.Context, steps []spec.CallStep, values map[string]string) error {
	for _, step := range steps {
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
			if v, ok := mapping.Scalar(decoded, path); ok {
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

func (drv *Driver) Close() error {
	return drv.conn.Close()
}
