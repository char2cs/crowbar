package agent

import (
	"fmt"
	"strings"
)

type CanonicalEvent struct {
	Kind      string
	SessionID string
	Message   string
	Raw       map[string]any
}

func (d *Descriptor) MapHook(canonical string, payload map[string]any) (CanonicalEvent, error) {
	fields, ok := d.Hooks.Events[canonical]
	if !ok {
		return CanonicalEvent{}, fmt.Errorf("agent: descriptor %q has no hook %q", d.ID, canonical)
	}
	get := func(field string) string {
		path, ok := fields[field]
		if !ok {
			return ""
		}
		return extract(payload, path)
	}
	return CanonicalEvent{
		Kind:      canonical,
		SessionID: get("session_id"),
		Message:   get("message"),
		Raw:       payload,
	}, nil
}

// extract walks a dotted path ("a.b.c") into a decoded payload, returning "" for
// any missing segment or a non-string leaf. A bare key ("session_id") is a
// one-segment path.
func extract(payload map[string]any, path string) string {
	if path == "" {
		return ""
	}
	var cur any = payload
	for _, p := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur, ok = m[p]
		if !ok {
			return ""
		}
	}
	if s, ok := cur.(string); ok {
		return s
	}
	return ""
}
