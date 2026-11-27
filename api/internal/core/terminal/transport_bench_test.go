package terminal

import (
	"bytes"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
)

type discardConn struct{ n int }

func (c *discardConn) WriteMessage(_ int, data []byte) error { c.n += len(data); return nil }
func (c *discardConn) ReadMessage() (int, []byte, error)     { select {} }
func (c *discardConn) SetWriteDeadline(time.Time) error      { return nil }
func (c *discardConn) Close() error                          { return nil }

func ptyChunk() []byte {
	line := []byte("\x1b[38;5;110m│\x1b[0m build \"step\" C:\\path ok ✓ 日本語 🚀 \x1b[1m done\x1b[0m\r\n")
	return bytes.Repeat(line, 4096/len(line)+1)[:4096]
}

func BenchmarkWritePump(b *testing.B) {
	e := newEngine(defaultConfig())
	StopMaintenanceForTest(e)
	chunk := ptyChunk()
	b.SetBytes(int64(len(chunk)))
	b.ReportAllocs()

	ch := make(chan session.OutputFrame, 64)
	done := make(chan struct{})
	conn := &discardConn{}
	go e.writePump(conn, ch, func() (int, bool) { return 0, false }, done)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch <- session.OutputFrame{SessionID: "bench", Data: chunk}
	}
	close(ch)
	<-done
}
