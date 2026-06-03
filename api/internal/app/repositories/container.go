package repositories

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/adapter"
	appHub "github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/app/dispatcher"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/conversation"
	"github.com/char2cs/crowbar/api/internal/app/repositories/kanbanitem"
	"github.com/char2cs/crowbar/api/internal/app/repositories/mcp"
	"github.com/char2cs/crowbar/api/internal/app/repositories/project"
	"github.com/char2cs/crowbar/api/internal/app/repositories/repository"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/task"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// Container holds all fully-constructed repositories.
type Container struct {
	Project      project.Project
	Repository   repository.Repository
	Conversation conversation.Conversation
	Task         task.Task
	AgentRun     agentrun.AgentRun
	KanbanItem   kanbanitem.KanbanItem
	ReviewThread reviewthread.ReviewThread
	MCP          mcp.MCPRepository
}

// New constructs all repositories, wires Asynx instances and GORM repos,
// builds the MCP server, and runs orphaned-run recovery before returning.
func New(
	ctx context.Context,
	engines *engine.Container,
	adapters *adapter.Container,
	chatHub appHub.ChatHub,
) (*Container, error) {
	axTask, err := newAsynx[domain.Task](adapters.TaskES)
	if err != nil {
		return nil, fmt.Errorf("repositories: asynx task: %w", err)
	}

	axAgentRun, err := newAsynx[domain.AgentRun](adapters.AgentRunES)
	if err != nil {
		return nil, fmt.Errorf("repositories: asynx agent_run: %w", err)
	}

	axKanban, err := newAsynx[domain.KanbanItem](adapters.KanbanItemES)
	if err != nil {
		return nil, fmt.Errorf("repositories: asynx kanban_item: %w", err)
	}

	axThread, err := newAsynx[domain.ReviewThread](adapters.ThreadES)
	if err != nil {
		return nil, fmt.Errorf("repositories: asynx review_thread: %w", err)
	}

	projectRepo, err := project.New(adapters.Store)
	if err != nil {
		return nil, fmt.Errorf("repositories: project: %w", err)
	}

	repoRepo, err := repository.New(adapters.Store)
	if err != nil {
		return nil, fmt.Errorf("repositories: repository: %w", err)
	}

	convRepo, err := conversation.New(adapters.Store)
	if err != nil {
		return nil, fmt.Errorf("repositories: conversation: %w", err)
	}

	taskRepo := task.New(axTask)
	agentRunRepo := agentrun.New(
		axAgentRun,
		agentrun.WithIDProvider(adapters.AgentRunES.ListAggregateIDs),
		agentrun.WithTaskPauser(func(ctx context.Context, taskID string) error {
			if err := taskRepo.Pause(ctx, taskID); err != nil {
				slog.WarnContext(ctx, "repositories: pause task on run recovery", "task_id", taskID, "err", err)
			}
			return nil
		}),
	)
	kanbanRepo := kanbanitem.New(axKanban)
	threadRepo := reviewthread.New(axThread)

	mcpRepo := mcp.New(agentRunRepo, taskRepo, kanbanRepo, threadRepo, chatHub, engines.FlowLoader)

	if err := agentRunRepo.RecoverOrphanedRuns(ctx); err != nil {
		return nil, fmt.Errorf("repositories: recover orphaned runs: %w", err)
	}

	disp := dispatcher.New(ctx, taskRepo, agentRunRepo, engines.FlowLoader, engines.AgentRuntime)
	if err := disp.Register(); err != nil {
		return nil, fmt.Errorf("repositories: dispatcher: %w", err)
	}

	return &Container{
		Project:      projectRepo,
		Repository:   repoRepo,
		Conversation: convRepo,
		Task:         taskRepo,
		AgentRun:     agentRunRepo,
		KanbanItem:   kanbanRepo,
		ReviewThread: threadRepo,
		MCP:          mcpRepo,
	}, nil
}

// RegisterHubProjections wires Asynx event subscriptions to hub broadcast calls.
func (c *Container) RegisterHubProjections(
	hub appHub.WebSocketHub,
) error {
	if _, err := c.Task.Subscribe("task.*", func(ctx context.Context, evt asynxModels.Event[domain.Task]) {
		hub.BroadcastTask(evt.Aggregate)
	}); err != nil {
		return fmt.Errorf("repositories: subscribe task: %w", err)
	}

	if _, err := c.AgentRun.Subscribe("agent_run.*", func(ctx context.Context, evt asynxModels.Event[domain.AgentRun]) {
		hub.BroadcastAgentRun(evt.Aggregate)
	}); err != nil {
		return fmt.Errorf("repositories: subscribe agent_run: %w", err)
	}

	if _, err := c.KanbanItem.Subscribe("kanban_item.*", func(ctx context.Context, evt asynxModels.Event[domain.KanbanItem]) {
		hub.BroadcastKanbanItem(evt.Aggregate)
	}); err != nil {
		return fmt.Errorf("repositories: subscribe kanban_item: %w", err)
	}

	if _, err := c.ReviewThread.Subscribe("review_thread.*", func(ctx context.Context, evt asynxModels.Event[domain.ReviewThread]) {
		hub.BroadcastReviewThread(evt.Aggregate)
	}); err != nil {
		return fmt.Errorf("repositories: subscribe review_thread: %w", err)
	}

	return nil
}

func newAsynx[T any](
	es asynxModels.Store,
) (asynx.Asynx[T], error) {
	return asynx.New[T]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}
