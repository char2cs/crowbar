package tools

import (
	"context"
	"encoding/json"
	"fmt"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	mcpinternal "github.com/char2cs/crowbar/api/internal/app/repositories/mcp/internal"
	"github.com/char2cs/crowbar/api/internal/app/repositories/mcp/internal/auth"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

// derivePhase maps a state name to a ReviewPhase. The ai_review state maps to
// ReviewPhaseAIReview; all other states fall back to ReviewPhaseHumanReview.
func derivePhase(stateName string) domain.ReviewPhase {
	if stateName == "ai_review" {
		return domain.ReviewPhaseAIReview
	}
	return domain.ReviewPhaseHumanReview
}

// deriveRole maps a state name to a review message role.
func deriveRole(stateName string) string {
	switch stateName {
	case "ai_review":
		return "reviewer"
	case "implementation":
		return "implementer"
	default:
		return "reviewer"
	}
}

// --- crowbar_open_thread ---

// NewOpenThreadTool returns the mcp-go Tool definition for crowbar_open_thread.
func NewOpenThreadTool() mcplib.Tool {
	return mcplib.NewTool(
		"crowbar_open_thread",
		mcplib.WithDescription(
			"Open a review thread anchored to a file and line. Use this to flag code for discussion. "+
				"Phase and opener are derived automatically from the calling agent's state.",
		),
		mcplib.WithString("file",
			mcplib.Required(),
			mcplib.Description("Relative path to the file being commented on."),
		),
		mcplib.WithNumber("line",
			mcplib.Required(),
			mcplib.Description("Line number in the file."),
		),
		mcplib.WithString("content",
			mcplib.Required(),
			mcplib.Description("The opening comment for the thread."),
		),
	)
}

// NewOpenThreadHandler constructs the crowbar_open_thread ToolHandlerFunc.
func NewOpenThreadHandler(
	agentRunRepo mcpinternal.AgentRunReader,
	taskRepo mcpinternal.TaskReader,
	threadRepo mcpinternal.ReviewThreadAccessor,
	flowLoader flow.Loader,
) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		token := TokenFromContext(ctx)
		actx, err := auth.Resolve(ctx, token, agentRunRepo, taskRepo, flowLoader)
		if err != nil {
			return toolError(err), nil
		}

		file, _ := req.GetArguments()["file"].(string)
		lineFloat, _ := req.GetArguments()["line"].(float64)
		line := int(lineFloat)
		content, _ := req.GetArguments()["content"].(string)

		phase := derivePhase(actx.State.Name)
		agentRunID := actx.AgentRun.ID

		thread, err := threadRepo.Open(ctx, actx.Task.ID, &agentRunID, file, line, phase, "reviewer", content)
		if err != nil {
			return toolError(fmt.Errorf("crowbar_open_thread: open: %w", err)), nil
		}

		data, err := json.Marshal(map[string]string{"thread_id": thread.ID})
		if err != nil {
			return toolError(fmt.Errorf("crowbar_open_thread: marshal: %w", err)), nil
		}
		return mcplib.NewToolResultText(string(data)), nil
	}
}

// --- crowbar_reply_thread ---

// NewReplyThreadTool returns the mcp-go Tool definition for
// crowbar_reply_thread.
func NewReplyThreadTool() mcplib.Tool {
	return mcplib.NewTool(
		"crowbar_reply_thread",
		mcplib.WithDescription(
			"Append a reply to an existing review thread. Role is inferred from the calling agent's state.",
		),
		mcplib.WithString("thread_id",
			mcplib.Required(),
			mcplib.Description("The ID of the thread to reply to."),
		),
		mcplib.WithString("content",
			mcplib.Required(),
			mcplib.Description("The reply content."),
		),
	)
}

// NewReplyThreadHandler constructs the crowbar_reply_thread ToolHandlerFunc.
func NewReplyThreadHandler(
	agentRunRepo mcpinternal.AgentRunReader,
	taskRepo mcpinternal.TaskReader,
	threadRepo mcpinternal.ReviewThreadAccessor,
	flowLoader flow.Loader,
) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		token := TokenFromContext(ctx)
		actx, err := auth.Resolve(ctx, token, agentRunRepo, taskRepo, flowLoader)
		if err != nil {
			return toolError(err), nil
		}

		threadID, _ := req.GetArguments()["thread_id"].(string)
		content, _ := req.GetArguments()["content"].(string)

		thread, err := threadRepo.Get(ctx, threadID)
		if err != nil {
			return toolError(fmt.Errorf("crowbar_reply_thread: get thread: %w", err)), nil
		}

		if thread.TaskID != actx.Task.ID {
			return toolError(auth.ErrForbidden), nil
		}

		role := deriveRole(actx.State.Name)

		if err := threadRepo.PostMessage(ctx, threadID, role, content); err != nil {
			return toolError(fmt.Errorf("crowbar_reply_thread: post message: %w", err)), nil
		}

		data, _ := json.Marshal(map[string]bool{"ok": true})
		return mcplib.NewToolResultText(string(data)), nil
	}
}

