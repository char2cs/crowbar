package transcript_test

import (
	"fmt"
	"testing"
)

// BenchmarkProjector_Apply is one commit of one message on a read chat of
// 100 and of 10,000 rows: its cost is the commit's, flat as the chat grows.
func BenchmarkProjector_Apply(b *testing.B) {
	for _, n := range []int{100, 10_000} {
		b.Run(fmt.Sprintf("rows=%d", n), func(b *testing.B) {
			p, next := cachedHistory(b, n, b.N)
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				p.Apply(chatID, next[i:i+1])
			}
		})
	}
}
