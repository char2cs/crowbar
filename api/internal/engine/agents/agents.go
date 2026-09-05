package agents

import (
	"context"
	"errors"
	"os"
	"sync"
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

// errAPITransportNotDeclared is StartAPIConn's refusal for a hooks-only
// descriptor — one with no runtime.api and no event overriding transport: api.
var errAPITransportNotDeclared = errors.New("agents: provider does not declare an api transport")

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

	// PermissionLevels reports which of Crowbar's own guarded/trusted/
	// full-auto names this provider can actually reach. A level absent here
	// must never be offered for this provider — never clamped to one that is.
	PermissionLevels() []string

	// PermissionVars is level's own named values for a transport that needs
	// them as request data rather than as an argv flag — see
	// spec.PermissionLevelSpec's own doc comment. Empty for a level this
	// provider doesn't declare.
	PermissionVars(level string) map[string]string

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

	// StartAPIConn dials this provider's API socket (already resolved by the
	// caller — the same TemplateCtx machinery SpawnPlan uses expands {socket})
	// and completes its declared handshake. Returns ErrAPITransportNotDeclared
	// for a hooks-only descriptor — never (nil, nil).
	StartAPIConn(ctx context.Context, socketPath string) (*APIConn, error)

	// APIServeArgv and APIAttachArgv are runtime.api.serve / .attach, already
	// template-expanded against ctx, mirroring PromptSteps's (x, bool) shape: ok
	// is false when this descriptor declares no such field.
	APIServeArgv(ctx TemplateCtx) ([]string, bool)
	APIAttachArgv(ctx TemplateCtx) ([]string, bool)

	// TransportFor is spec.Descriptor.TransportFor, exposed narrowly for the same
	// reason every other capability here is narrow rather than exposing the raw
	// descriptor: a caller outside this package can only ever reach a provider
	// through this interface, never through *spec.Descriptor by name.
	TransportFor(canonical string) string
}

type service struct {
	injected *registry.Registry

	resolved sync.Mutex
	// descriptors caches Get's answer per (homeDir, id), because Get sits on
	// the hottest path in the daemon: ingestResolvedHook (turn/ingest.go)
	// calls it once per ingested hook, including once per streamed delta —
	// several times a SECOND on a fast-streaming provider. protocol.Resolve
	// itself is deliberately always-fresh (reads and re-validates the YAML on
	// every call, so an on-disk override edited mid-development is picked up
	// at once — see its own tests), and measured at over a millisecond per
	// call for codex's descriptor alone. Paying that on every token is what
	// made Codex's live streaming view visibly stall while Claude's (whose
	// hooks-paced deltas arrive far less often) did not: the single goroutine
	// that folds each event into the ledger (apiconn.go's pumpAPIConn) fell
	// behind the incoming rate and only caught up once the provider stopped
	// producing more — confirmed live. Invalidated by Stat'ing the override
	// path (descriptorCacheEntry's own doc comment), which costs a syscall
	// but not a read+parse+validate, on every call — so an edited override is
	// still picked up without a restart, just without paying Resolve's full
	// cost to notice nothing changed.
	descriptors map[string]descriptorCacheEntry
}

// descriptorCacheEntry is one Get result, held until the on-disk override
// path it was resolved against changes.
type descriptorCacheEntry struct {
	agent Agent
	// overrideModTime is protocol.OverridePath's mtime as of this resolve, or
	// the zero Time when no override file existed then (including every
	// homeDir-less caller). Compared against a fresh Stat on every Get so an
	// override created, edited, or removed on disk invalidates the entry —
	// the zero Time changing to a real one, or a real one changing, are both
	// "this changed" under time.Time.Equal.
	overrideModTime time.Time
}

func New() Agents {
	return &service{injected: registry.New(), descriptors: map[string]descriptorCacheEntry{}}
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
	key := homeDir + "\x00" + id
	modTime := overrideModTime(protocol.OverridePath(homeDir, id))

	s.resolved.Lock()
	cached, ok := s.descriptors[key]
	s.resolved.Unlock()
	if ok && cached.overrideModTime.Equal(modTime) {
		return cached.agent, nil
	}

	d, err := protocol.Resolve(ctx, homeDir, id)
	if err != nil {
		return nil, err
	}
	a := &agent{spec: d}

	s.resolved.Lock()
	s.descriptors[key] = descriptorCacheEntry{agent: a, overrideModTime: modTime}
	s.resolved.Unlock()
	return a, nil
}

