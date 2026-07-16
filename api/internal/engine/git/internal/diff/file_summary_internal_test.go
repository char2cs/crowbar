package diff

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// The byte fixtures below are the verbatim NUL-delimited output of
// `git diff --name-status -M -z` and `git diff --numstat -M -z` captured from a
// real repo, so these tests pin the parser against git's actual wire format
// (single-char status + path; R<score> + old + new for renames; a trailing
// empty path field that flags a rename in numstat; "-" counts for binary).

func TestParseNameStatusZ(t *testing.T) {
	in := "A\x00added.txt\x00" +
		"M\x00keep.txt\x00" +
		"R100\x00old.txt\x00new.txt\x00" +
		"D\x00gone.txt\x00"

	got := parseNameStatusZ(in)

	require.Len(t, got, 4)
	assert.Equal(t, gitdomain.ReviewFileSummary{Path: "added.txt", Status: gitdomain.GitFileStatusAdded}, got[0])
	assert.Equal(t, gitdomain.ReviewFileSummary{Path: "keep.txt", Status: gitdomain.GitFileStatusModified}, got[1])
	assert.Equal(t, gitdomain.ReviewFileSummary{
		Path:    "new.txt",
		OldPath: "old.txt",
		Status:  gitdomain.GitFileStatusRenamed,
	}, got[2])
	assert.Equal(t, gitdomain.ReviewFileSummary{Path: "gone.txt", Status: gitdomain.GitFileStatusDeleted}, got[3])
}

func TestParseNumstatZ_TextRenameAndBinary(t *testing.T) {
	in := "3\t1\tkeep.txt\x00" +
		"0\t0\t\x00old.txt\x00new.txt\x00" +
		"-\t-\tlogo.png\x00"

	got := parseNumstatZ(in)

	require.Len(t, got, 3)
	assert.Equal(t, numCount{additions: 3, deletions: 1}, got["keep.txt"])
	// A rename's counts are keyed by the NEW path, never the old one.
	assert.Equal(t, numCount{additions: 0, deletions: 0}, got["new.txt"])
	_, hasOld := got["old.txt"]
	assert.False(t, hasOld, "rename counts must not be keyed by the source path")
	// Binary files carry the numstat "-" as -1 so a real 0/0 stays distinct.
	assert.Equal(t, numCount{additions: -1, deletions: -1}, got["logo.png"])
}

func TestStatusFromCode(t *testing.T) {
	assert.Equal(t, gitdomain.GitFileStatusAdded, statusFromCode("A"))
	assert.Equal(t, gitdomain.GitFileStatusModified, statusFromCode("M"))
	assert.Equal(t, gitdomain.GitFileStatusDeleted, statusFromCode("D"))
	assert.Equal(t, gitdomain.GitFileStatusRenamed, statusFromCode("R100"))
	assert.Equal(t, gitdomain.GitFileStatusRenamed, statusFromCode("C75"))
	assert.Equal(t, gitdomain.GitFileStatusModified, statusFromCode("T"))
	assert.Equal(t, gitdomain.GitFileStatusModified, statusFromCode(""))
}

func TestParseCount(t *testing.T) {
	assert.Equal(t, 7, parseCount("7"))
	assert.Equal(t, 0, parseCount("0"))
	assert.Equal(t, -1, parseCount("-"), "binary marker maps to -1")
	assert.Equal(t, 0, parseCount("garbage"), "unparseable count falls back to 0")
}
