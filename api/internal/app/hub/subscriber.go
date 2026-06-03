package hub

import "github.com/char2cs/crowbar/api/internal/domain"

// Subscriber receives domain broadcasts pushed by the Hub.
// Implemented by API-layer WebSocket handlers.
// Each PushX method must be non-blocking: if the handler's internal channel
// is full the call must return immediately rather than block the broadcaster.
type Subscriber interface {
	PushTask(t domain.Task)
	PushAgentRun(r domain.AgentRun)
	PushKanbanItem(i domain.KanbanItem)
	PushReviewThread(t domain.ReviewThread)
}
