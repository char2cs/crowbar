// Package tools provides the MCP tool handler functions registered on the
// crowbar MCP server. Each file covers one logical domain (signal, items,
// threads). All handlers call auth.Resolve to establish the caller's identity
// before executing any business logic.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	mcpinternal "github.com/char2cs/crowbar/api/internal/app/repositories/mcp/internal"
	"github.com/char2cs/crowbar/api/internal/app/repositories/mcp/internal/auth"
	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
	"github.com/char2cs/crowbar/api/internal/engine/flow/evaluator"
)

// tokenContextKey is the context key used to store the X-Agent-Run-Token
// value injected via the mcp-go HTTPContextFunc.
type tokenContextKey struct{}

// WithToken returns a context carrying the agent run token.
func WithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenContextKey{}, token)
}

// TokenFromContext retrieves the agent run token stored by WithToken.
func TokenFromContext(ctx context.Context) string {
	v, _ := ctx.Value(tokenContextKey{}).(string)
	return v
}

// TokenFromRequest extracts the X-Agent-Run-Token header from an HTTP request
// and stores it in the context. This is the HTTPContextFunc passed to the
// mcp-go StreamableHTTPServer.
func TokenFromRequest(ctx context.Context, r *http.Request) context.Context {
	token := r.Header.Get("X-Agent-Run-Token")
	return WithToken(ctx, token)
}

// toolError wraps an error into a tool result with isError=true.
func toolError(err error) *mcplib.CallToolResult {
	result := mcplib.NewToolResultText(err.Error())
	result.IsError = true
	return result
}

// --- crowbar_signal ---

// SignalResult is the JSON-serialisable response from crowbar_signal.
type SignalResult struct {
	OK        bool   `json:"ok"`
	NextState string `json:"next_state"`
}

// NewSignalTool returns the mcp-go Tool definition for crowbar_signal.
func NewSignalTool() mcplib.Tool {
	return mcplib.NewTool(
		"crowbar_signal",
		mcplib.WithDescription(
			"Trigger a state transition. Call this when the current task state is complete. "+
				"'event' must match a transition declared in the current state. "+
				"'output' is an optional markdown summary stored on the agent run and injected as context in future states.",
		),
		mcplib.WithString("event",
			mcplib.Required(),
			mcplib.Description("The transition event name (e.g. 'done', 'approved', 'rejected')."),
		),
		mcplib.WithString("output",
			mcplib.Description("Optional markdown summary of work completed in this state."),
		),
	)
}

// NewSignalHandler constructs the crowbar_signal ToolHandlerFunc.
func NewSignalHandler(
	agentRunRepo mcpinternal.AgentRunReader,
	taskRepo mcpinternal.TaskReader,
	chatHub hub.ChatHub,
	flowLoader flow.Loader,
) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		token := TokenFromContext(ctx)
		actx, err := auth.Resolve(ctx, token, agentRunRepo, taskRepo, flowLoader)
		if err != nil {
			return toolError(err), nil
		}

		if !auth.HasTool(actx, flow.ToolCrowbarSignal) {
			return toolError(auth.ErrForbidden), nil
		}

		event, _ := req.GetArguments()["event"].(string)
		output, _ := req.GetArguments()["output"].(string)

		nextState, ok := evaluator.Evaluate(actx.Flow, actx.Task.CurrentState, event)
		if !ok {
			return toolError(fmt.Errorf("invalid event %q for current state %q", event, actx.Task.CurrentState)), nil
		}

		if err := agentRunRepo.CompleteAgentRun(ctx, actx.AgentRun.ID, output); err != nil {
			return toolError(fmt.Errorf("crowbar_signal: complete agent run: %w", err)), nil
		}

		chatHub.Publish(actx.Task.ID, agent.ChatFrame{
			Type:     agent.ChatFrameTypeStateTransition,
			NewState: nextState,
		})

		if err := taskRepo.AdvanceState(ctx, actx.Task.ID, nextState, event); err != nil {
			return toolError(fmt.Errorf("crowbar_signal: advance state: %w", err)), nil
		}

		result := SignalResult{OK: true, NextState: nextState}
		data, err := json.Marshal(result)
		if err != nil {
			return toolError(fmt.Errorf("crowbar_signal: marshal result: %w", err)), nil
		}
		return mcplib.NewToolResultText(string(data)), nil
	}
}
