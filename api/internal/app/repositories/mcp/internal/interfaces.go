// Package internal defines the minimal repository interfaces used by the MCP
// server's subpackages. Defining them here — rather than importing from
// app/repositories — breaks the import cycle that would otherwise arise because
// app/repositories/container.go imports this (mcp) package.
package internal

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// AgentRunReader is the subset of the AgentRun repository required by the MCP
// auth and tool packages.
type AgentRunReader interface {
	GetByToken(ctx context.Context, token string) (domain.AgentRun, error)
	CompleteAgentRun(ctx context.Context, id string, output string) error
}

// TaskReader is the subset of the Task repository required by the MCP auth and
// tool packages.
type TaskReader interface {
	Get(ctx context.Context, id string) (domain.Task, error)
	AdvanceState(ctx context.Context, id string, nextState string, event string) error
}

// KanbanItemAccessor is the subset of the KanbanItem repository required by
// the MCP items tools.
type KanbanItemAccessor interface {
	Create(ctx context.Context, taskID string, title string, agentRunID string) (domain.KanbanItem, error)
	Get(ctx context.Context, id string) (domain.KanbanItem, error)
	ListByTask(ctx context.Context, taskID string) ([]domain.KanbanItem, error)
	UpdateStatus(ctx context.Context, id string, status string, agentRunID string) error
}

// ReviewThreadAccessor is the subset of the ReviewThread repository required by
// the MCP threads tools.
type ReviewThreadAccessor interface {
	Open(ctx context.Context, taskID string, agentRunID *string, file string, line int, phase domain.ReviewPhase, openedBy string, content string) (domain.ReviewThread, error)
	Get(ctx context.Context, id string) (domain.ReviewThread, error)
	ListByTask(ctx context.Context, taskID string, status string, phase string) ([]domain.ReviewThread, error)
	PostMessage(ctx context.Context, threadID string, role string, content string) error
	ResolveThread(ctx context.Context, threadID string, emoji string) error
}
