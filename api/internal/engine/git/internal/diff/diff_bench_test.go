package diff_test

import (
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

func BenchmarkHunkID(b *testing.B) {
	body := strings.Repeat("+added line\n-removed line\n context line\n", 20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		diff.HunkID("path/to/file.go", body)
	}
}
