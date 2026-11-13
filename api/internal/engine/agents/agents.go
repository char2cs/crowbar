package agents

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/catalog"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/modeldiscovery"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/selection"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spawn"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/template"
)

var errPromptSubmitUnsupported = errors.New("agents: provider does not support chat prompt submission")

// Allowed reports whether want is acceptable against a resolved catalogue. An empty known list
// rejects everything only when nothing could resolve it (!discovered); while discovery is pending
// or failed, rejecting would wedge selection.
func Allowed(known []string, discovered bool, want string) bool {
	if len(known) > 0 {
		return slices.Contains(known, want)
	}
	return discovered
}

type Agents interface {
	List(ctx context.Context, homeDir string) ([]Agent, error)

	Get(ctx context.Context, homeDir, id string) (Agent, error)

	RecordInjection(runnerID string, docs ...string)

	// ConsumeInjectedPrefix reports whether text carries a document runnerID was handed, and returns
	// text without it: a real prompt can ride the same positional, and the remainder is what the user
	// typed.
	ConsumeInjectedPrefix(runnerID, text string) (remainder string, found bool)

	ForgetRunner(runnerID string)

	// SetManifestFetchEnabled installs the getter a model.manifest refresh asks before hitting the
	// network. Until it is called fetching is disabled, so resolving a descriptor never makes a
	// network call as a side effect.
	SetManifestFetchEnabled(enabled func() bool)

	// Close stops every background model-discovery refresh and blocks until each, with any disk write
	// it owes, has returned: cancelling the lifecycle context cannot stop one past its last ctx check.
	// Idempotent.
	Close()
}

type Agent interface {
	ID() string
	Display() Display

	Installed() bool

	Capabilities() Capabilities

	WithTools(enabled bool) Agent

	Models() []string

	Efforts(model string) []string

	// DefaultModel is the model the provider itself states is its default, empty when it states none
	// or nothing has resolved yet; never inferred from Models()'s order.
	DefaultModel() string

	// DefaultEffort is the effort model runs at when none is chosen, as the provider states it; empty
	// when it states none.
	DefaultEffort(model string) string

	// PermissionLevels reports which of Crowbar's own guarded/trusted/
	// full-auto names this provider can actually reach. A level absent here
	// must never be offered for this provider — never clamped to one that is.
	PermissionLevels() []string

	// PermissionVars is level's named values for a transport that takes them as request data rather
	// than an argv flag, narrowed for a chat that left a folder; empty for an undeclared level.
	PermissionVars(level string, left bool) map[string]string
	// ModelParts is a composed model id's named parts, nil for any other id.
	ModelParts(model string) map[string]string

	SelectionSteps(sel Selection) []InjectStep

	// SelectionAPISteps is SelectionSteps for the api channel's `serve` process, so a caller can ask
	// which fields a carrier takes without launching anything.
	SelectionAPISteps(sel Selection) []InjectStep

	SelectionRestart(launched, desired Selection) bool

	SpawnPlan(ctx TemplateCtx, baseEnv []string, extra []InjectStep) (*SpawnPlan, error)

	PromptSteps(resume bool) ([]InjectStep, error)

	// PromptLeadingSigils is presentation.prompt_submit.leading_sigils: the
	// characters that open a control gesture rather than text, and the escape
	// that makes one text again. Empty chars when none are declared.
	PromptLeadingSigils() (chars []string, escape string)

	// PromptSteer renders text as the turn-end hook stdout that delivers it
	// into a RUNNING turn (presentation.prompt_submit.steer); ok is false when
	// the provider declares none or text is a command the descriptor excludes.
	PromptSteer(text string) (stdout string, ok bool)
	// ToolDenial renders the reply that refuses one tool call before it runs,
	// with reason shown to the model; ok is false when the provider declares none.
	ToolDenial(reason string) (stdout string, ok bool)
	// PromptIsCommand reports whether text is a command the CLI runs itself
	// (presentation.prompt_submit.steer.skip_prefixes), which may open no turn.
	PromptIsCommand(text string) bool

	ContextSteps(resuming bool) []InjectStep

	ResumeArg() (string, bool)
	SessionAssignArg() (string, bool)
	// RefusedResume is the line by which the CLI refused a resume, even wrapped.
	RefusedResume(exitCode int, output string) (line string, refused bool)
	// SessionPersists is session.persists: "turn_start" (or "") or "turn_end".
	SessionPersists() string

	// ParseHook turns one raw payload into a canonical event, reading the block of the channel the
	// delivery actually arrived on, never the event's declared transport (see spec.Channel).
	ParseHook(canonical string, raw []byte, channel Channel) (CanonicalEvent, error)

	ParseTelemetry(raw []byte, now time.Time) (Telemetry, error)

	AnswerCapability(canonical string) (AnswerCapability, bool)

	RenderAnswer(canonical string, raw []byte, decision AnswerDecision) ([]byte, error)

	SlashCatalog(ctx context.Context, opts ProbeOptions, acquire Acquire) (SlashCatalog, error)

	ProbeTelemetry(ctx context.Context, opts ProbeOptions, acquire Acquire, now time.Time) (Telemetry, error)

	MatchTerminalPrompt(screen string) (TerminalPrompt, bool)

	// OutboundCall resolves a canonical event Crowbar drives — compact_start,
	// interrupt, prompt — into this provider's own call and payload. The bool is the
	// capability: a provider that declares the event cannot be asked to do it.
	OutboundCall(canonical string, values map[string]string) (wire string, payload map[string]string, ok bool)

	// StartAPIConn completes the declared handshake over serve's stdio, closing
	// it on any error; ErrAPITransportNotDeclared for a hooks-only descriptor.
	// origin (nil-able) learns the sessions the conn produces.
	StartAPIConn(ctx context.Context, stdio APIStdio, origin APISessionOrigin) (*APIConn, error)

	// APIDialed is whether runtime.api reaches its server by dialing (delivery
	// websocket or http) instead of by a serve process; StartAPIEndpoint then
	// connects it, with the same refusal and cleanup as StartAPIConn.
	APIDialed() bool
	// APIOnlyPrompts is a descriptor whose connection is the only carrier of a
	// prompt: it declares no prompt_submit argv to restart a CLI with.
	APIOnlyPrompts() bool
	StartAPIEndpoint(ctx context.Context, tctx TemplateCtx, origin APISessionOrigin) (*APIConn, error)

	// APIServeArgv and APIAttachArgv are runtime.api.serve / .attach, already
	// template-expanded against ctx, mirroring PromptSteps's (x, bool) shape: ok
	// is false when this descriptor declares no such field.
	APIServeArgv(ctx TemplateCtx) ([]string, bool)
	APIAttachArgv(ctx TemplateCtx) ([]string, bool)

	// TransportFor is spec.Descriptor.TransportFor, exposed narrowly: callers outside this package
	// reach a provider only through this interface, never through *spec.Descriptor.
	TransportFor(canonical string) string

	// EventSurfaces is spec.Descriptor.EventSurfaces, exposed the same narrow
	// way.
	EventSurfaces(canonical string) []string

	// SurfaceChannel is spec.Descriptor.SurfaceChannel, exposed the same
	// narrow way: which delivery channel drives one SURFACE's facts
	// (the descriptor's surfaces.<name>.channel), "" for an undeclared surface.
	SurfaceChannel(surface string) string

	// TelemetryChannel is where usage reports arrive; TelemetryOnSurface asks whether a chat on one
	// surface can receive them, since a provider-wide flag cannot tell two of its chats apart.
	TelemetryChannel() string
	TelemetryOnSurface(surface string) bool

	// SurfaceStartHere is whether a brand-new chat may launch directly onto surface, the per-surface
	// form of Capabilities.TerminalStartHere.
	SurfaceStartHere(surface string) bool
}

