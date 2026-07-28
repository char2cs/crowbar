package branchreview

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// stubRevParser answers `<commit>^` with parent, or an error when the commit is
// a root commit.
type stubRevParser struct {
	parent string
	err    error
	sawRev string
}

func (s *stubRevParser) RevParse(
	_ context.Context,
	_ string,
	rev string,
) (string, error) {
	s.sawRev = rev
	return s.parent, s.err
}

func TestCommitRange_NamesBothEnds(t *testing.T) {
	// The load-bearing property. Every read underneath passes this ref to
	// `git diff <ref> --`, which against a SINGLE ref means "ref versus the
	// working tree" — for a commit that answers "everything since then", the
	// opposite of what the reader asked for.
	rev := &stubRevParser{parent: "aaaa1111"}
	got := commitRange(context.Background(), rev, "/repo", "bbbb2222")

	assert.Equal(t, "aaaa1111..bbbb2222", got)
	assert.Equal(t, "bbbb2222^", rev.sawRev, "the parent is asked for by ^, not assumed")
}

func TestCommitRange_RootCommitDiffsAgainstTheEmptyTree(t *testing.T) {
	// A root commit has no parent, so `<commit>^` does not resolve. Falling back
	// to the empty tree is what `git show` does; without it the very first commit
	// of a repository could not be viewed at all.
	rev := &stubRevParser{err: errors.New("unknown revision")}
	got := commitRange(context.Background(), rev, "/repo", "cccc3333")

	assert.Equal(t, emptyTreeSHA+"..cccc3333", got)
}

func TestCommitRange_EmptyParentIsTreatedAsRoot(t *testing.T) {
	// RevParse answering "" without an error is not a parent; taking it at face
	// value would build the ref "..cccc3333", which git reads as HEAD..cccc3333.
	rev := &stubRevParser{parent: ""}
	got := commitRange(context.Background(), rev, "/repo", "cccc3333")

	assert.Equal(t, emptyTreeSHA+"..cccc3333", got)
}

func TestResolveScopeRef_RejectsAnythingButAHexObjectName(t *testing.T) {
	// The resolved ref becomes an ARGUMENT to `git diff`, and git reads a leading
	// `-` as a flag. These must never reach a subprocess.
	u := &branchReviewUsecase{}
	for _, commit := range []string{
		"--output=/tmp/pwned",
		"-U9999",
		"HEAD",          // a valid ref, but not how this surface addresses a commit
		"main..other",   // range syntax must be built here, never accepted
		"abc",           // too short to be an object name
		"zzzz",          // not hex
		"aaaa bbbb",     // whitespace
		"aaaa;rm -rf /", // punctuation
		"$(whoami)",     //
	} {
		_, err := u.resolveScopeRef(context.Background(), domain.Workspace{}, commit)
		require.Errorf(t, err, "commit %q must be refused", commit)
		assert.ErrorIsf(t, err, apperr.ErrInvalidArgument, "commit %q must be an invalid argument", commit)
	}
}

func TestIsCommitScoped(t *testing.T) {
	// Drives whether the outline may be cached while the working tree is dirty:
	// a commit-scoped read is a diff of two immutable trees, so it may.
	assert.False(t, isCommitScoped(""))
	assert.True(t, isCommitScoped("aaaa1111"))
}
