package repositories

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/adapter"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
)

// ReadModels are the durable read models. They are opened before the asynx
// instances because each is its instance's durable subscriber.
type ReadModels struct {
	Workspace    *workspace.ReadModel
	ReviewThread *reviewthread.ReadModel
	AgentChat    *agentchat.ReadModel
}

// OpenReadModels opens every read model over its view DB.
func OpenReadModels(
	adapters *adapter.Container,
) (*ReadModels, error) {
	ws, err := workspace.OpenReadModel(adapters.WorkspaceView())
	if err != nil {
		return nil, fmt.Errorf("repositories: %w", err)
	}
	rt, err := reviewthread.OpenReadModel(adapters.ReviewThreadView())
	if err != nil {
		return nil, fmt.Errorf("repositories: %w", err)
	}
	chat, err := agentchat.OpenReadModel(adapters.AgentChatReadDB())
	if err != nil {
		return nil, fmt.Errorf("repositories: %w", err)
	}
	return &ReadModels{Workspace: ws, ReviewThread: rt, AgentChat: chat}, nil
}
