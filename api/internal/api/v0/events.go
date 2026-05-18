package v0

import (
	"encoding/json"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/crowbar/api/internal/app/hub"
)

type EventsHandler struct {
	hub *hub.Hub
}

func NewEventsHandler(h *hub.Hub) *EventsHandler {
	return &EventsHandler{hub: h}
}

func (h *EventsHandler) Stream(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	client := h.hub.Register()
	defer h.hub.Unregister(client)

	c.Stream(func(w io.Writer) bool {
		select {
		case evt, ok := <-client:
			if !ok {
				return false
			}
			data, _ := json.Marshal(evt)
			c.SSEvent(evt.Type, string(data))
			return true
		case <-time.After(30 * time.Second):
			c.SSEvent("ping", "")
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}
