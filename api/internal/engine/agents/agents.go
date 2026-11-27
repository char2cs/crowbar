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

	ConsumeInjectedPrefix(runnerID, text string) (remainder string, found bool)

	ForgetRunner(runnerID string)

	SetManifestFetchEnabled(enabled func() bool)

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

	DefaultModel() string

	DefaultEffort(model string) string

	PermissionLevels() []string

	PermissionVars(level string, left bool) map[string]string
	ModelParts(model string) map[string]string

	SelectionSteps(sel Selection) []InjectStep

	SelectionAPISteps(sel Selection) []InjectStep

	SelectionRestart(launched, desired Selection) bool

	SpawnPlan(ctx TemplateCtx, baseEnv []string, extra []InjectStep) (*SpawnPlan, error)

	PromptSteps(resume bool) ([]InjectStep, error)

	PromptLeadingSigils() (chars []string, escape string)

	PromptSteer(text string) (stdout string, ok bool)
	PromptType() (typing PromptTyping, ok bool)
	CanType(text string) bool
	PromptTypeWrites(text string) (writes [][]byte, ok bool)
	ComposerEmpty(screen ComposerScreen) bool
	ToolDenial(reason string) (stdout string, ok bool)
	PromptIsCommand(text string) bool

	ContextSteps(resuming bool) []InjectStep

	ResumeArg() (string, bool)
	SessionAssignArg() (string, bool)
	SessionPinsRelay() bool
	RefusedResume(exitCode int, output string) (line string, refused bool)
	SessionPersists() string

	ParseHook(canonical string, raw []byte, channel Channel) (CanonicalEvent, error)

	ParseTelemetry(raw []byte, now time.Time, channel Channel) (Telemetry, error)

	AnswerCapability(canonical string) (AnswerCapability, bool)

	RenderAnswer(canonical string, raw []byte, decision AnswerDecision) ([]byte, error)

	SlashCatalog(ctx context.Context, opts ProbeOptions, acquire Acquire) (SlashCatalog, error)

	ProbeTelemetry(ctx context.Context, opts ProbeOptions, acquire Acquire, now time.Time) (Telemetry, error)

	MatchTerminalPrompt(screen string) (TerminalPrompt, bool)

	OutboundCall(canonical string, values map[string]string) (wire string, payload map[string]string, ok bool)

	StartAPIConn(ctx context.Context, stdio APIStdio, origin APISessionOrigin) (*APIConn, error)

	APIDialed() bool
	APIOnlyPrompts() bool
	StartAPIEndpoint(ctx context.Context, tctx TemplateCtx, origin APISessionOrigin) (*APIConn, error)

	APIServeArgv(ctx TemplateCtx) ([]string, bool)
	APIAttachArgv(ctx TemplateCtx) ([]string, bool)

	TransportFor(canonical string) string

	EventSurfaces(canonical string) []string

	AnswerEndsAtAction() bool

	SurfaceChannel(surface string) string

	TelemetryChannel() string
	TelemetryOnSurface(surface string) bool

	SurfaceStartHere(surface string) bool
}

func Expand(s string, ctx TemplateCtx) string {
	return template.Expand(s, ctx)
}

type agent struct {
	spec      *spec.Descriptor
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

func (a *agent) ParseTelemetry(raw []byte, now time.Time, channel Channel) (Telemetry, error) {
	return protocol.RecvTelemetry(a.spec, raw, now, channel)
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

func (a *agent) AnswerEndsAtAction() bool {
	return a.spec.Events[HookMessageDelta].EndsAtAction
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
