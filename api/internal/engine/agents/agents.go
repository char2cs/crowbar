package agents

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/catalog"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/modeldiscovery"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/move"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/selection"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/sessionstore"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spawn"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/template"
)

var errPromptSubmitUnsupported = errors.New("agents: provider does not support chat prompt submission")

// Allowed reports whether want is acceptable against a resolved catalogue.
// A non-empty known list rejects anything it omits, same as always. An
// EMPTY one only means "nothing is valid" when nothing could ever resolve
// it (discovered is false — no catalogue declared, or a static one that is
// itself empty); when discovered is true, empty means "a live probe hasn't
// resolved this yet", and rejecting on that would wedge selection every
// time discovery is merely pending or failed. Every selection-validation
// gate (promptswitch.go, the two selection.go files) reads
// Agent.Models()/Efforts() plus Agent.Capabilities().ModelDiscovery through
// this rather than a bare slices.Contains.
func Allowed(known []string, discovered bool, want string) bool {
	if len(known) > 0 {
		return slices.Contains(known, want)
	}
	return discovered
}

// errAPITransportNotDeclared is StartAPIConn's refusal for a hooks-only
// descriptor — one with no runtime.api and no event overriding transport: api.
var errAPITransportNotDeclared = errors.New("agents: provider does not declare an api transport")

