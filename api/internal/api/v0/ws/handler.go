package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WSHandler implements hub.Subscriber and exposes WS upgrade handlers for each
// subscribable domain entity type. One WSHandler is shared across all
// connections; individual clients are managed inside each Broadcaster.
type WSHandler struct {
	tasks       *Broadcaster[domain.Task]
	agentRuns   *Broadcaster[domain.AgentRun]
	kanbanItems *Broadcaster[domain.KanbanItem]
	threads     *Broadcaster[domain.ReviewThread]

	taskRepo    repositories.Task
	agentRepo   repositories.AgentRun
	kanbanRepo  repositories.KanbanItem
	threadRepo  repositories.ReviewThread
}

// NewWSHandler constructs a WSHandler with empty broadcasters.
// The repos are used for the initial-sync frame sent on each new connection.
func NewWSHandler(
	taskRepo repositories.Task,
	agentRepo repositories.AgentRun,
	kanbanRepo repositories.KanbanItem,
	threadRepo repositories.ReviewThread,
) *WSHandler {
	return &WSHandler{
		tasks:       NewBroadcaster[domain.Task](),
		agentRuns:   NewBroadcaster[domain.AgentRun](),
		kanbanItems: NewBroadcaster[domain.KanbanItem](),
		threads:     NewBroadcaster[domain.ReviewThread](),

		taskRepo:   taskRepo,
		agentRepo:  agentRepo,
		kanbanRepo: kanbanRepo,
		threadRepo: threadRepo,
	}
}

// ---------------------------------------------------------------------------
// hub.Subscriber implementation
// ---------------------------------------------------------------------------

// PushTask fans out t to all Task subscribers.
func (h *WSHandler) PushTask(t domain.Task) {
	h.tasks.Broadcast(t, "task")
}

// PushAgentRun fans out r to all AgentRun subscribers.
func (h *WSHandler) PushAgentRun(r domain.AgentRun) {
	h.agentRuns.Broadcast(r, "agent_run")
}

// PushKanbanItem fans out i to all KanbanItem subscribers.
func (h *WSHandler) PushKanbanItem(i domain.KanbanItem) {
	h.kanbanItems.Broadcast(i, "kanban_item")
}

// PushReviewThread fans out t to all ReviewThread subscribers.
func (h *WSHandler) PushReviewThread(t domain.ReviewThread) {
	h.threads.Broadcast(t, "review_thread")
}

// ---------------------------------------------------------------------------
// WS endpoint handlers
// ---------------------------------------------------------------------------

// HandleTask handles WS /api/v0/tasks/:id.
// On connect it delivers the current task state as an initial sync frame, then
// streams all future updates whose ID matches the request param.
func (h *WSHandler) HandleTask(c *gin.Context) {
	taskID := c.Param("id")

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws: HandleTask: upgrade: %v", err)
		return
	}

	pred := func(t domain.Task) bool { return t.ID == taskID }
	ch, unsub := h.tasks.Subscribe(pred)
	defer unsub()

	// Initial sync: send current state before entering the streaming loop.
	ctx := context.Background()
	if task, err := h.taskRepo.Get(ctx, taskID); err == nil {
		h.sendInitialFrame(conn, "task", task)
	}

	done := make(chan struct{})
	go func() {
		readPump(conn)
		close(done)
	}()

	writePump(conn, ch, done)
}

// HandleKanbanItems handles WS /api/v0/tasks/:id/kanban-items.
// On connect it delivers all current items for the task as individual sync
// frames, then streams future updates.
func (h *WSHandler) HandleKanbanItems(c *gin.Context) {
	taskID := c.Param("id")

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws: HandleKanbanItems: upgrade: %v", err)
		return
	}

	pred := func(i domain.KanbanItem) bool { return i.TaskID == taskID }
	ch, unsub := h.kanbanItems.Subscribe(pred)
	defer unsub()

	// Initial sync: send every existing kanban item for this task.
	ctx := context.Background()
	if items, err := h.kanbanRepo.ListByTask(ctx, taskID); err == nil {
		for _, item := range items {
			h.sendInitialFrame(conn, "kanban_item", item)
		}
	}

	done := make(chan struct{})
	go func() {
		readPump(conn)
		close(done)
	}()

	writePump(conn, ch, done)
}

// HandleThreads handles WS /api/v0/tasks/:id/threads.
// On connect it delivers all current threads for the task as individual sync
// frames, then streams future updates.
func (h *WSHandler) HandleThreads(c *gin.Context) {
	taskID := c.Param("id")

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws: HandleThreads: upgrade: %v", err)
		return
	}

	pred := func(t domain.ReviewThread) bool { return t.TaskID == taskID }
	ch, unsub := h.threads.Subscribe(pred)
	defer unsub()

	// Initial sync: send every existing review thread for this task.
	ctx := context.Background()
	if threads, err := h.threadRepo.ListByTask(ctx, taskID, "", ""); err == nil {
		for _, t := range threads {
			h.sendInitialFrame(conn, "review_thread", t)
		}
	}

	done := make(chan struct{})
	go func() {
		readPump(conn)
		close(done)
	}()

	writePump(conn, ch, done)
}

// sendInitialFrame marshals entity into a WS envelope and writes it directly
// to conn as the initial sync frame sent before the streaming loop starts.
func (h *WSHandler) sendInitialFrame(conn *websocket.Conn, eventType string, entity any) {
	data, err := json.Marshal(wsEnvelope{Type: eventType, Data: entity})
	if err != nil {
		log.Printf("ws: sendInitialFrame: marshal %s: %v", eventType, err)
		return
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("ws: sendInitialFrame: write %s: %v", eventType, err)
	}
}

// ---------------------------------------------------------------------------
// Dispatch helper
// ---------------------------------------------------------------------------

// Dispatch returns a gin.HandlerFunc that routes to wsHandler when the request
// carries an "Upgrade: websocket" header, and to restHandler otherwise. Export
// this from the ws package so router.go can use it without importing gorilla
// directly.
func Dispatch(restHandler gin.HandlerFunc, wsHandler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Upgrade") == "websocket" {
			wsHandler(c)
		} else {
			restHandler(c)
		}
	}
}
