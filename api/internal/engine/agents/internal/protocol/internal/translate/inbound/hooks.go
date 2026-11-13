package inbound

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/promptorigin"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var (
	ErrUnsupportedFormat = errors.New("agents: unsupported hook format")

	ErrUndeclaredEvent = errors.New("agents: undeclared hook event")

	ErrForeignConversation = errors.New("agents: hook does not describe this CLI's own conversation")

	// ErrRequiredFieldMissing is a required: field that resolved to nothing against
	// the delivered payload, so a broken mapping fails loudly instead of reading empty.
	ErrRequiredFieldMissing = errors.New("agents: required field resolved to nothing")

	// ErrVariantMismatch is a payload that fails the event's when: discriminator: a
	// different variant of the same wire event, which the matching sibling handles,
	// so callers drop it quietly.
	ErrVariantMismatch = errors.New("agents: payload is a different variant of this wire event")
)

// VariantMismatchError names the descriptor, event, channel and discriminator that
// rejected the delivery, so a quiet drop is still diagnosable.
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

// RequiredFieldError names the descriptor, event, channel and field that failed
// to resolve.
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

// Parse turns one raw provider payload into a canonical event, reading the field
// map of the channel the delivery ACTUALLY arrived on: a dual-shape event's shape
// is per message, not per declaration.
func Parse(d *spec.Descriptor, canonical string, raw []byte, channel spec.Channel) (models.CanonicalEvent, error) {
	decoded, err := decode(d, raw, channel)
	if err != nil {
		return models.CanonicalEvent{}, err
	}
	// The ownership guard confirms an HTTP-delivered payload names THIS CLI's own
	// conversation, since any local process can POST one. Only the api channel, a
	// private connection Crowbar dialled itself, is exempt; every other channel is checked.
	if channel != spec.ChannelAPI {
		if field, ok := ownsConversation(d, decoded); !ok {
			return models.CanonicalEvent{}, &ForeignConversationError{Field: field}
		}
	}
	// The field map is the block of the channel THIS delivery arrived on, never
	// inferred from the payload's shape or the event's static transport.
	fields, declared := d.EventFieldsFor(canonical, channel)
	if !declared {
		return models.CanonicalEvent{}, fmt.Errorf(
			"%w: %q on %q (channel %q)", ErrUndeclaredEvent, canonical, d.ID, channel)
	}
	// when: selects which deliveries of one wire hook this event accepts, so it runs
	// before required: (a different variant owes this one no fields). It is what keeps
	// a hook carrying both an idle report and a prompt from ending a live turn.
	if when := d.EventWhenFor(canonical, channel); len(when) > 0 &&
		!mapping.Match(decoded, when) {
		return models.CanonicalEvent{}, &VariantMismatchError{
			Descriptor: d.ID, Event: canonical, Channel: string(channel), When: when,
		}
	}
	// A required: field resolving to nothing is a named error. It reads the same
	// fields/decoded pair build() uses, so the check cannot disagree with it.
	for _, req := range d.EventRequired(canonical) {
		if mapping.String(decoded, fields[req]) == "" {
			return models.CanonicalEvent{}, &RequiredFieldError{
				Descriptor: d.ID, Event: canonical, Channel: string(channel), Field: req,
			}
		}
	}
	tool := toolSpecs{
		status: d.EventStatusMap(canonical), kinds: d.EventKindMap(canonical), shellKinds: d.EventShellKindMap(canonical),
		locations: d.EventLocations(canonical), diffFiles: d.EventDiffFiles(canonical), patch: d.EventPatch(canonical),
	}
	ev := build(canonical, fields, d.EventSteps(canonical), d.EventStatusMap(canonical), tool, decoded)
	if ev.Choice != nil && ev.Choice.Kind == models.ChoiceToolPermission {
		ev.Choice.Options = append(ev.Choice.Options, suggestionOptions(fields, d.EventSuggestions(canonical, decoded))...)
		markReasons(ev.Choice.Options, d, canonical)
	}
	if canonical == spec.HookUserPrompt {
		ev = endedShell(d, handedBack(d, ev))
	}
	return ev, nil
}

// handedBack is a prompt its descriptor declares a subagent's handed-back
// result as that result: a subagent_result of the scope it names, opening the
// parent's turn the prompt's own turn id names.
func handedBack(d *spec.Descriptor, ev models.CanonicalEvent) models.CanonicalEvent {
	p, ok := promptorigin.Match(d, ev.Message)
	if !ok || p.Kind != spec.InjectedPromptSubagentResult {
		return ev
	}
	scope := p.ScopeID.Find(ev.Message)
	if scope == "" {
		return ev
	}
	status := p.Status.Find(ev.Message)
	if mapped, ok := d.EventStatusMap(spec.HookSubagentResult)[status]; ok {
		status = mapped
	}
	return models.CanonicalEvent{
		Kind: spec.HookSubagentResult, SessionID: ev.SessionID, TurnID: ev.TurnID, Raw: ev.Raw,
		Subagent: &models.SubagentEvent{ID: scope, Status: status, Result: strings.TrimSpace(p.Result.Find(ev.Message))},
	}
}

