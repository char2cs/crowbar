package agenttools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/engine/mcp"
)

// Deps is everything the tool surface needs from the rest of the app. Later
// phases extend this struct; a nil dependency simply means the tools that need
// it are not registered.
type Deps struct {
	Resolver     *Resolver
	Chats        ChatRenamer
	Review       ReviewReader
	Threads      ThreadReader
	ThreadWrites ThreadWriter
	// Idempotency is the per-daemon retry guard the write tools dedup through. It
	// is a dependency rather than something a tool creates for itself because it
	// must outlive the per-request ToolSet — see Idempotency.
	Idempotency *Idempotency
	// ThreadBroadcast fans an agent-written review thread out to connected
	// clients. Without it an agent's finding is stored but invisible to a user
	// watching the review pane — see ThreadBroadcast.
	ThreadBroadcast ThreadBroadcast
	// ChatReads is the read port list_workspaces and get_chat_log use to list a
	// workspace's chats and to resolve a chat id to the workspace it belongs to.
	// It reuses ChatReader — the same interface the Resolver already needs from
	// the chat store — rather than declaring a second, near-identical one.
	ChatReads ChatReader
	// ChatLogs is the read port get_chat_log calls to render another chat's
	// ledger. It is consulted only once the chat's workspace has already passed
	// c.CanSee — see getChatLog — so a nil ChatLogs simply means the tool that
	// would read it does not exist.
	ChatLogs ChatLogReader
	// Metrics counts calls and failures per tool. Unlike every port above, a nil
	// Metrics must NOT suppress a tool's registration — losing observability is
	// never a reason to lose capability — so it is read only through
	// Metrics.Record, which is safe to call on a nil receiver, and it is
	// deliberately absent from newAgentToolDeps's non-nil checks.
	Metrics *Metrics
	// ToolAccess reports whether the CALLER'S PROVIDER may use this surface at
	// all — the user's per-provider "Tools" switch, consulted on every call.
	//
	// It is a port here rather than a flag baked into the ToolSet because the
	// switch is a live permission, not a spawn-time one: the CLI that registered
	// Crowbar's MCP server at spawn keeps its stdio channel for the life of the
	// process, so a preference read once at spawn leaves a chat spawned with
	// tools ON holding full tool access after the user has switched it off. See
	// ToolSet.Call.
	ToolAccess ProviderToolAccess
}

// ProviderToolAccess answers, for one provider, whether its agent may reach
// Crowbar's tool surface right now. It takes the provider rather than the caller
// because the preference is stored per provider and this port must not need the
// authority model to answer it.
type ProviderToolAccess func(ctx context.Context, providerID string) (bool, error)

// refuseDisabledTools rejects a call whose provider has its tool surface
// switched off, and reports a read failure as a refusal rather than guessing.
//
// A nil ToolAccess means no per-provider switch was wired, which is the surface
// as it behaved before the switch existed: every registered tool is callable.
// That is deliberate — this port is filled in by agent.New alongside Chats and
// ChatLogs, so a nil here is a hand-assembled Deps in a test, never production,
// and TestDispatchMCP_ToolCallIsRefusedOnceToolsAreSwitchedOff pins the real
// wiring against a real *agent.Usecase.
func (d Deps) refuseDisabledTools(ctx context.Context, providerID string) error {
	if d.ToolAccess == nil {
		return nil
	}
	on, err := d.ToolAccess(ctx, providerID)
	if err != nil {
		return fmt.Errorf("agenttools: tool access: %w", err)
	}
	if !on {
		return ErrToolsDisabled
	}
	return nil
}

type toolDef struct {
	name        string
	description string
	schema      json.RawMessage
	run         func(ctx context.Context, c Caller, args json.RawMessage) (string, error)
}

// ToolSet is built PER REQUEST around one caller's credentials, which is what
// makes it impossible to reach a tool handler without a successful Resolve —
// there is no code path that calls run() with an unauthenticated Caller.
type ToolSet struct {
	deps     Deps
	runnerID string
	token    string
	defs     []toolDef
}