func Expand(s string, ctx TemplateCtx) string {
	return template.Expand(s, ctx)
}

type agent struct {
	spec *spec.Descriptor
	// discovery is nil for an agent built without New; Models/Efforts then read the descriptor's
	// static catalogue.
	discovery *modeldiscovery.Cache
}

func (a *agent) ID() string { return a.spec.ID }

func (a *agent) Display() Display {
	return Display{Name: a.spec.DisplayName, Icon: a.spec.Icon, SignIn: a.spec.SignIn}
}

func (a *agent) Installed() bool { return protocol.Installed(a.spec.Spawn.Cmd) }

func (a *agent) Capabilities() Capabilities {
	caps := Capabilities{
		SlashCatalog:   a.spec.Presentation.SlashCatalog != nil,
		Telemetry:      a.spec.Telemetry != nil,
		ModelSelect:    a.spec.Model != nil,
		EffortSelect:   a.spec.Effort != nil,
		ModelDiscovery: a.spec.Model != nil && (a.spec.Model.Discover != nil || a.spec.Model.Manifest != nil),

		TerminalPrompts: protocol.TerminalPrompts(a.spec),
		Compaction:      protocol.CanSend(a.spec, "compact_start"),
		ShellStop:       protocol.CanSend(a.spec, "shell_stop"),
		Observes:        protocol.Observes(a.spec),

		Hotswap:     a.spec.Runtime.Hotswap,
		HasTerminal: a.spec.Runtime.Transport != "api" || len(a.spec.Runtime.API.Attach) > 0,

		TerminalStartHere: a.spec.SurfaceStartHere(spec.SurfaceTerminal),
	}
	if ps := a.spec.Presentation.PromptSubmit; ps != nil {
		caps.PromptSubmit = true
		caps.Delivery = ps.Strategy
		caps.PromptSteer = ps.Steer != nil
	}
	return caps
}

func (a *agent) WithTools(enabled bool) Agent {
	if enabled {
		return a
	}

	stripped := *a.spec
	stripped.MCPInject = nil
	return &agent{spec: &stripped, discovery: a.discovery}
}

