package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools/internal/render"
	"github.com/char2cs/crowbar/api/internal/engine/mcp"
)

type ChatTurn = render.ChatTurn

type Deps struct {
	Resolver        *Resolver
	Scope           WorkspaceLister
	Chats           ChatRenamer
	Workspaces      WorkspaceBranchRenamer
	Branches        BranchOuter
	Review          ReviewReader
	Threads         ThreadReader
	ThreadWrites    ThreadWriter
	ThreadChanges   ThreadChanges
	Idempotency     *Idempotency
	Browser         BrowserBridge
	ThreadBroadcast ThreadBroadcast
	ChatReads       ChatReader
	ChatLogs        ChatLogReader
	Lineage         ChatLineageReader
	Metrics         *Metrics
	ToolAccess      ProviderToolAccess
}

type ProviderToolAccess func(ctx context.Context, providerID string) (bool, error)

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
	runImage    func(ctx context.Context, c Caller, args json.RawMessage) (string, []mcp.Image, error)
}

type ToolSet struct {
	deps    Deps
	resolve func(context.Context) (Caller, error)
	defs    []toolDef
}

func NewToolSet(deps Deps, runnerID, token string) *ToolSet {
	return newToolSet(deps, func(ctx context.Context) (Caller, error) {
		return deps.Resolver.Resolve(ctx, runnerID, token)
	})
}

func NewRelayToolSet(deps Deps, relayID, token string) *ToolSet {
	return newToolSet(deps, func(ctx context.Context) (Caller, error) {
		return deps.Resolver.ResolveRelay(ctx, relayID, token)
	})
}

func newToolSet(deps Deps, resolve func(context.Context) (Caller, error)) *ToolSet {
	ts := &ToolSet{deps: deps, resolve: resolve}
	ts.defs = append(ts.defs, chatTools(deps)...)
	ts.defs = append(ts.defs, branchNameTools(deps)...)
	ts.defs = append(ts.defs, branchOutTools(deps)...)
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

const UnknownToolMetric = "<unknown>"

func (t *ToolSet) Call(ctx context.Context, name string, args json.RawMessage) (string, error) {
	text, _, err := t.CallWithImages(ctx, name, args)
	return text, err
}

func (t *ToolSet) CallWithImages(
	ctx context.Context,
	name string,
	args json.RawMessage,
) (result string, images []mcp.Image, err error) {
	defer func() { t.deps.Metrics.Record(t.metricName(name), err == nil) }()

	caller, err := t.resolve(ctx)
	if err != nil {
		return "", nil, err
	}
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

func (t *ToolSet) metricName(name string) string {
	for _, d := range t.defs {
		if d.name == name {
			return name
		}
	}
	return UnknownToolMetric
}

func decode(args json.RawMessage, into any) error {
	if err := json.Unmarshal(args, into); err != nil {
		return fmt.Errorf("agenttools: arguments are not valid for this tool: %w", err)
	}
	return nil
}
