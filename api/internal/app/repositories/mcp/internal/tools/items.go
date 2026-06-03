package tools

import (
	"context"
	"encoding/json"
	"fmt"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	mcpinternal "github.com/char2cs/crowbar/api/internal/app/repositories/mcp/internal"
	"github.com/char2cs/crowbar/api/internal/app/repositories/mcp/internal/auth"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

// --- crowbar_create_item ---

// NewCreateItemTool returns the mcp-go Tool definition for crowbar_create_item.
func NewCreateItemTool() mcplib.Tool {
	return mcplib.NewTool(
		"crowbar_create_item",
		mcplib.WithDescription(
			"Create a new KanbanItem for the current task. Only callable in states where items tracking is enabled.",
		),
		mcplib.WithString("title",
			mcplib.Required(),
			mcplib.Description("Short title for the kanban item."),
		),
		mcplib.WithString("description",
			mcplib.Description("Optional longer description of the item."),
		),
	)
}

// NewCreateItemHandler constructs the crowbar_create_item ToolHandlerFunc.
func NewCreateItemHandler(
	agentRunRepo mcpinternal.AgentRunReader,
	taskRepo mcpinternal.TaskReader,
	kanbanRepo mcpinternal.KanbanItemAccessor,
	flowLoader flow.Loader,
) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		token := TokenFromContext(ctx)
		actx, err := auth.Resolve(ctx, token, agentRunRepo, taskRepo, flowLoader)
		if err != nil {
			return toolError(err), nil
		}

		if !actx.State.Items {
			return toolError(auth.ErrForbidden), nil
		}

		title, _ := req.GetArguments()["title"].(string)

		item, err := kanbanRepo.Create(ctx, actx.Task.ID, title, actx.AgentRun.ID)
		if err != nil {
			return toolError(fmt.Errorf("crowbar_create_item: create: %w", err)), nil
		}

		data, err := json.Marshal(map[string]string{"item_id": item.ID})
		if err != nil {
			return toolError(fmt.Errorf("crowbar_create_item: marshal: %w", err)), nil
		}
		return mcplib.NewToolResultText(string(data)), nil
	}
}

// --- crowbar_update_item_status ---

// NewUpdateItemStatusTool returns the mcp-go Tool definition for
// crowbar_update_item_status.
func NewUpdateItemStatusTool() mcplib.Tool {
	return mcplib.NewTool(
		"crowbar_update_item_status",
		mcplib.WithDescription(
			"Update the status of a KanbanItem. The status must be one of the values declared in the flow's ItemStatuses list.",
		),
		mcplib.WithString("item_id",
			mcplib.Required(),
			mcplib.Description("The ID of the item to update."),
		),
		mcplib.WithString("status",
			mcplib.Required(),
			mcplib.Description("The new status value."),
		),
	)
}

// NewUpdateItemStatusHandler constructs the crowbar_update_item_status
// ToolHandlerFunc.
func NewUpdateItemStatusHandler(
	agentRunRepo mcpinternal.AgentRunReader,
	taskRepo mcpinternal.TaskReader,
	kanbanRepo mcpinternal.KanbanItemAccessor,
	flowLoader flow.Loader,
) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		token := TokenFromContext(ctx)
		actx, err := auth.Resolve(ctx, token, agentRunRepo, taskRepo, flowLoader)
		if err != nil {
			return toolError(err), nil
		}

		itemID, _ := req.GetArguments()["item_id"].(string)
		status, _ := req.GetArguments()["status"].(string)

		item, err := kanbanRepo.Get(ctx, itemID)
		if err != nil {
			return toolError(fmt.Errorf("crowbar_update_item_status: get item: %w", err)), nil
		}

		if item.TaskID != actx.Task.ID {
			return toolError(auth.ErrForbidden), nil
		}

		validStatus := false
		for _, s := range actx.Flow.ItemStatuses {
			if s == status {
				validStatus = true
				break
			}
		}
		if !validStatus {
			return toolError(fmt.Errorf("crowbar_update_item_status: unknown status %q", status)), nil
		}

		if err := kanbanRepo.UpdateStatus(ctx, itemID, status, actx.AgentRun.ID); err != nil {
			return toolError(fmt.Errorf("crowbar_update_item_status: update: %w", err)), nil
		}

		data, _ := json.Marshal(map[string]bool{"ok": true})
		return mcplib.NewToolResultText(string(data)), nil
	}
}

// --- crowbar_get_items ---

// NewGetItemsTool returns the mcp-go Tool definition for crowbar_get_items.
func NewGetItemsTool() mcplib.Tool {
	return mcplib.NewTool(
		"crowbar_get_items",
		mcplib.WithDescription("List all KanbanItems for the current task."),
	)
}

// NewGetItemsHandler constructs the crowbar_get_items ToolHandlerFunc.
func NewGetItemsHandler(
	agentRunRepo mcpinternal.AgentRunReader,
	taskRepo mcpinternal.TaskReader,
	kanbanRepo mcpinternal.KanbanItemAccessor,
	flowLoader flow.Loader,
) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		token := TokenFromContext(ctx)
		actx, err := auth.Resolve(ctx, token, agentRunRepo, taskRepo, flowLoader)
		if err != nil {
			return toolError(err), nil
		}

		items, err := kanbanRepo.ListByTask(ctx, actx.Task.ID)
		if err != nil {
			return toolError(fmt.Errorf("crowbar_get_items: list: %w", err)), nil
		}

		data, err := json.Marshal(items)
		if err != nil {
			return toolError(fmt.Errorf("crowbar_get_items: marshal: %w", err)), nil
		}
		return mcplib.NewToolResultText(string(data)), nil
	}
}
