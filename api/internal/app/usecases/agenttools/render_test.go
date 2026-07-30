package agenttools_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// countLinesContaining reports how many of lines contain substr, so a test can
// assert an anchor row appears exactly once even though a thread's own prose
// might happen to repeat the same text.
func countLinesContaining(lines []string, substr string) int {
	n := 0
	for _, l := range lines {
		if strings.Contains(l, substr) {
			n++
		}
	}
	return n
}

func TestRenderThreads_IsLineOrientedWithProseOnItsOwnLine(t *testing.T) {
	out := agenttools.RenderThreadsForTest([]domain.ReviewThread{{
		ID: "t1", FilePath: "src/auth.go", StartLine: 41, EndLine: 47,
		Side: domain.ReviewSideRight, Status: domain.ReviewThreadStatusOpen,
		Messages: []domain.ReviewMessage{
			{ID: "m1", Author: "mateo", Body: "Root cause: the mutex isn't released.\n- retry-count: 3"},
			{ID: "m2", Author: "claude", IsAgent: true, Body: "Agreed, bounding it."},
		},
	}})

	require.Contains(t, out, "t1")
	require.Contains(t, out, "src/auth.go:41-47")
	require.Contains(t, out, "right")
	require.Contains(t, out, "unresolved")

	// The body contains a colon, a leading dash and a newline — none of that may
	// corrupt the row structure: the anchor row must appear exactly once, and both
	// halves of the split body must survive intact.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Equal(t, 1, countLinesContaining(lines, "src/auth.go:41-47"),
		"the anchor row must appear exactly once")
	require.Contains(t, out, "Root cause: the mutex isn't released.")
	require.Contains(t, out, "- retry-count: 3")
	require.Contains(t, out, "claude (agent)")
}

func TestRenderThreads_EmptyIsExplicit(t *testing.T) {
	require.Contains(t, agenttools.RenderThreadsForTest(nil), "No review threads")
}
