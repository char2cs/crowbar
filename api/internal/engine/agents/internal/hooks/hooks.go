// Package hooks turns a raw provider hook payload into a Crowbar event.
//
// Three things happen here, in this order, and they are deliberately not
// separable by a caller: decode, ownership-check, map. The ownership check is the
// guard that stops a provider's own internal work being filed as the user's
// conversation, and a guard a caller can forget to call is a guard that will
// eventually be forgotten.
package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/payload"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var (
	// ErrUnsupportedFormat reports a descriptor declaring a payload encoding the
	// engine cannot read.
	ErrUnsupportedFormat = errors.New("agents: unsupported hook format")
	// ErrUndeclaredEvent reports a canonical kind this descriptor does not map.
	// Not every provider has every concept, so this is an ordinary outcome the
	// caller drops rather than a failure.
	ErrUndeclaredEvent = errors.New("agents: undeclared hook event")
	// ErrForeignConversation reports a payload that is not the CLI's own
	// user-facing conversation.
	ErrForeignConversation = errors.New("agents: hook does not describe this CLI's own conversation")
)

// ForeignConversationError carries which declared field gave the payload away, so
// the drop can be logged with a reason instead of a shrug.
type ForeignConversationError struct {
	Field string
}

func (e *ForeignConversationError) Error() string {
	return fmt.Sprintf("%s (missing %q)", ErrForeignConversation, e.Field)
}

func (e *ForeignConversationError) Unwrap() error { return ErrForeignConversation }

// Parse decodes raw, rejects payloads that are not this CLI's own conversation,
// and maps the declared fields into a Crowbar event.
func Parse(d *spec.Descriptor, canonical string, raw []byte) (models.CanonicalEvent, error) {
	decoded, err := decode(d, raw)
	if err != nil {
		return models.CanonicalEvent{}, err
	}
	if field, ok := ownsConversation(d, decoded); !ok {
		return models.CanonicalEvent{}, &ForeignConversationError{Field: field}
	}
	fields, declared := d.Hooks.Event(canonical)
	if !declared {
		return models.CanonicalEvent{}, fmt.Errorf("%w: %q on %q", ErrUndeclaredEvent, canonical, d.ID)
	}
	return build(canonical, fields, decoded), nil
}

func decode(d *spec.Descriptor, raw []byte) (map[string]any, error) {
	if d.Hooks.Format != "json" {
		return nil, fmt.Errorf("%w %q on %q", ErrUnsupportedFormat, d.Hooks.Format, d.ID)
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("agents: parse hook payload for %q: %w", d.ID, err)
	}
	return m, nil
}

// ownsConversation reports whether a payload describes the CLI's OWN user-facing
// conversation — the only kind Crowbar hosts.
//
// The rule is entirely the descriptor's: the engine only checks that what the
// provider declared as always-present actually is. Nothing here knows what a
// transcript is, so no provider vocabulary is interpreted.
//
// "Present" means a NON-EMPTY STRING, and it has to: the payload this exists to
// reject spells its absent transcript as an explicit JSON null, which decodes to
// a map entry that IS there. A presence-only check would pass it and silently
// undo the guard.
func ownsConversation(d *spec.Descriptor, decoded map[string]any) (string, bool) {
	for _, field := range d.Hooks.RequirePayloadFields {
		if payload.String(decoded, field) == "" {
			return field, false
		}
	}
	return "", true
}

// build assembles the event for one canonical kind. Every reader is total: a
// field the descriptor did not map, or a provider that stopped sending one, lands
// on the zero value rather than on an error, because a hook must never break the
// vendor CLI's turn.
func build(canonical string, fields map[string]string, decoded map[string]any) models.CanonicalEvent {
	get := func(name string) string { return firstNonEmpty(decoded, fields[name]) }

	ev := models.CanonicalEvent{
		Kind:      canonical,
		SessionID: get("session_id"),
		Message:   get("message"),
		AsyncWork: payload.Count(decoded, fields["async_work"]),
		Model:     get("model"),
		Effort:    get("effort"),
		Reason:    get("reason"),
		Raw:       decoded,
	}
	switch canonical {
	case spec.HookToolPre, spec.HookToolPost, spec.HookToolFail:
		ev.Tool = buildTool(fields, decoded)
	case spec.HookSubagentPre, spec.HookSubagentPost:
		ev.Subagent = &models.SubagentEvent{
			ID:        get("subagent_id"),
			AgentType: get("agent_type"),
		}
	case spec.HookNotification:
		ev.Interrupt = &models.InterruptEvent{Kind: models.InterruptNotification, Detail: ev.Message}
	case spec.HookPermission:
		ev.Interrupt = &models.InterruptEvent{Kind: models.InterruptPermission, Detail: ev.Message}
		ev.Choice = permissionChoice(fields, decoded)
	case spec.HookElicitation:
		ev.Interrupt = &models.InterruptEvent{Kind: models.InterruptElicitation, Detail: ev.Message}
		ev.Choice = elicitationChoice(fields, decoded, ev.Message)
	case spec.HookMessageDelta:
		ev.Delta = buildDelta(fields, decoded)
	case spec.HookTurnFailed:
		ev.Failure = &models.TurnFailure{Reason: get("reason"), Detail: get("detail")}
	case spec.HookCompactPre:
		ev.Interrupt = &models.InterruptEvent{Kind: models.InterruptCompaction, Detail: get("trigger")}
	case spec.HookCompactPost:
		ev.Interrupt = &models.InterruptEvent{
			Kind: models.InterruptCompaction, Detail: get("trigger"), Resolved: true,
		}
	}
	return ev
}

