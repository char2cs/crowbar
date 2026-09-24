package inbound

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var (
	ErrUnsupportedFormat = errors.New("agents: unsupported hook format")

	ErrUndeclaredEvent = errors.New("agents: undeclared hook event")

	ErrForeignConversation = errors.New("agents: hook does not describe this CLI's own conversation")

	// ErrRequiredFieldMissing is design spec 2.3's own rule: a field an event
	// declares required: resolved to nothing against the delivered payload.
	// This is the highest-value rule the descriptor channel-split migration
	// adds — a mapping that silently resolves to empty used to be
	// indistinguishable from one that never had anything to resolve.
	ErrRequiredFieldMissing = errors.New("agents: required field resolved to nothing")

	// ErrVariantMismatch is a delivery that named a canonical event whose own
	// when: discriminator its payload does not satisfy: the same wire event,
	// a different variant. Not a failure of anything — the sibling event that
	// DOES match is delivered separately and handles it — so every caller
	// drops it quietly.
	ErrVariantMismatch = errors.New("agents: payload is a different variant of this wire event")
)

// VariantMismatchError names which descriptor, event and channel rejected the
// delivery, and the discriminator it was judged against — the same four facts
// RequiredFieldError carries, for the same reason: a silent drop is
// indistinguishable from a mapping that never ran.
type VariantMismatchError struct {
	Descriptor string
	Event      string
	Channel    string
	When       map[string][]string
}

func (e *VariantMismatchError) Error() string {
	return fmt.Sprintf("%s: descriptor %q event %q channel %q when %v",
		ErrVariantMismatch, e.Descriptor, e.Event, e.Channel, e.When)
}

func (e *VariantMismatchError) Unwrap() error { return ErrVariantMismatch }

type ForeignConversationError struct {
	Field string
}

func (e *ForeignConversationError) Error() string {
	return fmt.Sprintf("%s (missing %q)", ErrForeignConversation, e.Field)
}

func (e *ForeignConversationError) Unwrap() error { return ErrForeignConversation }

// RequiredFieldError names exactly which descriptor, event, channel and
// field failed to resolve — the four facts design spec 2.3 asks the error to
// carry.
type RequiredFieldError struct {
	Descriptor string
	Event      string
	Channel    string
	Field      string
}

func (e *RequiredFieldError) Error() string {
	return fmt.Sprintf("%s: descriptor %q event %q channel %q field %q",
		ErrRequiredFieldMissing, e.Descriptor, e.Event, e.Channel, e.Field)
}

func (e *RequiredFieldError) Unwrap() error { return ErrRequiredFieldMissing }

// Parse turns one raw provider payload into a canonical event, reading the
// field map channel selects — the block the delivery ACTUALLY arrived on,
// never the event's static declared transport. See spec.EventSpec.
// WireEventFor's own doc comment for why: a dual-shape event's shape arrives
// per message, not per event declaration.
func Parse(d *spec.Descriptor, canonical string, raw []byte, channel spec.Channel) (models.CanonicalEvent, error) {
	decoded, err := decode(d, raw)
	if err != nil {
		return models.CanonicalEvent{}, err
	}
	// The ownership guard (RequiredPayloadFields, e.g. codex's transcript_path)
	// exists for HTTP-delivered hook payloads: any process on the machine can
	// POST one, so Crowbar must confirm it actually names THIS CLI's own
	// conversation before trusting it.
	//
	// THE TRAP (pre-channel-split): the guard used to be skipped whenever
	// TransportFor(canonical) == "api", on the reasoning that an api-transport
	// event structurally never carries a hooks-only field. That reasoning
	// breaks for a DUAL-SHAPE event (codex's session_start/user_prompt/
	// turn_stop, which inherit the api default but are still ALSO fired
	// hooks-shaped by codex's own internal memory-consolidation session) — the
	// skip was keyed on the event's STATIC declared transport, not on whether
	// THIS delivery is actually hooks-shaped, so it let the memory session's
	// payload through unchecked and reintroduced the chat-theft bug this guard
	// exists for.
	//
	// channel is now the fix, directly: it is the ACTUAL delivery channel
	// (caller-supplied, derived from where the bytes physically arrived — see
	// turn/ingest.go's channelFor — never inferred from the payload or the
	// event's declared transport). The guard is skipped ONLY for ChannelAPI —
	// the one channel that structurally cannot be forged (a private jsonrpc2
	// connection Crowbar itself dialed to the child process, not an HTTP
	// surface anything on the machine can POST to) — and runs for hooks and
	// for any channel this switch does not yet know about, fail-closed.
	if channel != spec.ChannelAPI {
		if field, ok := ownsConversation(d, decoded); !ok {
			return models.CanonicalEvent{}, &ForeignConversationError{Field: field}
		}
	}
	// EventFieldsFor, not EventFields: a channel-scoped event's field map
	// lives in its api:/hooks: block, selected by the channel THIS delivery
	// actually arrived on — never inferred from the payload's own shape, and
	// never from the event's static Transport (see the design spec's chat-
	// theft account). A legacy event answers the same regardless of channel.
	fields, declared := d.EventFieldsFor(canonical, channel)
	if !declared {
		return models.CanonicalEvent{}, fmt.Errorf(
			"%w: %q on %q (channel %q)", ErrUndeclaredEvent, canonical, d.ID, channel)
	}
	// when: — the sum-type discriminator, applied on EVERY channel.
	//
	// BEFORE required:, deliberately: a delivery that is a different variant of
	// the same wire event has no obligation to carry the fields THIS variant
	// declares required, so checking required: first would report a missing
	// field where the real answer is "not this event".
	//
	// This is what lets one wire hook mean two canonical events. claude's
	// Notification carries both "I am waiting for your input" (its only
	// authoritative idle report) and a permission prompt; settings.json fires
	// one relay command per canonical name, so BOTH arrive, and only the
	// descriptor's own when: can tell them apart. Arming idle off an unfiltered
	// Notification would abandon a genuinely live turn seconds later — the
	// discriminator is the safety mechanism, not an optimisation. See
	// spec.Descriptor.EventWhenFor.
	if when := d.EventWhenFor(canonical, channel); len(when) > 0 &&
		!mapping.Match(decoded, when) {
		return models.CanonicalEvent{}, &VariantMismatchError{
			Descriptor: d.ID, Event: canonical, Channel: string(channel), When: when,
		}
	}
	// required: (design spec 2.3) — a field the event declares required that
	// resolves to nothing against THIS payload is a hard, named error, not a
	// CanonicalEvent silently missing it. Checked against the same fields/
	// decoded pair build() is about to read, so this can never disagree with
	// what actually gets constructed.
	for _, req := range d.EventRequired(canonical) {
		if mapping.String(decoded, fields[req]) == "" {
			return models.CanonicalEvent{}, &RequiredFieldError{
				Descriptor: d.ID, Event: canonical, Channel: string(channel), Field: req,
			}
		}
	}
	return build(canonical, fields, d.EventSteps(canonical), decoded), nil
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
		// Absent, not merely empty: a payload whose shape never carries this
		// field at all (a genuine api-transport delivery of a dual-shape
		// event) is not evidence of anything and must not be rejected — see
		// Parse's own doc on why this replaced a transport-wide skip.
		if !mapping.Present(decoded, []string{field}) {
			continue
		}
		if mapping.String(decoded, []string{field}) == "" {
			return field, false
		}
	}
	return "", true
}

