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
	ErrUnsupportedFormat = errors.New("agents: unsupported hook format")

	ErrUndeclaredEvent = errors.New("agents: undeclared hook event")

	ErrForeignConversation = errors.New("agents: hook does not describe this CLI's own conversation")
)

type ForeignConversationError struct {
	Field string
}

func (e *ForeignConversationError) Error() string {
	return fmt.Sprintf("%s (missing %q)", ErrForeignConversation, e.Field)
}

func (e *ForeignConversationError) Unwrap() error { return ErrForeignConversation }

func Parse(d *spec.Descriptor, canonical string, raw []byte) (models.CanonicalEvent, error) {
	decoded, err := decode(d, raw)
	if err != nil {
		return models.CanonicalEvent{}, err
	}
	if field, ok := ownsConversation(d, decoded); !ok {
		return models.CanonicalEvent{}, &ForeignConversationError{Field: field}
	}
	fields, declared := d.EventFields(canonical)
	if !declared {
		return models.CanonicalEvent{}, fmt.Errorf("%w: %q on %q", ErrUndeclaredEvent, canonical, d.ID)
	}
	return build(canonical, fields, decoded), nil
}

func decode(d *spec.Descriptor, raw []byte) (map[string]any, error) {
	if format := d.HookFormat(); format != "json" {
		return nil, fmt.Errorf("%w %q on %q", ErrUnsupportedFormat, format, d.ID)
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

func ownsConversation(d *spec.Descriptor, decoded map[string]any) (string, bool) {
	for _, field := range d.RequiredPayloadFields() {
		if payload.String(decoded, field) == "" {
			return field, false
		}
	}
	return "", true
}

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

		Result:     firstNonEmptyJSON(decoded, fields["tool_result"]),
		Error:      firstNonEmpty(decoded, fields["tool_error"]),
		Status:     firstNonEmpty(decoded, fields["tool_status"]),
		DurationMS: duration,
	}
}

// branches splits an alternation. v2 spelled it with a comma and v3 spells it `||`;
// both are accepted so one parser serves both shapes while they coexist.
func branches(expr string) []string {
	if strings.Contains(expr, "||") {
		return strings.Split(expr, "||")
	}
	return strings.Split(expr, ",")
}

func firstNonEmptyJSON(decoded map[string]any, expr string) []byte {
	if expr == "" {
		return nil
	}
	for _, path := range branches(expr) {
		if v := payload.JSON(decoded, strings.TrimSpace(path)); len(v) > 0 {
			return v
		}
	}
	return nil
}

func firstNonEmpty(decoded map[string]any, expr string) string {
	if expr == "" {
		return ""
	}
	for _, path := range branches(expr) {
		if v := payload.String(decoded, strings.TrimSpace(path)); v != "" {
			return v
		}
	}
	return ""
}

func Declared(d *spec.Descriptor) []string {
	if d == nil {
		return nil
	}
	return d.DeclaredEvents()
}
