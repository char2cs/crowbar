package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools/internal/render"
	"github.com/char2cs/crowbar/api/internal/engine/mcp"
)

// ChatTurn is one turn of a chat as the tool surface renders it. It is the
// renderer's shape, named here because it is what this package's callers see.
type ChatTurn = render.ChatTurn

// Deps is everything the tool surface needs from the rest of the app. Later
// phases extend this struct; a nil dependency simply means the tools that need
// it are not registered.
type Deps struct {
	// Resolver is built by the chat usecase from Scope and its own runners.
	Resolver *Resolver
	// Scope is the workspace store a caller is scoped against.
	Scope        WorkspaceLister
	Chats        ChatRenamer
	Workspaces   WorkspaceBranchRenamer
	Review       ReviewReader
	Threads      ThreadReader
	ThreadWrites ThreadWriter
	// ThreadChanges wakes wait_for_review_comments; nil withdraws that tool.
	ThreadChanges ThreadChanges
	// Idempotency is the per-daemon retry guard the write tools dedup through. It
	// is a dependency rather than something a tool creates for itself because it
	// must outlive the per-request ToolSet — see Idempotency.
	Idempotency *Idempotency
	// Browser routes the browser_* tools to the desktop app; nil withdraws them.
	Browser BrowserBridge
	// ThreadBroadcast fans an agent-written review thread out to connected clients.
	ThreadBroadcast ThreadBroadcast
	// ChatReads lets list_workspaces list chats and get_chat_log resolve a chat id to its workspace.
	ChatReads ChatReader
	// ChatLogs reads another chat's ledger for get_chat_log, only after its scope checks pass.
	ChatLogs ChatLogReader
	// Lineage is the Chats-tree authority get_chat_log refuses a caller's own descendants with; nil
	// withdraws get_chat_log rather than serving it without that refusal.
	Lineage ChatLineageReader
	// Metrics counts calls and failures per tool. Unlike the ports above, nil never suppresses a tool:
	// it is read only through the nil-safe Metrics.Record.
	Metrics *Metrics
	// ToolAccess reports the user's per-provider "Tools" switch, consulted on every call: the CLI keeps
	// its MCP channel for the process's life, so a spawn-time read would outlive the user turning it off.
	ToolAccess ProviderToolAccess
}

// ProviderToolAccess answers whether one provider's agents may reach Crowbar's tools right now. It takes
// the provider, not the caller, since the preference is per provider and needs no authority model.
type ProviderToolAccess func(ctx context.Context, providerID string) (bool, error)

// refuseDisabledTools rejects a call whose provider has tools switched off, treating a read failure as a
// refusal. A nil ToolAccess (only a hand-built test Deps) leaves every registered tool callable.
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
	// runImage replaces run for a tool that answers with an image as well.
	runImage func(ctx context.Context, c Caller, args json.RawMessage) (string, []mcp.Image, error)
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
	ts.defs = append(ts.defs, branchNameTools(deps)...)
	ts.defs = append(ts.defs, reviewTools(deps)...)
	ts.defs = append(ts.defs, contextTools(deps)...)
	ts.defs = append(ts.defs, browserTools(deps)...)
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

// Call resolves the caller, checks its provider's tool switch, then dispatches name to its handler.
// Every attempt is counted per tool, including rejections; the switch is read off the resolved caller,
// never anything the relay said.
func (t *ToolSet) Call(ctx context.Context, name string, args json.RawMessage) (string, error) {
	text, _, err := t.CallWithImages(ctx, name, args)
	return text, err
}

// CallWithImages is Call for tools that may answer with images too.
func (t *ToolSet) CallWithImages(
	ctx context.Context,
	name string,
	args json.RawMessage,
) (result string, images []mcp.Image, err error) {
	defer func() { t.deps.Metrics.Record(t.metricName(name), err == nil) }()

	caller, err := t.deps.Resolver.Resolve(ctx, t.runnerID, t.token)
	if err != nil {
		return "", nil, err
	}
	// `=`, not `:=`: a shadowed err here would leave the deferred Record above
	// counting a refused call as a success.
	if err = t.deps.refuseDisabledTools(ctx, caller.ProviderID); err != nil {
		return "", nil, err
	}
	for _, d := range t.defs {
		if d.name != name {
			continue
		}
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		if d.runImage != nil {
			return d.runImage(ctx, caller, args)
		}
		text, err := d.run(ctx, caller, args)
		return text, nil, err
	}
	return "", nil, fmt.Errorf("agenttools: unknown tool %q", name)
}

// metricName is the Metrics key for name: itself if this set registered it, else UnknownToolMetric, so
// model-invented names cannot grow the counter map without bound.
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