// endedShell is a prompt its descriptor declares the end of a background task
// as that task named: still the prompt that opens the turn answering it.
func endedShell(d *spec.Descriptor, ev models.CanonicalEvent) models.CanonicalEvent {
	if ev.Kind != spec.HookUserPrompt {
		return ev
	}
	p, ok := promptorigin.Match(d, ev.Message)
	if !ok || p.Kind != spec.InjectedPromptTaskNotification {
		return ev
	}
	id := p.TaskID.Find(ev.Message)
	if id == "" {
		return ev
	}
	status := p.Status.Find(ev.Message)
	if mapped, ok := p.StatusMap[status]; ok {
		status = mapped
	}
	ev.Shell = &models.ShellEvent{ID: id, Status: status, Summary: p.Summary.Find(ev.Message), Note: p.Event.Find(ev.Message)}
	return ev
}

// decode reads raw as JSON. Only a hook delivery declares its format; an api frame
// is JSON by its transport, so a descriptor with no hooks block still parses.
func decode(d *spec.Descriptor, raw []byte, channel spec.Channel) (map[string]any, error) {
	if format := d.HookFormat(); format != "json" && channel != spec.ChannelAPI {
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
		// Only a present-but-empty field is foreign: a payload shape that never
		// carries it (an api delivery of a dual-shape event) proves nothing.
		if !mapping.Present(decoded, mapping.Paths(field)) {
			continue
		}
		if mapping.String(decoded, mapping.Paths(field)) == "" {
			return field, false
		}
	}
	return "", true
}

func build(
	canonical string,
	fields spec.FieldMap,
	steps *spec.StepsSpec,
	statusMap map[string]string,
	tool toolSpecs,
	decoded map[string]any,
) models.CanonicalEvent {
	get := func(name string) string { return mapping.String(decoded, fields[name]) }

	ev := models.CanonicalEvent{
		Kind:       canonical,
		SessionID:  get("session_id"),
		Message:    get("message"),
		TurnID:     get("turn_id"),
		TurnStatus: mapped(statusMap, get("turn_status")),
		Model:      get("model"),
		Effort:     get("effort"),
		Reason:     get("reason"),
		Raw:        decoded,
	}
	switch canonical {
	case spec.HookSessionStart:
		// Why it started, in Crowbar's words: a clear starts the conversation over.
		ev.Reason = mapped(statusMap, ev.Reason)
	case spec.HookUserPrompt:
		ev.Subagent = scope(get("scope_id"))
	case spec.HookToolPre, spec.HookToolPost, spec.HookToolFail, spec.HookTerminalInput:
		ev.Tool = buildTool(fields, tool, decoded)
		ev.Subagent = scope(get("scope_id"))
	case spec.HookSubagentPre, spec.HookSubagentPost, spec.HookSubagentResult:
		ev.Subagent = buildSubagent(fields, statusMap, decoded)
	case spec.HookPlanStep:
		ev.Step = buildStep(fields, statusMap, decoded)
		ev.Subagent = scope(ev.Step.ScopeID)
	case spec.HookNotification:
		ev.Interrupt = &models.InterruptEvent{Kind: models.InterruptNotification, Detail: ev.Message}
	case spec.HookPermission:
		ev.Interrupt = &models.InterruptEvent{Kind: models.InterruptPermission, Detail: ev.Message}
		ev.Choice = permissionChoice(fields, decoded)
	case spec.HookElicitation:
		ev.Interrupt = &models.InterruptEvent{Kind: models.InterruptElicitation, Detail: ev.Message}
		ev.Choice = elicitationChoice(fields, decoded, ev.Message)
	case spec.HookMessageDelta, spec.HookReasoningDelta, spec.HookToolOutputDelta,
		spec.HookDiffUpdate:
		// Same payload shape, deliberately: a thought and an answer are both
		// streamed text belonging to one item. Only ev.Kind tells them apart, and
		// only the consumer acts on that difference.
		ev.Delta = buildDelta(fields, decoded)
	case spec.HookMessageComplete:
		ev.Complete = &models.MessageComplete{
			TurnID: ev.TurnID, MessageID: get("message_id"), Text: get("text"), Phase: get("phase"),
		}
	case spec.HookPlanUpdate:
		ev.Plan = buildPlan(steps, decoded)
	case spec.HookTurnFailed:
		kind, _ := mapping.Scalar(decoded, fields["failure_kind"])
		ev.Failure = &models.TurnFailure{Reason: get("reason"), Detail: get("detail"), Kind: statusMap[kind]}
	case spec.HookCompactPre, spec.HookCompactPost:
		ev.Interrupt = &models.InterruptEvent{
			Kind: models.InterruptCompaction, Detail: get("trigger"), Resolved: canonical == spec.HookCompactPost,
		}
	}
	return ev
}

