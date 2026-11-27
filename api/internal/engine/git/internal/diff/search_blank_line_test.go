package diff_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
)

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
	assert.Equal(t, []gitdomain.SearchHit{
		{Path: "f.txt", Side: "new", LineNumber: 5, Preview: "MATCHME"},
	}, hits)
}
