package agents

import (
	"context"
	"errors"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/catalog"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/move"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/selection"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spawn"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/template"
)

var errPromptSubmitUnsupported = errors.New("agents: provider does not support chat prompt submission")

type Agents interface {
	List(ctx context.Context, homeDir string) ([]Agent, error)

	Get(ctx context.Context, homeDir, id string) (Agent, error)

	RecordInjection(runnerID string, docs ...string)

	WasInjected(runnerID, text string) bool

	ForgetRunner(runnerID string)
}

type Agent interface {
	ID() string
	Display() Display

	Installed() bool

	Capabilities() Capabilities

	WithTools(enabled bool) Agent

	Models() []string

	Efforts(model string) []string

	SelectionSteps(sel Selection) []InjectStep

	SelectionRestart(launched, desired Selection) bool

	SpawnPlan(ctx TemplateCtx, baseEnv []string, extra []InjectStep) (*SpawnPlan, error)

	PromptSteps(resume bool) ([]InjectStep, error)

	ContextSteps(resuming bool) []InjectStep

	ResumeArg() (string, bool)

	ParseHook(canonical string, raw []byte) (CanonicalEvent, error)

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
}

type service struct {
	injected *registry.Registry
}

func New() Agents {
	return &service{injected: registry.New()}
}

func (s *service) List(ctx context.Context, homeDir string) ([]Agent, error) {
	descriptors, err := protocol.All(ctx, homeDir)
	if err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(descriptors))
	for _, d := range descriptors {
		out = append(out, &agent{spec: d})
	}
	return out, nil
}

func (s *service) Get(ctx context.Context, homeDir, id string) (Agent, error) {
	d, err := protocol.Resolve(ctx, homeDir, id)
	if err != nil {
		return nil, err
	}
	return &agent{spec: d}, nil
}

func (s *service) RecordInjection(runnerID string, docs ...string) {
	s.injected.SetInjected(runnerID, docs...)
}

func (s *service) WasInjected(runnerID, text string) bool {
	return s.injected.Consume(runnerID, text)
}

func (s *service) ForgetRunner(runnerID string) {
	s.injected.Forget(runnerID)
}

func Expand(s string, ctx TemplateCtx) string {
	return template.Expand(s, ctx)
}

func Decide(currentSession, announcedSession, knownChatID string, known bool) Decision {
	return move.Decide(currentSession, announcedSession, knownChatID, known)
}

type agent struct {
	spec *spec.Descriptor
}

func (a *agent) ID() string { return a.spec.ID }

func (a *agent) Display() Display {
	return Display{Name: a.spec.DisplayName, Icon: a.spec.Icon}
}

func (a *agent) Installed() bool { return protocol.Installed(a.spec.Spawn.Cmd) }

func (a *agent) Capabilities() Capabilities {
	caps := Capabilities{
		SlashCatalog: a.spec.Presentation.SlashCatalog != nil,
		Telemetry:    a.spec.Telemetry != nil,
		ModelSelect:  a.spec.Model != nil,
		EffortSelect: a.spec.Effort != nil,

		TerminalPrompts: protocol.TerminalPrompts(a.spec),
		Compaction:      protocol.CanSend(a.spec, "compact_start"),
		Observes:        protocol.Observes(a.spec),

		Hotswap:     a.spec.Runtime.Hotswap,
		HasTerminal: a.spec.Runtime.Transport != "api" || len(a.spec.Runtime.API.Attach) > 0,
	}
	if ps := a.spec.Presentation.PromptSubmit; ps != nil {
		caps.PromptSubmit = true
		caps.Delivery = ps.Strategy
	}
	return caps
}

func (a *agent) WithTools(enabled bool) Agent {
	if enabled {
		return a
	}

	stripped := *a.spec
	stripped.MCPInject = nil
	return &agent{spec: &stripped}
}

func (a *agent) Models() []string {
	return selection.Models(a.spec)
}

func (a *agent) Efforts(model string) []string {
	return selection.Efforts(a.spec, model)
}

func (a *agent) SelectionSteps(sel Selection) []InjectStep {
	return selection.Steps(a.spec, sel)
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

func (a *agent) ContextSteps(resuming bool) []InjectStep {
	if resuming {
		return spec.CloneSteps(a.spec.ResumeContextInject)
	}
	return spec.CloneSteps(a.spec.ContextInject)
}

func (a *agent) ResumeArg() (string, bool) {
	if a.spec.Session.Resume == nil || a.spec.Session.Resume.Arg == "" {
		return "", false
	}
	return a.spec.Session.Resume.Arg, true
}

func (a *agent) ParseHook(canonical string, raw []byte) (CanonicalEvent, error) {
	return protocol.Recv(a.spec, canonical, raw)
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

var (
	_ Agents = (*service)(nil)
	_ Agent  = (*agent)(nil)
	_        = models.Telemetry{}
)
