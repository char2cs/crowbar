package handler

import (
	"encoding/json"
	"io"
	"os"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// session holds the per-connection state for a PTY WebSocket session.
type session struct {
	conn *websocket.Conn
	ptmx *os.File
}

// pumpPTYToWS reads PTY output and forwards it to the WebSocket as binary
// frames. On PTY EOF it sends a WebSocket close frame before returning.
func (s *session) pumpPTYToWS() {
	buf := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			if writeErr := s.conn.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
				return
			}
		}
		if err != nil {
			_ = s.conn.WriteMessage(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			)
			return
		}
	}
}

// pumpWSToPTY reads WebSocket frames from the client and forwards them to the
// PTY. Frames that unmarshal as a resize command trigger a PTY window resize
// instead of being written to stdin.
func (s *session) pumpWSToPTY() {
	for {
		msgType, data, err := s.conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType != websocket.TextMessage && msgType != websocket.BinaryMessage {
			continue
		}
		var frame resizeFrame
		if json.Unmarshal(data, &frame) == nil && frame.Type == "resize" {
			_ = pty.Setsize(s.ptmx, &pty.Winsize{
				Cols: frame.Cols,
				Rows: frame.Rows,
			})
			continue
		}
		if _, writeErr := io.Writer(s.ptmx).Write(data); writeErr != nil {
			return
		}
	}
}
