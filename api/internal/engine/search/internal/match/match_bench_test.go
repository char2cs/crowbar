package match_test

import (
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/search/internal/match"
)

// BenchmarkFindMatches_ManyMatches benchmarks FindMatches on a 10 KB line with
// many occurrences of the target token.
func BenchmarkFindMatches_ManyMatches(b *testing.B) {
	token := "hello"
	// Build a ~10 KB line: "hello world " repeated until we exceed 10240 bytes.
	chunk := token + " world "
	var sb strings.Builder
	for sb.Len() < 10240 {
		sb.WriteString(chunk)
	}
	line := sb.String()

	re, err := match.BuildPattern(token, true, false, false)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = match.FindMatches(re, line)
	}
}

// BenchmarkBuildPattern benchmarks pattern compilation.
func BenchmarkBuildPattern(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = match.BuildPattern("hello.world", false, true, false)
	}
}