func NewToolSet(deps Deps, runnerID, token string) *ToolSet {
	ts := &ToolSet{deps: deps, runnerID: runnerID, token: token}
	ts.defs = append(ts.defs, chatTools(deps)...)
	ts.defs = append(ts.defs, reviewTools(deps)...)
	ts.defs = append(ts.defs, contextTools(deps)...)
	return ts
}

func (t *ToolSet) Tools() []mcp.Tool {
	out := make([]mcp.Tool, 0, len(t.defs))
	for _, d := range t.defs {
		out = append(out, mcp.Tool{Name: d.name, Description: d.description, InputSchema: d.schema})
	}
	return out
}

// UnknownToolMetric is the single Metrics bucket every call naming no
// registered tool is counted under. See ToolSet.metricName.
const UnknownToolMetric = "<unknown>"

// Call resolves the caller, then dispatches name to its handler.
//
// Every attempt is recorded — including the ones that never reach a handler —
// via a single deferred Record covering every return path. Keeping the
// pre-dispatch rejections is deliberate: Resolve runs before name is checked
// against t.defs, and an unauthorized or out-of-scope attempt at a SPECIFIC
// tool is exactly the datum this counter exists to surface, so it must not be
// folded into one generic "rejected" bucket.
//
// What IS folded is a name matching no registered tool, because name is
// arbitrary agent-supplied text and the counters are a map keyed on it: a
// hallucinating model could otherwise grow that map without bound, one entry
// per invented name, for the lifetime of the daemon. Real tools keep their own
// attribution; everything else lands in UnknownToolMetric. See metricName.
//
// The provider's tool switch is checked HERE, per call, immediately after the
// caller is resolved and before any handler runs. It cannot be checked at spawn
// and remembered: the vendor CLI holds its MCP channel for the life of the
// process, so a chat spawned while the switch was on would otherwise keep full
// tool access forever after the user turned it off — a permission the UI
// promises and nothing enforced. The check runs after Resolve because the
// provider comes off the RESOLVED caller, never off anything the relay said.
func (t *ToolSet) Call(ctx context.Context, name string, args json.RawMessage) (result string, err error) {
	defer func() { t.deps.Metrics.Record(t.metricName(name), err == nil) }()

	caller, err := t.deps.Resolver.Resolve(ctx, t.runnerID, t.token)
	if err != nil {
		return "", err
	}
	// `=`, not `:=`: a shadowed err here would leave the deferred Record above
	// counting a refused call as a success.
	if err = t.deps.refuseDisabledTools(ctx, caller.ProviderID); err != nil {
		return "", err
	}
	for _, d := range t.defs {
		if d.name != name {
			continue
		}
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		return d.run(ctx, caller, args)
	}
	return "", fmt.Errorf("agenttools: unknown tool %q", name)
}

// metricName is the Metrics key a call to name is counted under: name itself
// when it names a tool this set actually registered, and UnknownToolMetric
// otherwise.
//
// The bound is the point. Record runs on a defer that fires before name has
// been validated against t.defs, and the caller of Call is a model — so without
// this the counter map is an unbounded allocation keyed on text an agent
// invents. Checking against t.defs (not a hardcoded list) also means the bucket
// tracks whatever this particular ToolSet registered: a tool suppressed by a
// missing dependency is genuinely not a tool this caller has, and an attempt at
// it is not something to attribute per-name.
func (t *ToolSet) metricName(name string) string {
	for _, d := range t.defs {
		if d.name == name {
			return name
		}
	}
	return UnknownToolMetric
}

// decode unmarshals a tool's arguments, turning a decode failure into a message
// the model can act on rather than a bare syntax error.
func decode(args json.RawMessage, into any) error {
	if err := json.Unmarshal(args, into); err != nil {
		return fmt.Errorf("agenttools: arguments are not valid for this tool: %w", err)
	}
	return nil
}
