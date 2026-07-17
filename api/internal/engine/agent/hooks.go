package agent

import (
	"fmt"
	"strings"
)

type CanonicalEvent struct {
	Kind      string
	SessionID string
	Message   string
	// AsyncWork is how many units of asynchronous work the CLI reports STILL
	// OUTSTANDING as of this event — a LEVEL it re-states every time, not a delta.
	//
	// It is provider-agnostic by construction: the descriptor names the array whose
	// LENGTH is that level (hooks.events.<kind>.async_work), and a provider that
	// names nothing reports 0 forever. The engine counts; it never learns what the
	// entries mean.
	//
	// A level, because the edges are a lie. Measured against claude 2.1.212, the
	// SubagentStart/SubagentStop pair does NOT balance — the two hooks observe
	// different populations (Start fires only for typed subagents; Stop also fires
	// for anonymous internal ones), so one run gave 4 starts against 9 stops and
	// another gave 3 starts against 0 stops. Counting edges therefore drifts in BOTH
	// directions: too many stops clear the spinner early, too few strand it ON
	// forever. A re-stated level cannot drift, because every report overwrites the
	// last one wholesale and no arithmetic survives between them.
	AsyncWork int
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
	count := func(field string) int {
		path, ok := fields[field]
		if !ok {
			return 0
		}
		return countAt(payload, path)
	}
	return CanonicalEvent{
		Kind:      canonical,
		SessionID: get("session_id"),
		Message:   get("message"),
		AsyncWork: count("async_work"),
		Raw:       payload,
	}, nil
}

// countAt walks a dotted path to an ARRAY and returns its length — the level
// CanonicalEvent.AsyncWork carries. Anything else (missing, not an array, a scalar)
// is 0.
//
// 0 for a path that isn't there is the whole safe-degradation story: a provider that
// maps no async_work, and a CLI too old to send the field, both land here and report
// "nothing outstanding" — which folds Working straight back to the turn alone, i.e.
// exactly the behaviour before this field existed.
func countAt(payload map[string]any, path string) int {
	if path == "" {
		return 0
	}
	var cur any = payload
	for _, p := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return 0
		}
		cur, ok = m[p]
		if !ok {
			return 0
		}
	}
	if arr, ok := cur.([]any); ok {
		return len(arr)
	}
	return 0
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
