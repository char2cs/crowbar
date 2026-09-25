package terminal

import (
	"bytes"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
)

// discardConn is a WSConn that accepts every write instantly, so a benchmark measures the
// engine's own per-frame work (framing, encoding, copies) and nothing else.
type discardConn struct{ n int }

func (c *discardConn) WriteMessage(_ int, data []byte) error { c.n += len(data); return nil }
func (c *discardConn) ReadMessage() (int, []byte, error)     { select {} }
func (c *discardConn) SetWriteDeadline(time.Time) error      { return nil }
func (c *discardConn) Close() error                          { return nil }

// ptyChunk is representative TUI output: SGR runs, box drawing and a few CJK/emoji runes,
// quotes and backslashes (which JSON has to escape) — 4 KiB, a typical PTY read.
func ptyChunk() []byte {
	line := []byte("\x1b[38;5;110m│\x1b[0m build \"step\" C:\\path ok ✓ 日本語 🚀 \x1b[1m done\x1b[0m\r\n")
	return bytes.Repeat(line, 4096/len(line)+1)[:4096]
}

// BenchmarkWritePump measures the output path from a session's fan-out channel to the
// socket: bytes/op is PTY bytes delivered, allocs/op the per-frame garbage.
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