// discoversModels reports whether Models/Efforts/DefaultModel read the shared discovery cache: for a
// live probe or a manifest catalogue, which a descriptor never declares together.
func (a *agent) discoversModels() bool {
	m := a.spec.Model
	return m != nil && (m.Discover != nil || m.Manifest != nil) && a.discovery != nil
}

func (a *agent) Models() []string {
	if a.discoversModels() {
		return a.discovery.Models(a.spec.ID)
	}
	return selection.Models(a.spec)
}

func (a *agent) Efforts(model string) []string {
	if a.discoversModels() {
		return a.discovery.Efforts(a.spec.ID, model)
	}
	return selection.Efforts(a.spec, model)
}

func (a *agent) DefaultModel() string {
	if a.discoversModels() {
		return a.discovery.DefaultModel(a.spec.ID)
	}
	return ""
}

func (a *agent) DefaultEffort(model string) string {
	if a.discoversModels() {
		return a.discovery.DefaultEffort(a.spec.ID, model)
	}
	return ""
}

func (a *agent) PermissionLevels() []string {
	return selection.PermissionLevels(a.spec)
}

func (a *agent) ModelParts(model string) map[string]string {
	return selection.ModelParts(a.spec, model)
}

func (a *agent) PermissionVars(level string, left bool) map[string]string {
	return selection.PermissionVars(a.spec, level, left)
}

func (a *agent) SelectionSteps(sel Selection) []InjectStep {
	return selection.Steps(a.spec, sel)
}

func (a *agent) SelectionAPISteps(sel Selection) []InjectStep {
	return selection.APISteps(a.spec, sel)
}

func (a *agent) SelectionRestart(launched, desired Selection) bool {
	return selection.RestartRequired(a.spec, launched, desired)
}

func (a *agent) SpawnPlan(ctx TemplateCtx, baseEnv []string, extra []InjectStep) (*SpawnPlan, error) {
	return spawn.Plan(a.spec, ctx, baseEnv, extra)
}

func (a *agent) PromptSteps(resume bool) ([]InjectStep, error) {
	steps, ok := spawn.PromptSteps(a.spec, resume)
	if !ok {
		return nil, errPromptSubmitUnsupported
	}
	return steps, nil
}

func (a *agent) APIOnlyPrompts() bool {
	return a.spec.Runtime.Transport == "api" && a.spec.Presentation.PromptSubmit == nil
}

func (a *agent) PromptLeadingSigils() ([]string, string) {
	ps := a.spec.Presentation.PromptSubmit
	if ps == nil || ps.LeadingSigils == nil {
		return nil, ""
	}
	return slices.Clone(ps.LeadingSigils.Chars), ps.LeadingSigils.Escape
}

func (a *agent) ContextSteps(resuming bool) []InjectStep {
	if resuming {
		return spec.CloneSteps(a.spec.ResumeContextInject)
	}
	return spec.CloneSteps(a.spec.ContextInject)
}

func (a *agent) ParseHook(canonical string, raw []byte, channel Channel) (CanonicalEvent, error) {
	return protocol.Recv(a.spec, canonical, raw, channel)
}

func (a *agent) ParseTelemetry(raw []byte, now time.Time) (Telemetry, error) {
	return protocol.RecvTelemetry(a.spec, raw, now)
}

func (a *agent) AnswerCapability(canonical string) (AnswerCapability, bool) {
	return protocol.AnswerCapability(a.spec, canonical)
}

func (a *agent) RenderAnswer(
	canonical string,
	raw []byte,
	decision AnswerDecision,
) ([]byte, error) {
	return protocol.Reply(a.spec, canonical, raw, decision)
}

func (a *agent) SlashCatalog(
	ctx context.Context,
	opts ProbeOptions,
	acquire Acquire,
) (SlashCatalog, error) {
	return catalog.Probe(ctx, a.spec, opts, acquire)
}

func (a *agent) OutboundCall(
	canonical string,
	values map[string]string,
) (string, map[string]string, bool) {
	return protocol.Send(a.spec, canonical, values)
}

func (a *agent) MatchTerminalPrompt(screen string) (TerminalPrompt, bool) {
	return protocol.MatchTerminalPrompt(a.spec, screen)
}

func (a *agent) ProbeTelemetry(
	ctx context.Context,
	opts ProbeOptions,
	acquire Acquire,
	now time.Time,
) (Telemetry, error) {
	return protocol.ProbeTelemetry(ctx, a.spec, opts, acquire, now)
}

func (a *agent) TransportFor(canonical string) string {
	return a.spec.TransportFor(canonical)
}

func (a *agent) EventSurfaces(canonical string) []string {
	return a.spec.EventSurfaces(canonical)
}

func (a *agent) SurfaceChannel(surface string) string {
	return string(a.spec.SurfaceChannel(surface))
}

func (a *agent) SurfaceStartHere(surface string) bool {
	return a.spec.SurfaceStartHere(surface)
}

var (
	_ Agents = (*service)(nil)
	_ Agent  = (*agent)(nil)
)
