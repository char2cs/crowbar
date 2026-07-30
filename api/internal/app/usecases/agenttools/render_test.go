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

// A human reply's Author is "" (branchreview.Reply hardcodes it — see its doc
// comment), which used to render as a blank name: "    : Why does this divide
// by...". In a thread with a human, an agent reply, and a second human, two
// blank-prefixed lines were indistinguishable. Defaulting a blank, non-agent
// author to "user" in the renderer fixes this without touching branchreview,
// the thread store, or the frontend.
func TestRenderThreads_DefaultsBlankHumanAuthorToUser(t *testing.T) {
	out := agenttools.RenderThreadsForTest([]domain.ReviewThread{{
		ID: "t1", FilePath: "src/div.go", StartLine: 10, EndLine: 12,
		Side: domain.ReviewSideRight, Status: domain.ReviewThreadStatusOpen,
		Messages: []domain.ReviewMessage{
			{ID: "m1", Author: "", IsAgent: false, Body: "Why does this divide by zero?"},
			{ID: "m2", Author: "claude", IsAgent: true, Body: "It cannot: n is checked above."},
			{ID: "m3", Author: "", IsAgent: false, Body: "Ah, missed that."},
		},
	}})

	require.NotContains(t, out, "    : ", "a blank author must never render as an unnamed speaker")
	require.Contains(t, out, "user: Why does this divide by zero?")
	require.Contains(t, out, "claude (agent): It cannot: n is checked above.")
	require.Contains(t, out, "user: Ah, missed that.")
}

func TestRenderWorkspaces_MarksTheCallersOwnWorkspace(t *testing.T) {
	caller := domain.Workspace{ID: "ws-a"}
	visible := []domain.Workspace{{ID: "ws-a"}, {ID: "ws-a1"}}
	out := agenttools.RenderWorkspacesForTest(caller, visible, nil)

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Contains(t, lines, "* ws-a")
	require.Contains(t, lines, "ws-a1")
	require.NotContains(t, lines, "ws-a", "the caller's own row must carry the * marker, not a bare id")
	require.NotContains(t, lines, "* ws-a1")
}

func TestRenderWorkspaces_FoldsEachWorkspacesChatsIn(t *testing.T) {
	caller := domain.Workspace{ID: "ws-a"}
	visible := []domain.Workspace{{ID: "ws-a"}, {ID: "ws-a1"}}
	chats := map[string][]domain.AgentChat{
		"ws-a":  {{ID: "c1", Title: "Fix Auth Bug"}},
		"ws-a1": {{ID: "c2", Title: ""}},
	}
	out := agenttools.RenderWorkspacesForTest(caller, visible, chats)

	require.Contains(t, out, "c1")
	require.Contains(t, out, "Fix Auth Bug")
	require.Contains(t, out, "c2")
	require.Contains(t, out, "(untitled)", "an untitled chat must still render as a row, not vanish")
}

func TestRenderWorkspaces_EmptyIsExplicit(t *testing.T) {
	require.Contains(t, agenttools.RenderWorkspacesForTest(domain.Workspace{}, nil, nil), "No visible workspaces")
}
