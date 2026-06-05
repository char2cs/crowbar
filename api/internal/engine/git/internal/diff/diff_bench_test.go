package diff_test

import (
	"context"
	"fmt"
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

func BenchmarkParseFiles(b *testing.B) {
	// Build a realistic multi-file unified diff fixture (~500 lines).
	var sb strings.Builder
	for fileIdx := 0; fileIdx < 10; fileIdx++ {
		name := fmt.Sprintf("pkg/file%d.go", fileIdx)
		fmt.Fprintf(&sb, "diff --git a/%s b/%s\n", name, name)
		fmt.Fprintf(&sb, "index abc123..def456 100644\n")
		fmt.Fprintf(&sb, "--- a/%s\n+++ b/%s\n", name, name)
		for hunk := 0; hunk < 5; hunk++ {
			start := hunk*20 + 1
			fmt.Fprintf(&sb, "@@ -%d,10 +%d,12 @@\n", start, start)
			for j := 0; j < 8; j++ {
				fmt.Fprintf(&sb, " context line %d\n", j)
			}
			fmt.Fprintf(&sb, "-removed line\n")
			fmt.Fprintf(&sb, "+added line one\n")
			fmt.Fprintf(&sb, "+added line two\n")
		}
	}
	fixture := sb.String()
	ctx := context.Background()
	dir := b.TempDir()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		diff.ParseFilesFromText(ctx, dir, fixture)
	}
}
