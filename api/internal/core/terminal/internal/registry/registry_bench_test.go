package registry

import (
	"os"
	"strconv"
	"testing"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
)

// BenchmarkListByChat measures the per-chat lookup cost on a registry
// populated across several chats (§13 perf-sensitive paths).
func BenchmarkListByChat(b *testing.B) {
	r := New()
	dir := b.TempDir()
	for i := 0; i < 256; i++ {
		id := "s" + strconv.Itoa(i)
		chatID := "chat" + strconv.Itoa(i%8)
		s, err := session.New(id, "/bin/sh", dir, "", os.Environ(), 80, 24, 0)
		if err != nil {
			b.Fatalf("session.New: %v", err)
		}
		b.Cleanup(s.Kill)
		r.Add(id, chatID, s)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.ListByChat("chat3")
	}
}
