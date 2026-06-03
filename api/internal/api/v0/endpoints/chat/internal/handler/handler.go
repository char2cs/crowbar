// Package handler implements the ChatHandler WebSocket logic.
package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agent"
)

// pingInterval is the time between WebSocket ping frames. Declared as a
// variable so tests can shorten it without modifying the production value.
var pingInterval = 30 * time.Second

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// incomingMsg is the JSON shape expected from the browser.
type incomingMsg struct {
	Type    agent.ChatFrameType `json:"type"`
	Content string              `json:"content"`
}

// hubber is the subset of hub.ChatHub used by the handler.
// Defined locally to break the import cycle between chat and chat/internal/handler.
type hubber interface {
	Subscribe(taskID string) (frames <-chan agent.ChatFrame, unsubscribe func())
	Forward(taskID string, content string)
}

// conversationWriter is the subset of repositories.Conversation used by the handler.
// Defined locally to break the import cycle between chat and repositories.
type conversationWriter interface {
	Create(ctx context.Context, taskID string, role domain.ConversationMessageRole, msgType domain.ConversationMessageType, content string) (domain.ConversationMessage, error)
}

// ChatHandler is satisfied by the Handler struct.
type ChatHandler interface {
	Handle(ctx context.Context, c *gin.Context, taskID string)
}

// Handler implements ChatHandler.
type Handler struct {
	hub          hubber
	conversation conversationWriter
}

// New constructs a Handler.
func New(h hubber, conv conversationWriter) ChatHandler {
	return &Handler{hub: h, conversation: conv}
}

// Handle upgrades the HTTP connection to WebSocket, then runs readPump and
// writePump concurrently. It returns when either pump exits.
func (h *Handler) Handle(ctx context.Context, c *gin.Context, taskID string) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("chat handler: upgrade: %v", err)
		return
	}

	frames, unsubscribe := h.hub.Subscribe(taskID)

	var wg sync.WaitGroup
	wg.Add(2)

	// pumpErr is set by whichever pump exits first; the other pump stops when
	// conn is closed.
	go func() {
		defer wg.Done()
		h.readPump(ctx, conn, taskID)
		// Close the connection so writePump's ReadMessage / WriteMessage returns.
		conn.Close()
	}()

	go func() {
		defer wg.Done()
		h.writePump(ctx, conn, taskID, frames)
		conn.Close()
	}()

	wg.Wait()
	unsubscribe()
}

// readPump reads user messages from the WebSocket connection, persists them,
// and forwards them to the hub.
func (h *Handler) readPump(ctx context.Context, conn *websocket.Conn, taskID string) {
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			// Normal disconnect or connection reset.
			return
		}

		var in incomingMsg
		if err := json.Unmarshal(msg, &in); err != nil {
			log.Printf("chat handler: readPump: malformed message: %v", err)
			continue
		}

		if in.Type != agent.ChatFrameTypeUserMessage {
			continue
		}

		content := in.Content

		_, err = h.conversation.Create(
			ctx,
			taskID,
			domain.ConversationMessageRoleUser,
			domain.ConversationMessageTypeText,
			content,
		)
		if err != nil {
			log.Printf("chat handler: readPump: persist user message: %v", err)
		}

		h.hub.Forward(taskID, content)
	}
}

// writePump reads ChatFrames from the subscription channel, marshals them to
// JSON, and writes them to the WebSocket. It also sends periodic pings and
// assembles the full agent turn text for persistence.
func (h *Handler) writePump(ctx context.Context, conn *websocket.Conn, taskID string, frames <-chan agent.ChatFrame) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	var turnBuf strings.Builder

	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				return
			}

			// Accumulate agent chunks into a turn buffer.
			if frame.Type == agent.ChatFrameTypeAgentChunk {
				turnBuf.WriteString(frame.Delta)
			}

			b, err := json.Marshal(frame)
			if err != nil {
				log.Printf("chat handler: writePump: marshal: %v", err)
				continue
			}

			if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
				return
			}

			// On turn end, persist the assembled text as an agent message.
			if frame.Type == agent.ChatFrameTypeAgentTurnEnd {
				fullText := turnBuf.String()
				turnBuf.Reset()
				if fullText != "" {
					_, err = h.conversation.Create(
						ctx,
						taskID,
						domain.ConversationMessageRoleAgent,
						domain.ConversationMessageTypeText,
						fullText,
					)
					if err != nil {
						log.Printf("chat handler: writePump: persist agent turn: %v", err)
					}
				}
			}

		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-ctx.Done():
			return
		}
	}
}
