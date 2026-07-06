package agent

import (
	"fmt"
	"strings"
)

type CanonicalEvent struct {
	Kind       string
	SessionID  string
	Transcript string
	MoveSignal string
	Raw        map[string]any
}

func (d *Descriptor) MapHook(canonical string, payload map[string]any) (CanonicalEvent, error) {
	hm, ok := d.Hooks[canonical]
	if !ok {
		return CanonicalEvent{}, fmt.Errorf("agent: descriptor %q has no hook %q", d.ID, canonical)
	}
	get := func(field string) string {
		path, ok := hm.Fields[field]
		if !ok {
			return ""
		}
		return extract(payload, path)
	}
	return CanonicalEvent{
		Kind:       canonical,
		SessionID:  get("session_id"),
		Transcript: get("transcript"),
		MoveSignal: get("move_signal"),
		Raw:        payload,
	}, nil
}

// extract reads a shallow `$.field` path from the payload (the only shape the
// descriptors use). Returns "" for a missing/non-string value.
func extract(payload map[string]any, path string) string {
	key := strings.TrimPrefix(path, "$.")
	if v, ok := payload[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
