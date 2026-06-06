package protocol

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// BenchmarkReadMessage measures the framing hot path at keystroke rate.
// A buffer of N pre-framed messages is built once; the benchmark reads them
// all per iteration to amortise allocation noise.
func BenchmarkReadMessage(b *testing.B) {
	const msgCount = 50

	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"textDocument/completion","params":{"textDocument":{"uri":"file:///workspace/main.go"},"position":{"line":42,"character":10}}}`)

	// Pre-build the framed buffer once.
	var raw bytes.Buffer
	for i := 0; i < msgCount; i++ {
		require.NoError(b, WriteMessage(&raw, payload))
	}
	framed := raw.Bytes()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r := bufio.NewReader(bytes.NewReader(framed))
		for j := 0; j < msgCount; j++ {
			_, err := ReadMessage(r)
			if err != nil {
				b.Fatalf("ReadMessage failed at msg %d: %v", j, err)
			}
		}
	}
}
