package stream

import (
	"log/slog"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type Presence struct{ Viewers int }

func (h *Hub) presenceLocked(scope stream.Scope) {
	if h.closed || scope.Kind != stream.KindChat {
		return
	}
	f, ok := h.presenceFrame(scope.Key, len(h.scopes[scope.String()]))
	if !ok {
		return
	}
	h.seq++
	f.V = h.seq
	h.fanOutLocked(f)
}

func (h *Hub) presenceFrame(id string, viewers int) (stream.Frame, bool) {
	f := stream.Frame{T: stream.FrameEntity, Scope: stream.Projects.String(), Kind: string(stream.KindPresence), ID: id}
	if viewers == 0 {
		f.Gone = true
		return f, true
	}
	raw, err := h.encode(stream.KindPresence, Presence{Viewers: viewers})
	if err != nil {
		slog.Error("stream: encode a presence", "chat", id, "err", err)
		return f, false
	}
	f.Data = raw
	return f, true
}

func (h *Hub) syncedLocked(c *connection, name string, unread ...string) bool {
	if name == stream.Projects.String() {
		for _, f := range h.presenceRowsLocked() {
			if !c.push(f, false) {
				return false
			}
		}
	}
	return c.push(stream.Synced(name, unread...), false)
}

func (h *Hub) presenceRowsLocked() []stream.Frame {
	var rows []stream.Frame
	for name, holders := range h.scopes {
		id, ok := strings.CutPrefix(name, string(stream.KindChat)+":")
		if !ok {
			continue
		}
		if f, ok := h.presenceFrame(id, len(holders)); ok {
			f.V = h.seq
			rows = append(rows, f)
		}
	}
	return rows
}
