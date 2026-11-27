package apidriver

import (
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/jsonrpc"
)

const codeMethodNotFound = -32601

func (drv *Driver) refuseUnmapped(conn wire, frame jsonrpc.Frame) {
	if frame.ID == nil {
		return
	}
	if err := conn.ReplyError(frame.ID, codeMethodNotFound, "method not found: "+frame.Method); err != nil {
		slog.DebugContext(drv.ctx, "apidriver: refuse unmapped request", "provider", drv.d.ID, "method", frame.Method, "err", err)
	}
}
