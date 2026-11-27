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
	if !resuming {
		drv.armLead(out)
	}
	return out, nil
}

func (drv *Driver) markEstablished(out map[string]string) {
	if out["session_id"] == "" {
		return
	}
	drv.mu.Lock()
	defer drv.mu.Unlock()
	drv.established = true
	drv.remembered["session_id"] = out["session_id"]
}

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

func (drv *Driver) reopen(values map[string]string) map[string]string {
	drv.mu.Lock()
	out := cloneValues(drv.birth)
	drv.established = false
	drv.remembered = map[string]string{}
	drv.literals = map[string]bool{}
	drv.frameValues = map[string]frameValue{}
	drv.mu.Unlock()

	for k, v := range values {
		out[k] = v
	}
	return out
}

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

func (drv *Driver) Dispatch(
	ctx context.Context, canonical string, values map[string]string,
) (map[string]string, error) {
	out, err := drv.EstablishSession(ctx, canonical, values)
	if err != nil {
		return nil, err
	}
	ev := drv.d.Events[canonical]
	drv.setInflight(len(ev.Action) > 0)
	values, lead := drv.leadValues(values)
	if lead != "" {
		out["text"] = values["text"]
	}
	err = drv.runSteps(ctx, ev.Action, out)
	if err == nil {
		drv.spendLead(lead)
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
	drv.spendLead(lead)
	return recovered, nil
}

func (drv *Driver) InjectAt(ctx context.Context, at string, values map[string]string) error {
	step := drv.injectAt(at)
	if step != nil && step.PromptLead != "" {
		drv.armLead(values)
		return nil
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

func (drv *Driver) runSteps(ctx context.Context, steps []spec.CallStep, values map[string]string) error {
	for _, step := range steps {
		prior := drv.withdraw(step.Capture)
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
			v, literal, ok := mapping.Captured(decoded, mapping.Paths(path))
			drv.mu.Lock()
			switch {
			case ok:
				values[field] = v
				drv.remembered[field] = v
				if literal {
					drv.literals[field] = true
				}
			case prior[field].held:
				drv.remembered[field] = prior[field].value
				drv.literals[field] = prior[field].literal
			}
			drv.mu.Unlock()
		}
	}
	return nil
}

type heldValue struct {
	value         string
	literal, held bool
}

func (drv *Driver) withdraw(capture map[string]string) map[string]heldValue {
	if len(capture) == 0 {
		return nil
	}
	drv.mu.Lock()
	defer drv.mu.Unlock()
	prior := make(map[string]heldValue, len(capture))
	for field := range capture {
		v, held := drv.remembered[field]
		prior[field] = heldValue{value: v, literal: drv.literals[field], held: held}
		delete(drv.remembered, field)
		delete(drv.literals, field)
	}
	return prior
}

func cloneValues(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}
