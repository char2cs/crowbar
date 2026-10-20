package mapping_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
)

// A pattern read runs for every tool event, so it must not compile its regexp each time.
func BenchmarkString_PatternBranch(b *testing.B) {
	branch, err := mapping.Pattern("tool_name", mcpName)
	if err != nil {
		b.Fatal(err)
	}
	paths := mapping.Expr{branch}
	for name, tool := range map[string]string{"match": "mcp__echo__shout", "miss": "Bash"} {
		doc := map[string]any{"tool_name": tool}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				_ = mapping.String(doc, paths)
			}
		})
	}
}