// overrideModTime is the zero Time for an empty path (no override possible)
// or one Stat could not reach (none exists, or a permission error — either
// way, Resolve itself will fail the same Stat too, so there is nothing this
// cache could get more wrong than Resolve already would).
func overrideModTime(path string) time.Time {
	if path == "" {
		return time.Time{}
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
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

func (a *agent) PermissionLevels() []string {
	return selection.PermissionLevels(a.spec)
}

func (a *agent) PermissionVars(level string) map[string]string {
	return selection.PermissionVars(a.spec, level)
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

func (a *agent) StartAPIConn(ctx context.Context, socketPath string) (*APIConn, error) {
	if a.spec.Runtime.Transport != "api" && !hasAPIEventOverride(a.spec) {
		return nil, errAPITransportNotDeclared
	}
	return protocol.StartAPIDriver(ctx, a.spec, socketPath)
}

// APIServeArgv carries the SAME MCPInject/ConfigInjection steps
// APIAttachArgv's sibling SpawnPlan applies to a hooks-attached CLI — a
// provider's own MCP server is a fact about the provider, not about which of
// its processes happens to be talking to Crowbar right now (see spawn.Inject).
// Without this, an api-transport serve process is never told Crowbar's tools
// exist at all: confirmed live against codex-cli 0.149.1, whose thread/start
// started only the servers config.toml already knew about, never crowbar's.
//
// A failed injection degrades to ok=false, same as no declared serve argv at
// all: the caller already knows that means "run this provider over hooks
// alone" (design spec §2.2b), which is the right answer here too rather than
// serving with tools silently missing.
func (a *agent) APIServeArgv(ctx TemplateCtx) ([]string, bool) {
	if len(a.spec.Runtime.API.Serve) == 0 {
		return nil, false
	}
	argv := expandArgv(a.spec.Runtime.API.Serve, ctx)
	plan := &SpawnPlan{Executable: argv[0], Argv: append([]string{}, argv[1:]...)}
	if err := spawn.Inject(a.spec, ctx, plan, nil); err != nil {
		return nil, false
	}
	return append([]string{plan.Executable}, plan.Argv...), true
}

// APIAttachArgv carries the SAME MCPInject/ConfigInjection steps APIServeArgv
// now does — see that method's doc comment. It matters even more here: the
// attached process is an ORDINARY hooks-transport CLI the instant it starts
// (config_injection is where session_start/user_prompt/turn_stop/tool_pre&
// post/subagent_pre&post/permission/compact_pre&post/session_end all get
// wired to {crowbar_hook}), so without this the one process a non-hotswap
// provider hands a live turn over to would report NOTHING back to Crowbar's
// ledger — indistinguishable from the chat going silently out of sync.
//
// It also carries spawn.Args (see spawn.PrependArgs), the same as a primary
// interactive spawn does — the attached process IS an interactive TUI in
// every way that matters here. Without this, codex's attached `resume`
// carries config_injection's per-segment hook wiring but never
// --dangerously-bypass-hook-trust, so it parks on its interactive hook-trust
// confirmation screen instead of reaching the composer: confirmed live, this
// is what "switch to terminal" looked like for codex before this fix.
func (a *agent) APIAttachArgv(ctx TemplateCtx) ([]string, bool) {
	if len(a.spec.Runtime.API.Attach) == 0 {
		return nil, false
	}
	argv := expandArgv(a.spec.Runtime.API.Attach, ctx)
	plan := &SpawnPlan{Executable: argv[0], Argv: append([]string{}, argv[1:]...)}
	spawn.PrependArgs(a.spec, ctx, plan)
	if err := spawn.Inject(a.spec, ctx, plan, nil); err != nil {
		return nil, false
	}
	return append([]string{plan.Executable}, plan.Argv...), true
}

func (a *agent) TransportFor(canonical string) string {
	return a.spec.TransportFor(canonical)
}

func expandArgv(argv []string, ctx TemplateCtx) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		out[i] = template.Expand(a, ctx)
	}
	return out
}

// hasAPIEventOverride reports whether any event declares transport: api even
// when the runtime default is something else — defensive: neither shipped
// descriptor relies on this today (codex's runtime.transport IS api once
// merged), but StartAPIConn must not assume Runtime.Transport is the only
// source of truth.
func hasAPIEventOverride(d *spec.Descriptor) bool {
	for _, e := range d.Events {
		if e.Transport == "api" {
			return true
		}
	}
	return false
}

var (
	_ Agents = (*service)(nil)
	_ Agent  = (*agent)(nil)
	_        = models.Telemetry{}
)
