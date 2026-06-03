package hub

import "github.com/char2cs/crowbar/api/internal/domain"

// WebSocketHub is the app-layer interface for broadcasting domain events to
// connected WebSocket clients and registering new subscribers. Defined here so
// app builders can depend on it without importing the api layer.
type WebSocketHub interface {
	// Register adds a Subscriber to the set that receives domain broadcasts.
	Register(s Subscriber)

	BroadcastTask(t domain.Task)
	BroadcastAgentRun(r domain.AgentRun)
	BroadcastKanbanItem(i domain.KanbanItem)
	BroadcastReviewThread(t domain.ReviewThread)
}
