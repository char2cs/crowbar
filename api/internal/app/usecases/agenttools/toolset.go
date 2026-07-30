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

// Call resolves the caller, then dispatches name to its handler.
//
// Every attempt is recorded against the literal name the caller asked for —
// including the ones that never reach a handler — via a single deferred
// Record covering every return path. That attribution is deliberate: Resolve
// runs before name is checked against t.defs, so at the point a resolve
// failure returns, name has not been validated as a real tool. Counting it
// anyway, rather than folding every pre-dispatch rejection into one generic
// bucket, is what lets an unauthorized or out-of-scope attempt at a SPECIFIC
// tool be told apart from one aimed at another — the whole point of this
// counter is to surface exactly that kind of attempt.
func (t *ToolSet) Call(ctx context.Context, name string, args json.RawMessage) (result string, err error) {
	defer func() { t.deps.Metrics.Record(name, err == nil) }()

	caller, err := t.deps.Resolver.Resolve(ctx, t.runnerID, t.token)
	if err != nil {
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

// decode unmarshals a tool's arguments, turning a decode failure into a message
// the model can act on rather than a bare syntax error.
func decode(args json.RawMessage, into any) error {
	if err := json.Unmarshal(args, into); err != nil {
		return fmt.Errorf("agenttools: arguments are not valid for this tool: %w", err)
	}
	return nil
}