// mapped is v in Crowbar's words where m names it, else v unchanged.
func mapped(m map[string]string, v string) string {
	if out, ok := m[v]; ok {
		return out
	}
	return v
}

// scope is the subagent an event ran under; nil is the main conversation.
func scope(id string) *models.SubagentEvent {
	if id == "" {
		return nil
	}
	return &models.SubagentEvent{ID: id}
}

func buildSubagent(fields spec.FieldMap, statusMap map[string]string, decoded map[string]any) *models.SubagentEvent {
	get := func(name string) string { return mapping.String(decoded, fields[name]) }
	a := &models.SubagentEvent{
		ID: get("scope_id"), AgentType: get("agent_type"), Title: get("title"),
		Status: mapped(statusMap, get("status")), Result: get("result"),
	}
	tokens, hasTokens := mapping.Int(decoded, fields["usage.tokens"])
	uses, hasUses := mapping.Int(decoded, fields["usage.tool_uses"])
	duration, hasDuration := mapping.Int(decoded, fields["usage.duration"])
	if hasTokens || hasUses || hasDuration {
		a.Usage = &models.SubagentUsage{Tokens: int64(tokens), ToolUses: int64(uses), DurationMs: int64(duration)}
	}
	return a
}

func buildStep(fields spec.FieldMap, statusMap map[string]string, decoded map[string]any) *models.PlanStepEvent {
	get := func(name string) string { return mapping.String(decoded, fields[name]) }
	return &models.PlanStepEvent{
		ScopeID: get("scope_id"), StepID: get("step_id"), Text: get("text"), Detail: get("detail"),
		Status: mapped(statusMap, get("status")),
	}
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

// buildPlan reads the whole plan array: the newest list is the entire truth, so
// nothing merges or drifts. Text-less steps are skipped; an unmapped status passes
// through unchanged, since an unstyled row beats silently shortening the plan.
func buildPlan(steps *spec.StepsSpec, decoded map[string]any) []models.PlanStep {
	if steps == nil || steps.Items == "" {
		return nil
	}
	rows := mapping.Objects(decoded, mapping.Paths(steps.Items))
	out := make([]models.PlanStep, 0, len(rows))
	for _, row := range rows {
		text := mapping.String(row, mapping.Paths(steps.Text))
		if text == "" {
			continue
		}
		status := mapped(steps.StatusMap, mapping.String(row, mapping.Paths(steps.Status)))
		out = append(out, models.PlanStep{Text: text, Status: status})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// toolSpecs is what an event's descriptor declares beside its field map to read a tool call.
type toolSpecs struct {
	status, kinds, shellKinds map[string]string
	locations                 *spec.LocationsSpec
	diffFiles                 *spec.DiffFilesSpec
	patch                     *spec.PatchSpec
}

func buildTool(fields spec.FieldMap, specs toolSpecs, decoded map[string]any) *models.ToolEvent {
	statusMap, kindMap, shellKinds := specs.status, specs.kinds, specs.shellKinds
	locations, diffFiles, patch := specs.locations, specs.diffFiles, specs.patch
	duration, _ := mapping.Int(decoded, fields["duration_ms"])
	status := mapped(statusMap, mapping.String(decoded, fields["tool_status"]))
	kind := mapping.String(decoded, fields["tool_kind"])
	if mapped, ok := mapping.LookupKind(kindMap, kind); ok {
		kind = mapped
	}
	var exitCode *int
	if code, ok := mapping.Int(decoded, fields["exit_code"]); ok {
		exitCode = &code
	}
	interrupted, _ := mapping.Bool(decoded, fields["interrupted"])
	tool := &models.ToolEvent{
		ID:        mapping.String(decoded, fields["tool_id"]),
		Name:      mapping.String(decoded, fields["tool_name"]),
		Kind:      kind,
		Locations: buildLocations(locations, decoded),
		Target:    mapping.String(decoded, fields["tool_target"]),
		Input:     mapping.JSON(decoded, fields["tool_input"]),

		Title:       mapping.String(decoded, fields["tool_title"]),
		MCPServer:   mapping.String(decoded, fields["mcp_server"]),
		MCPTool:     mapping.String(decoded, fields["mcp_tool"]),
		ExitCode:    exitCode,
		Interrupted: interrupted,

		EditBefore: mapping.String(decoded, fields["edit_before"]),
		EditAfter:  mapping.String(decoded, fields["edit_after"]),
		Cwd:        mapping.String(decoded, fields["cwd"]),
		Diff:       buildDiffFiles(diffFiles, decoded),
		Patch:      buildPatch(patch, decoded),

		Result:         mapping.JSON(decoded, fields["tool_result"]),
		Output:         mapping.String(decoded, fields["tool_output"]),
		Error:          mapping.String(decoded, fields["tool_error"]),
		Status:         status,
		DurationMS:     duration,
		SpawnsScope:    mapping.String(decoded, fields["spawns_scope"]),
		AddressesScope: mapping.String(decoded, fields["addresses_scope"]),
		SpawnsShell:    mapping.String(decoded, fields["spawns_shell"]),
		StopsShell:     mapping.String(decoded, fields["stops_shell"]),
		ShellKind:      mapped(shellKinds, mapping.String(decoded, fields["shell_kind"])),
		ShellRef:       mapping.String(decoded, fields["shell_ref"]),
	}
	tool.ShellPersistent, _ = mapping.Bool(decoded, fields["shell_persistent"])
	tool.ShellTimeoutMS, _ = mapping.Int(decoded, fields["shell_timeout_ms"])
	return tool
}

func buildDiffFiles(files *spec.DiffFilesSpec, decoded map[string]any) string {
	if files == nil {
		return ""
	}
	var out strings.Builder
	for _, row := range mapping.Objects(decoded, mapping.Paths(files.Items)) {
		path := strings.TrimLeft(mapping.String(row, mapping.Paths(files.Path)), "/")
		diff := mapping.String(row, mapping.Paths(files.Diff))
		if path == "" || diff == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		whole := ""
		if files.Kind != "" {
			whole = files.Whole[mapping.String(row, mapping.Paths(files.Kind))]
		}
		switch {
		case whole != "":
			writeWholeFile(&out, path, diff, whole == spec.DiffAdded)
			continue
		case !strings.HasPrefix(diff, "diff --git ") && !strings.HasPrefix(diff, "--- "):
			fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", path, path)
		}
		out.WriteString(diff)
		if !strings.HasSuffix(diff, "\n") {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// writeWholeFile writes content as the one hunk that creates (added) or removes path.
func writeWholeFile(out *strings.Builder, path, content string, added bool) {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if added {
		fmt.Fprintf(out, "--- /dev/null\n+++ b/%s\n@@ -0,0 +1%s @@\n", path, hunkCount(len(lines)))
	} else {
		fmt.Fprintf(out, "--- a/%s\n+++ /dev/null\n@@ -1%s +0,0 @@\n", path, hunkCount(len(lines)))
	}
	sign := map[bool]string{true: "+", false: "-"}[added]
	for _, line := range lines {
		out.WriteString(sign + line + "\n")
	}
}

// hunkCount is a hunk range's ",count" suffix, which unified diff omits for one line.
func hunkCount(n int) string {
	if n == 1 {
		return ""
	}
	return fmt.Sprintf(",%d", n)
}

func buildPatch(patch *spec.PatchSpec, decoded map[string]any) []models.PatchHunk {
	if patch == nil {
		return nil
	}
	var out []models.PatchHunk
	for _, row := range mapping.Objects(decoded, mapping.Paths(patch.Items)) {
		oldStart, _ := mapping.Int(row, mapping.Paths(patch.OldStart))
		newStart, _ := mapping.Int(row, mapping.Paths(patch.NewStart))
		lines := mapping.Strings(row, mapping.Paths(patch.Lines))
		if len(lines) == 0 {
			continue
		}
		out = append(out, models.PatchHunk{OldStart: oldStart, NewStart: newStart, Lines: lines})
	}
	return out
}

func buildLocations(specification *spec.LocationsSpec, decoded map[string]any) []models.ToolLocation {
	if specification == nil || specification.Items == "" || specification.Path == "" {
		return nil
	}
	rows := mapping.Objects(decoded, mapping.Paths(specification.Items))
	out := make([]models.ToolLocation, 0, len(rows))
	for _, row := range rows {
		path := mapping.String(row, mapping.Paths(specification.Path))
		if path == "" {
			continue
		}
		line, _ := mapping.Int(row, mapping.Paths(specification.Line))
		out = append(out, models.ToolLocation{Path: path, Line: line})
	}
	return out
}

func Declared(d *spec.Descriptor) []string {
	if d == nil {
		return nil
	}
	return d.DeclaredEvents()
}