// buildDelta reads one increment of an assistant message.
//
// Index defaults to zero and Final to false when a provider omits them, which is
// the safe direction for both: a missing index reads as the start of a message
// rather than as a gap, and a missing Final leaves the message open rather than
// closing one the provider never said was done.
func buildDelta(fields map[string]string, decoded map[string]any) *models.MessageDelta {
	index, _ := payload.Int(decoded, fields["index"])
	final, _ := payload.Bool(decoded, fields["final"])
	return &models.MessageDelta{
		TurnID:    firstNonEmpty(decoded, fields["turn_id"]),
		MessageID: firstNonEmpty(decoded, fields["message_id"]),
		Index:     index,
		Final:     final,
		Text:      firstNonEmpty(decoded, fields["text"]),
	}
}

func buildTool(fields map[string]string, decoded map[string]any) *models.ToolEvent {
	duration, _ := payload.Int(decoded, fields["duration_ms"])
	return &models.ToolEvent{
		ID:     firstNonEmpty(decoded, fields["tool_id"]),
		Name:   firstNonEmpty(decoded, fields["tool_name"]),
		Target: firstNonEmpty(decoded, fields["tool_target"]),
		Input:  payload.JSON(decoded, fields["tool_input"]),
		// tool_result takes the SAME alternation the target does, and for the same
		// reason: a failing tool reports what happened under a different key from a
		// succeeding one (claude 2.1.234 puts it in `error`, measured 2026-08-17), and
		// the full text belongs in the content store either way.
		Result:     firstNonEmptyJSON(decoded, fields["tool_result"]),
		Error:      firstNonEmpty(decoded, fields["tool_error"]),
		Status:     firstNonEmpty(decoded, fields["tool_status"]),
		DurationMS: duration,
	}
}

// firstNonEmptyJSON is firstNonEmpty over a SUBTREE rather than a string leaf: it
// returns the first path in an alternation whose leaf encodes to anything.
func firstNonEmptyJSON(decoded map[string]any, mapping string) []byte {
	if mapping == "" {
		return nil
	}
	if !strings.Contains(mapping, ",") {
		return payload.JSON(decoded, mapping)
	}
	for _, path := range strings.Split(mapping, ",") {
		if v := payload.JSON(decoded, strings.TrimSpace(path)); len(v) > 0 {
			return v
		}
	}
	return nil
}

// firstNonEmpty reads the first path in a comma-separated ALTERNATION that
// yields a value.
//
// It exists for one honest reason: a single provider concept can live at
// different payload paths depending on which tool fired. Claude's tool target is
// `tool_input.file_path` for a file edit and `tool_input.command` for a shell
// call, and there is no one path that answers both. The alternative — a per-tool
// mapping table in the descriptor — would make the descriptor enumerate a
// provider's tool catalogue, which goes stale on every provider release.
//
// Dotted paths contain no commas, so the separator is unambiguous. A mapping with
// no comma behaves exactly as a plain path.
func firstNonEmpty(decoded map[string]any, mapping string) string {
	if mapping == "" {
		return ""
	}
	if !strings.Contains(mapping, ",") {
		return payload.String(decoded, mapping)
	}
	for _, path := range strings.Split(mapping, ",") {
		if v := payload.String(decoded, strings.TrimSpace(path)); v != "" {
			return v
		}
	}
	return ""
}

// Declared returns the canonical kinds this descriptor maps, sorted, so a caller
// can report what an agent can and cannot observe rather than showing an empty
// panel that looks broken.
func Declared(d *spec.Descriptor) []string {
	if d == nil {
		return nil
	}
	out := make([]string, 0, len(d.Hooks.Events))
	for kind := range d.Hooks.Events {
		out = append(out, kind)
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
