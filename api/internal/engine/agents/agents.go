// Package agents maps provider-owned CLI facts into Crowbar-neutral values.
//
// It owns no chat, no runner and no process lifetime. It renders spawn plans,
// interprets hook and telemetry payloads, and runs bounded capability probes —
// and it does all of that from declarative descriptors, so supporting another CLI
// is a YAML file rather than a branch in Go.
//
// Two hard constraints hold throughout. Crowbar hosts the ordinary interactive
// CLI in a real PTY and never a headless one, which the spawn planner enforces
// rather than trusts. And nothing above this package learns a provider's
// vocabulary: callers switch on canonical kinds and read Crowbar's own facts.
package agents

import (
	"context"
	"errors"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/catalog"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/hooks"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/move"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spawn"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/telemetry"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/template"
)

var errPromptSubmitUnsupported = errors.New("agents: provider does not support chat prompt submission")

// Agents resolves configured agents and remembers what was injected into their
// live processes.
type Agents interface {
	// List enumerates every known agent: the descriptors embedded in the binary,
	// overlaid by any override on disk under homeDir. A single invalid override
	// costs that agent its row rather than failing the list.
	List(ctx context.Context, homeDir string) ([]Agent, error)

	// Get resolves one agent by id, preferring an on-disk override. Returns
	// ErrUnknownAgent when nothing answers to the id.
	Get(ctx context.Context, homeDir, id string) (Agent, error)

	// RecordInjection remembers documents handed to a runner at spawn, so the CLI
	// echoing one back through its user-prompt hook can be recognised rather than
	// recorded as something the user typed.
	RecordInjection(runnerID string, docs ...string)

	// WasInjected reports — and consumes — a match against RecordInjection.
	WasInjected(runnerID, text string) bool

	// ForgetRunner drops a dead runner's injected documents.
	ForgetRunner(runnerID string)
}

// Agent is one configured provider CLI.
//
// Its ID is the domain's ProviderID: this type is the engine's handle on that
// provider, not a new identity. It is named Agent rather than Provider because
// the Git forge integration already owns that word.
type Agent interface {
	ID() string
	Display() Display

	// Installed reports whether the CLI is present on this machine. Install-only:
	// neither CLI exposes a reliable machine-readable auth check, so a logged-out
	// install reads installed and fails at spawn, where it can be explained.
	Installed() bool

	// Capabilities reports which optional surfaces this agent declares. An absent
	// capability renders as absent UI, never as a disabled control implying
	// breakage.
	Capabilities() Capabilities

	// WithTools returns the agent a spawn should render: this one when Crowbar's
	// tool surface is switched on for the provider, or a copy with its MCP
	// registration removed when it is not.
	//
	// It copies rather than clearing in place, because the resolved agent may be
	// shared: a switch that emptied the field would turn one chat's preference
	// into every later chat's.
	WithTools(enabled bool) Agent

	// SpawnPlan renders a concrete process launch.
	SpawnPlan(ctx TemplateCtx, baseEnv []string, extra []InjectStep) (*SpawnPlan, error)

	// PromptSteps returns the argv steps that carry a chat prompt into a fresh or
	// resumed CLI. Returns ErrPromptSubmitUnsupported when the provider is
	// terminal-only for that operation.
	PromptSteps(resume bool) ([]InjectStep, error)

	// ContextSteps returns the steps that deliver Crowbar's context document. A
	// resumed session may need a different channel from a fresh one: verified
	// against codex, a resumed CLI silently ignores new developer instructions, so
	// the only channel that reaches it is a user message.
	ContextSteps(resuming bool) []InjectStep

	// ResumeArg returns the argument template that resumes a native session.
	ResumeArg() (string, bool)

	// ParseHook decodes, ownership-checks and maps one raw hook payload.
	//
	// The three are fused deliberately. The ownership check is what stops a
	// provider's own internal session being filed as the user's conversation, and
	// a guard a caller can forget to call is a guard that will be forgotten.
	ParseHook(canonical string, raw []byte) (CanonicalEvent, error)

	// ParseTelemetry maps a provider-pushed telemetry payload onto whichever facts
	// it reported. Absent facts stay absent; nothing is derived that was not
	// reported.
	ParseTelemetry(raw []byte, now time.Time) (Telemetry, error)

	// SlashCatalog runs the declared inventory in the given worktree.
	SlashCatalog(ctx context.Context, opts ProbeOptions, acquire Acquire) (SlashCatalog, error)

	// ProbeTelemetry runs the declared telemetry command, for a provider that is
	// polled rather than one that pushes. Returns ErrTelemetryUnsupported when no
	// probe is declared, which is the case for both agents shipped today.
	ProbeTelemetry(ctx context.Context, opts ProbeOptions, acquire Acquire, now time.Time) (Telemetry, error)
}

type service struct {
	injected *registry.Registry
}

// New constructs the agents engine.
func New() Agents {
	return &service{injected: registry.New()}
}

func (s *service) List(ctx context.Context, homeDir string) ([]Agent, error) {
	descriptors, err := descriptor.All(ctx, homeDir)
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
	d, err := descriptor.Resolve(ctx, homeDir, id)
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

// Expand renders a template against a spawn context. It is exported because
// Crowbar's own configured prompts — the handoff pointer, the capability preamble
// — interpolate the same placeholders a descriptor does, and they must not grow a
// second expansion table that can drift from this one.
func Expand(s string, ctx TemplateCtx) string {
	return template.Expand(s, ctx)
}

// Decide is the context-move reducer, re-exported because it is a pure function
// of the caller's own state and needs no engine instance.
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

func (a *agent) Installed() bool { return descriptor.Installed(a.spec.Spawn.Cmd) }

func (a *agent) Capabilities() Capabilities {
	caps := Capabilities{
		SlashCatalog: a.spec.Presentation.SlashCatalog != nil,
		Telemetry:    a.spec.Telemetry != nil,
		Observes:     hooks.Declared(a.spec),
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
	// Shallow copy: only the one slice header is replaced, and nothing mutates
	// through the others.
	stripped := *a.spec
	stripped.MCPInject = nil
	return &agent{spec: &stripped}
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
	return hooks.Parse(a.spec, canonical, raw)
}

func (a *agent) ParseTelemetry(raw []byte, now time.Time) (Telemetry, error) {
	return telemetry.ParseCallback(a.spec, raw, now)
}

func (a *agent) SlashCatalog(
	ctx context.Context,
	opts ProbeOptions,
	acquire Acquire,
) (SlashCatalog, error) {
	return catalog.Probe(ctx, a.spec, opts, acquire)
}

func (a *agent) ProbeTelemetry(
	ctx context.Context,
	opts ProbeOptions,
	acquire Acquire,
	now time.Time,
) (Telemetry, error) {
	return telemetry.Probe(ctx, a.spec, opts, acquire, now)
}

// compile-time assertions that the exported surface is satisfied.
var (
	_ Agents = (*service)(nil)
	_ Agent  = (*agent)(nil)
	_        = models.Telemetry{}
)
