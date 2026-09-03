package diff_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

// TestSearchDiff_BlankContextLine_SuppressBlankEmptyConfig pins body()'s
// len(line)==0 branch for real. By default git renders a blank source line as
// a context line with a single leading space (" "), which is not zero-length
// and never reaches this branch — only with the real, user-controllable
// `diff.suppressBlankEmpty` config set does git emit a truly empty line, and
// the line-number counters must still advance for both sides across it.
func TestSearchDiff_BlankContextLine_SuppressBlankEmptyConfig(t *testing.T) {
	dir := initRepo(t)
	mustGit(t, dir, "config", "diff.suppressBlankEmpty", "true")
	writeFile(t, dir, "f.txt", "a\nb\n\nc\nd\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-m", "base")
	base := headSHA(t, dir)
	writeFile(t, dir, "f.txt", "a\nb\n\nc\nMATCHME\n")

	hits, truncated, err := diff.SearchDiff(context.Background(), dir, base, "MATCHME", gitdomain.SearchOpts{})

	require.NoError(t, err)
	assert.False(t, truncated)
	// Line 5, not 4: a blank line at 3 not advancing the counters would shift
	// every line number after it down by one.
	assert.Equal(t, []gitdomain.SearchHit{
		{Path: "f.txt", Side: "new", LineNumber: 5, Preview: "MATCHME"},
	}, hits)
}