func build(
	canonical string,
	fields spec.FieldMap,
	steps *spec.StepsSpec,
	decoded map[string]any,
) models.CanonicalEvent {
	get := func(name string) string { return mapping.String(decoded, fields[name]) }

	ev := models.CanonicalEvent{
		Kind:      canonical,
		SessionID: get("session_id"),
		Message:   get("message"),
		TurnID:    get("turn_id"),
		AsyncWork: mapping.Count(decoded, fields["async_work"]),
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
	case spec.HookMessageDelta, spec.HookReasoningDelta, spec.HookToolOutputDelta:
		// Same payload shape, deliberately: a thought and an answer are both
		// streamed text belonging to one item. Only ev.Kind tells them apart, and
		// only the consumer acts on that difference.
		ev.Delta = buildDelta(fields, decoded)
	case spec.HookPlanUpdate:
		ev.Plan = buildPlan(steps, decoded)
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

func buildDelta(fields spec.FieldMap, decoded map[string]any) *models.MessageDelta {
	index, _ := mapping.Int(decoded, fields["index"])
	final, _ := mapping.Bool(decoded, fields["final"])
	return &models.MessageDelta{
		TurnID:    mapping.String(decoded, fields["turn_id"]),
		MessageID: mapping.String(decoded, fields["message_id"]),
		Index:     index,
		Sequenced: len(fields["index"]) > 0,
		Final:     final,
		Text:      mapping.String(decoded, fields["text"]),
	}
}

// buildPlan reads the whole plan array, WHOLESALE: the newest list is the entire
// truth, so there is nothing to merge and nothing that can drift — the same
// anti-drift rule turn_stop's async-work LEVEL follows.
//
// A step with no text is skipped (a plan entry with nothing to say is not a step),
// but an unknown status passes through UNCHANGED rather than being dropped: a
// status the descriptor's map does not name is still a step worth showing, and
// silently shortening the plan would be worse than an unstyled row.
func buildPlan(steps *spec.StepsSpec, decoded map[string]any) []models.PlanStep {
	if steps == nil || steps.Items == "" {
		return nil
	}
	rows := mapping.Objects(decoded, []string{steps.Items})
	out := make([]models.PlanStep, 0, len(rows))
	for _, row := range rows {
		text := mapping.String(row, []string{steps.Text})
		if text == "" {
			continue
		}
		status := mapping.String(row, []string{steps.Status})
		if mapped, ok := steps.StatusMap[status]; ok {
			status = mapped
		}
		out = append(out, models.PlanStep{Text: text, Status: status})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildTool(fields spec.FieldMap, decoded map[string]any) *models.ToolEvent {
	duration, _ := mapping.Int(decoded, fields["duration_ms"])
	return &models.ToolEvent{
		ID:     mapping.String(decoded, fields["tool_id"]),
		Name:   mapping.String(decoded, fields["tool_name"]),
		Target: mapping.String(decoded, fields["tool_target"]),
		Input:  mapping.JSON(decoded, fields["tool_input"]),

		Result:          mapping.JSON(decoded, fields["tool_result"]),
		Error:           mapping.String(decoded, fields["tool_error"]),
		Status:          mapping.String(decoded, fields["tool_status"]),
		DurationMS:      duration,
		NestedSessionID: mapping.String(decoded, fields["nested_session_id"]),
	}
}

func Declared(d *spec.Descriptor) []string {
	if d == nil {
		return nil
	}
	return d.DeclaredEvents()
}