type Agents interface {
	List(ctx context.Context, homeDir string) ([]Agent, error)

	Get(ctx context.Context, homeDir, id string) (Agent, error)

	RecordInjection(runnerID string, docs ...string)

	// ConsumeInjectedPrefix reports whether text contains a document this
	// runnerID was handed and, if so, returns text with that document (and
	// mergeLeadingPositional's own separator) removed — see
	// registry.ConsumePrefix's own doc for why a bare boolean isn't enough: a
	// real prompt can ride the SAME positional as the injected document, and
	// the remainder left after removing it is what the user actually typed.
	ConsumeInjectedPrefix(runnerID, text string) (remainder string, found bool)

	ForgetRunner(runnerID string)

	// SetManifestFetchEnabled installs the live getter a model.manifest:
	// source's background refresh reads before hitting the network — pulled
	// fresh on every refresh attempt, never cached across a toggle. A nil
	// getter (the state before this is ever called — every bare New(), every
	// test) means DISABLED: only the embedded bundle and disk cache apply, so
	// a raw Agents never makes a network call as a side effect of resolving
	// a descriptor. The app layer wires the real, DB-backed getter (whose own
	// default is enabled) once at boot — see chat.assembly.go.
	SetManifestFetchEnabled(enabled func() bool)

	// Close stops every background model-discovery refresh this service has
	// forked and BLOCKS until each has returned, the disk write it may owe
	// under homeDir included. Cancelling the WithLifecycle context alone does
	// not do that: a refresh whose probe or fetch has already returned is past
	// every ctx check left to it, so only this join promises that nothing
	// writes under a home the caller is about to release. Idempotent.
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

	// DefaultModel is whichever model id the provider ITSELF states is its
	// default (model.discover.default_when) — empty when the source states
	// nothing at all, or nothing has resolved yet. Never inferred from
	// Models()'s own order.
	DefaultModel() string

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

	// SelectionAPISteps is SelectionSteps' sibling for the api channel: the
	// same choice declared onto the `serve` process, for a spawn that forks
	// no process for SelectionSteps to ride. APIServeArgv already renders
	// these; this is here so a caller can ask WHICH fields a carrier takes
	// without launching anything.
	SelectionAPISteps(sel Selection) []InjectStep

	SelectionRestart(launched, desired Selection) bool

	SpawnPlan(ctx TemplateCtx, baseEnv []string, extra []InjectStep) (*SpawnPlan, error)

	PromptSteps(resume bool) ([]InjectStep, error)

	// PromptLeadingSigils is presentation.prompt_submit.leading_sigils: the
	// characters this CLI reads as a control gesture rather than as text when
	// one of them opens a message, and the escape that makes one text again.
	// Empty chars for a provider that declares none.
	PromptLeadingSigils() (chars []string, escape string)

	ContextSteps(resuming bool) []InjectStep

	ResumeArg() (string, bool)

	// SessionExists reports whether the provider still has sessionID on disk
	// (session.locate); declared is false when the descriptor gives no way to
	// check, and the id must then be trusted.
	SessionExists(sessionID string) (exists, declared bool)

	// ParseHook turns one raw provider payload into a canonical event, reading
	// the field map channel selects: the channel-scoped block the delivery
	// ACTUALLY arrived on (a channel-scoped event's own api:/hooks: block),
	// never the event's static declared transport — see spec.Channel and
	// TransportFor's own doc comment for why those are different facts.
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

	// StartAPIConn dials this provider's API socket (already resolved by the
	// caller — the same TemplateCtx machinery SpawnPlan uses expands {socket})
	// and completes its declared handshake. Returns ErrAPITransportNotDeclared
	// for a hooks-only descriptor — never (nil, nil). origin (nil-able) is told
	// which sessions the connection goes on to produce itself — see
	// APISessionOrigin.
	StartAPIConn(ctx context.Context, socketPath string, origin APISessionOrigin) (*APIConn, error)

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

	// EventSurfaces is spec.Descriptor.EventSurfaces, exposed the same narrow
	// way (design spec P6b tag 2).
	EventSurfaces(canonical string) []string

	// SurfaceChannel is spec.Descriptor.SurfaceChannel, exposed the same
	// narrow way: which delivery channel drives one SURFACE's facts (design
	// spec 2.5's surfaces.<name>.channel), "" for an undeclared surface.
	SurfaceChannel(surface string) string

	// TelemetryChannel is the delivery channel this provider's usage reports
	// arrive on, and TelemetryOnSurface asks whether a chat on one SURFACE
	// can receive them at all — the chat-scoped form of the capability, since
	// a provider-scoped flag cannot tell two chats of the same provider apart.
	TelemetryChannel() string
	TelemetryOnSurface(surface string) bool

	// SurfaceStartHere is spec.Descriptor.SurfaceStartHere: whether a
	// brand-new chat may be LAUNCHED directly onto this surface. The same
	// fact Capabilities.TerminalStartHere reports for the terminal, asked
	// per surface by the spawn path.
	SurfaceStartHere(surface string) bool
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

	// discovery is the shared, provider-keyed cache behind a model.discover
	// OR model.manifest descriptor's Models()/Efforts() — one instance for
	// every agent this service ever builds, so a refresh triggered by one
	// List/Get is visible to the next, however many *agent values point at
	// it.
	discovery *modeldiscovery.Cache

	// sessions locates provider sessions on disk for the resume ladder.
	sessions *sessionstore.Finder

	// manifestFetchMu guards manifestFetch, the live getter
	// SetManifestFetchEnabled installs — read fresh on every manifest
	// refresh, never latched at construction, so a settings toggle takes
	// effect on the very next refresh rather than needing a restart.
	manifestFetchMu sync.RWMutex
	manifestFetch   func() bool
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

// serviceOpts is New's own option set — currently just the lifecycle context
// its model-discovery cache forks background work against.
type serviceOpts struct {
	lifecycle context.Context
}

// Option configures New.
type Option func(*serviceOpts)

// WithLifecycle ties every background model-discovery refresh this service
// ever forks to ctx: cancelling it (a daemon's own shutdown) stops in-flight
// probes/fetches and refuses to store or write one that raced past the
// cancellation — see modeldiscovery.Cache's own doc. Omitted, refreshes are
// bound to context.Background() — never cancelled by anything — which is the
// state every existing bare New() call (test or otherwise) is already in;
// the app layer is the only caller that needs this, wiring its own
// shutdown-bound context once at boot.
func WithLifecycle(ctx context.Context) Option {
	return func(o *serviceOpts) { o.lifecycle = ctx }
}

func New(opts ...Option) Agents {
	cfg := serviceOpts{lifecycle: context.Background()}
	for _, o := range opts {
		o(&cfg)
	}
	return &service{
		injected:    registry.New(),
		descriptors: map[string]descriptorCacheEntry{},
		discovery:   modeldiscovery.NewCache(cfg.lifecycle),
		sessions:    sessionstore.New(),
	}
}

func (s *service) List(ctx context.Context, homeDir string) ([]Agent, error) {
	descriptors, err := protocol.All(ctx, homeDir)
	if err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(descriptors))
	for _, d := range descriptors {
		s.refreshModelsIfDeclared(d, homeDir)
		out = append(out, &agent{spec: d, discovery: s.discovery, sessions: s.sessions})
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
	s.refreshModelsIfDeclared(d, homeDir)
	a := &agent{spec: d, discovery: s.discovery, sessions: s.sessions}

	s.resolved.Lock()
	s.descriptors[key] = descriptorCacheEntry{agent: a, overrideModTime: modTime}
	s.resolved.Unlock()
	return a, nil
}

// refreshModelsIfDeclared kicks off a background catalogue refresh for a
// descriptor that declares model.discover or model.manifest — a no-op (and
// zero extra cost on every other Get/List call) for one that declares
// neither.
func (s *service) refreshModelsIfDeclared(d *spec.Descriptor, homeDir string) {
	if d.Model == nil {
		return
	}
	if d.Model.Discover != nil {
		s.discovery.Refresh(d, homeDir)
	}
	if d.Model.Manifest != nil {
		s.discovery.RefreshManifest(d, homeDir, protocol.EmbeddedModelManifest(), s.manifestFetchAllowed())
	}
}

// SetManifestFetchEnabled installs the live getter refreshModelsIfDeclared's
// model.manifest half reads before hitting the network.
func (s *service) SetManifestFetchEnabled(enabled func() bool) {
	s.manifestFetchMu.Lock()
	defer s.manifestFetchMu.Unlock()
	s.manifestFetch = enabled
}

// manifestFetchAllowed defaults to DISABLED (nil getter, the state before
// SetManifestFetchEnabled is ever called) — every bare New(), every test —
// so resolving a descriptor never makes a network call as a side effect
// until the app layer explicitly wires the real, DB-backed getter (whose own
// default IS enabled) at boot.
func (s *service) manifestFetchAllowed() bool {
	s.manifestFetchMu.RLock()
	defer s.manifestFetchMu.RUnlock()
	if s.manifestFetch == nil {
		return false
	}
	return s.manifestFetch()
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

// Close joins this service's model-discovery cache — see Agents.Close.
func (s *service) Close() {
	s.discovery.Close()
}

func (s *service) RecordInjection(runnerID string, docs ...string) {
	s.injected.SetInjected(runnerID, docs...)
}

func (s *service) ConsumeInjectedPrefix(runnerID, text string) (string, bool) {
	return s.injected.ConsumePrefix(runnerID, text)
}

func (s *service) ForgetRunner(runnerID string) {
	s.injected.Forget(runnerID)
}

func Expand(s string, ctx TemplateCtx) string {
	return template.Expand(s, ctx)
}

// Decide places a conversation a CLI has announced — see move.Decide, and
// especially what crowbarOriginated is load-bearing for.
func Decide(
	currentSession, announcedSession, knownChatID string,
	known, crowbarOriginated bool,
) Decision {
	return move.Decide(currentSession, announcedSession, knownChatID, known, crowbarOriginated)
}

type agent struct {
	spec *spec.Descriptor
	// discovery is nil for an agent built outside this package's own New()
	// (a handful of white-box tests construct &agent{spec: d} directly);
	// Models/Efforts fall back to the descriptor's static catalogue then,
	// same as a descriptor with no discover: block at all.
	discovery *modeldiscovery.Cache
	// sessions is the service's shared session locator (nil in white-box tests).
	sessions *sessionstore.Finder
}

func (a *agent) ID() string { return a.spec.ID }

func (a *agent) Display() Display {
	return Display{Name: a.spec.DisplayName, Icon: a.spec.Icon}
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
		Observes:        protocol.Observes(a.spec),

		Hotswap:     a.spec.Runtime.Hotswap,
		HasTerminal: a.spec.Runtime.Transport != "api" || len(a.spec.Runtime.API.Attach) > 0,

		TerminalStartHere: a.spec.SurfaceStartHere(spec.SurfaceTerminal),
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
	return &agent{spec: &stripped, discovery: a.discovery}
}

// discoversModels reports whether Models/Efforts/DefaultModel must read the
// shared discovery cache instead of the descriptor's own static tables —
// true for EITHER a live probe (model.discover) or a fetched/bundled
// catalogue (model.manifest); the two are mutually exclusive per descriptor
// (rules.modelCatalog) so there is never a question of which one to prefer.
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

func (a *agent) PermissionLevels() []string {
	return selection.PermissionLevels(a.spec)
}

func (a *agent) PermissionVars(level string) map[string]string {
	return selection.PermissionVars(a.spec, level)
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

func (a *agent) ResumeArg() (string, bool) {
	if a.spec.Session.Resume == nil || a.spec.Session.Resume.Arg == "" {
		return "", false
	}
	return a.spec.Session.Resume.Arg, true
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

func (a *agent) StartAPIConn(
	ctx context.Context, socketPath string, origin APISessionOrigin,
) (*APIConn, error) {
	if a.spec.Runtime.Transport != "api" && !hasAPIEventOverride(a.spec) {
		return nil, errAPITransportNotDeclared
	}
	return protocol.StartAPIDriver(ctx, a.spec, socketPath, origin)
}

// APIServeArgv is the api-transport `serve` argv: MCP + session config (never
// hook wiring — spawn.InjectServe) plus the chat's model/effort via api_apply,
// since this process is the only carrier a PTY-less runner has.
func (a *agent) APIServeArgv(ctx TemplateCtx) ([]string, bool) {
	if len(a.spec.Runtime.API.Serve) == 0 {
		return nil, false
	}
	argv := expandArgv(a.spec.Runtime.API.Serve, ctx)
	plan := &SpawnPlan{Executable: argv[0], Argv: append([]string{}, argv[1:]...)}
	sel := Selection{Model: ctx.Model, Effort: ctx.Effort}
	if err := spawn.InjectServe(a.spec, ctx, plan, selection.APISteps(a.spec, sel)); err != nil {
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

func (a *agent) EventSurfaces(canonical string) []string {
	return a.spec.EventSurfaces(canonical)
}

func (a *agent) SurfaceChannel(surface string) string {
	return string(a.spec.SurfaceChannel(surface))
}

func (a *agent) SurfaceStartHere(surface string) bool {
	return a.spec.SurfaceStartHere(surface)
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
