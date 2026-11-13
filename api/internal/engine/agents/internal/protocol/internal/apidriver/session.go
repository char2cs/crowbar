package apidriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/jsonrpc"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/outbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// EstablishSession runs canonical's Fresh (no session id) or Resume steps once per
// connection and returns values merged with the captures. It is separate from
// Dispatch so an attach argv can name the session before anything is said on it.
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
	drv.lastEvent = canonical
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
	drv.literals = map[string]bool{}
	drv.mu.Unlock()

	for k, v := range values {
		out[k] = v
	}
	return out
}

// reestablish re-enters, via the event's Resume steps, a session the provider
// paged out of memory and now refuses.
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

// fillBlanksFromRemembered fills every blank value in out from what this
// connection remembered at establish; a blank caller value never overwrites it.
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
	drv.setInflight(len(ev.Action) > 0)
	err = drv.runSteps(ctx, ev.Action, out)
	if err == nil {
		return out, nil
	}
	drv.setInflight(false)
	if !drv.sessionLost(err) {
		return nil, err
	}
	slog.WarnContext(ctx, "apidriver: session is gone under an established connection; recovering",
		"provider", drv.d.ID, "event", canonical, "lost_session_id", out["session_id"], "err", err)

	recovered, recoverErr := drv.recoverLostSession(ctx, ev, values, out["session_id"])
	if recoverErr != nil {
		return nil, fmt.Errorf("%w (after %w)", recoverErr, err)
	}
	drv.setInflight(len(ev.Action) > 0)
	return recovered, nil
}

// InjectAt runs the inject step declared for lifecycle moment at, if any. A
// context inject runs once, right after a resume loads a session that existed
// before this connection: a fresh one's creating call already carries it.
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
	merged[callIDKey] = newCallID()
	literals := maps.Clone(drv.literals)
	drv.mu.Unlock()
	for k, v := range values {
		merged[k] = v
	}

	payload, _ := outbound.ExpandTree(step.Send, merged, literals).(map[string]any)
	if _, err := drv.current().Call(ctx, step.Call, payload); err != nil {
		return fmt.Errorf("apidriver: %s: %s: %w", drv.d.ID, step.Call, err)
	}
	return nil
}

// runSteps runs one Fresh/Resume/Action list in order, folding each Capture
// into values. A field about to be recaptured is dropped from remembered first:
// a racing Send then fails on an empty id rather than naming a finished turn.
func (drv *Driver) runSteps(ctx context.Context, steps []spec.CallStep, values map[string]string) error {
	for _, step := range steps {
		if len(step.Capture) > 0 {
			drv.mu.Lock()
			for field := range step.Capture {
				delete(drv.remembered, field)
				delete(drv.literals, field)
			}
			drv.mu.Unlock()
		}
		drv.mu.Lock()
		literals := maps.Clone(drv.literals)
		drv.mu.Unlock()
		expand := maps.Clone(values)
		expand[callIDKey] = newCallID()
		payload, _ := outbound.ExpandTree(step.Send, expand, literals).(map[string]any)
		for _, key := range step.Optional {
			if payload[key] == "" {
				delete(payload, key)
			}
		}
		result, err := drv.current().Call(ctx, step.Call, payload)
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
			if v, literal, ok := mapping.Captured(decoded, mapping.Paths(path)); ok {
				values[field] = v
				drv.mu.Lock()
				drv.remembered[field] = v
				if literal {
					drv.literals[field] = true
				}
				drv.mu.Unlock()
			}
		}
	}
	return nil
}

func cloneValues(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}
