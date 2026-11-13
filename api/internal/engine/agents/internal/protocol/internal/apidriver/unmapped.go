package apidriver

import (
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/jsonrpc"
)

// codeMethodNotFound is JSON-RPC's answer for a method the receiver does not serve.
const codeMethodNotFound = -32601

// refuseUnmapped answers a server request the descriptor maps nothing for, so the
// remote side fails fast. A notification (no id) expects no answer and gets none.
func (drv *Driver) refuseUnmapped(conn wire, frame jsonrpc.Frame) {
	if frame.ID == nil {
		return
	}
	if err := conn.ReplyError(frame.ID, codeMethodNotFound, "method not found: "+frame.Method); err != nil {
		slog.DebugContext(drv.ctx, "apidriver: refuse unmapped request", "provider", drv.d.ID, "method", frame.Method, "err", err)
	}
}