// --- crowbar_get_threads ---

// NewGetThreadsTool returns the mcp-go Tool definition for crowbar_get_threads.
func NewGetThreadsTool() mcplib.Tool {
	return mcplib.NewTool(
		"crowbar_get_threads",
		mcplib.WithDescription("List review threads for the current task. Optionally filter by status and phase."),
		mcplib.WithString("status",
			mcplib.Description("Filter by thread status: 'open', 'agreed', or 'force_approved'. Leave empty for all."),
		),
		mcplib.WithString("phase",
			mcplib.Description("Filter by review phase: 'ai_review' or 'human_review'. Leave empty for all."),
		),
	)
}

// NewGetThreadsHandler constructs the crowbar_get_threads ToolHandlerFunc.
func NewGetThreadsHandler(
	agentRunRepo mcpinternal.AgentRunReader,
	taskRepo mcpinternal.TaskReader,
	threadRepo mcpinternal.ReviewThreadAccessor,
	flowLoader flow.Loader,
) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		token := TokenFromContext(ctx)
		actx, err := auth.Resolve(ctx, token, agentRunRepo, taskRepo, flowLoader)
		if err != nil {
			return toolError(err), nil
		}

		status, _ := req.GetArguments()["status"].(string)
		phase, _ := req.GetArguments()["phase"].(string)

		threads, err := threadRepo.ListByTask(ctx, actx.Task.ID, status, phase)
		if err != nil {
			return toolError(fmt.Errorf("crowbar_get_threads: list: %w", err)), nil
		}

		data, err := json.Marshal(threads)
		if err != nil {
			return toolError(fmt.Errorf("crowbar_get_threads: marshal: %w", err)), nil
		}
		return mcplib.NewToolResultText(string(data)), nil
	}
}

// --- crowbar_resolve_thread ---

// NewResolveThreadTool returns the mcp-go Tool definition for
// crowbar_resolve_thread.
func NewResolveThreadTool() mcplib.Tool {
	return mcplib.NewTool(
		"crowbar_resolve_thread",
		mcplib.WithDescription(
			"Mark a review thread as agreed. If the thread is already resolved this is a no-op. "+
				"An optional emoji is rendered as a react bubble in the UI.",
		),
		mcplib.WithString("thread_id",
			mcplib.Required(),
			mcplib.Description("The ID of the thread to resolve."),
		),
		mcplib.WithString("emoji",
			mcplib.Description("Optional emoji shown as a react bubble (e.g. '✅')."),
		),
	)
}

// NewResolveThreadHandler constructs the crowbar_resolve_thread
// ToolHandlerFunc.
func NewResolveThreadHandler(
	agentRunRepo mcpinternal.AgentRunReader,
	taskRepo mcpinternal.TaskReader,
	threadRepo mcpinternal.ReviewThreadAccessor,
	flowLoader flow.Loader,
) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		token := TokenFromContext(ctx)
		actx, err := auth.Resolve(ctx, token, agentRunRepo, taskRepo, flowLoader)
		if err != nil {
			return toolError(err), nil
		}

		threadID, _ := req.GetArguments()["thread_id"].(string)
		emoji, _ := req.GetArguments()["emoji"].(string)

		thread, err := threadRepo.Get(ctx, threadID)
		if err != nil {
			return toolError(fmt.Errorf("crowbar_resolve_thread: get thread: %w", err)), nil
		}

		if thread.TaskID != actx.Task.ID {
			return toolError(auth.ErrForbidden), nil
		}

		// Already resolved — return success as a no-op.
		if thread.Status != domain.ReviewThreadStatusOpen {
			data, _ := json.Marshal(map[string]bool{"ok": true})
			return mcplib.NewToolResultText(string(data)), nil
		}

		if err := threadRepo.ResolveThread(ctx, threadID, emoji); err != nil {
			return toolError(fmt.Errorf("crowbar_resolve_thread: resolve: %w", err)), nil
		}

		data, _ := json.Marshal(map[string]bool{"ok": true})
		return mcplib.NewToolResultText(string(data)), nil
	}
}
