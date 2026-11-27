package branchreview

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

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
	rev := &stubRevParser{parent: "aaaa1111"}
	got := commitRange(context.Background(), rev, "/repo", "bbbb2222")

	assert.Equal(t, "aaaa1111..bbbb2222", got)
	assert.Equal(t, "bbbb2222^", rev.sawRev, "the parent is asked for by ^, not assumed")
}

func TestCommitRange_RootCommitDiffsAgainstTheEmptyTree(t *testing.T) {
	rev := &stubRevParser{err: errors.New("unknown revision")}
	got := commitRange(context.Background(), rev, "/repo", "cccc3333")

	assert.Equal(t, emptyTreeSHA+"..cccc3333", got)
}

func TestCommitRange_EmptyParentIsTreatedAsRoot(t *testing.T) {
	rev := &stubRevParser{parent: ""}
	got := commitRange(context.Background(), rev, "/repo", "cccc3333")

	assert.Equal(t, emptyTreeSHA+"..cccc3333", got)
}

func TestResolveScopeRef_RejectsAnythingButAHexObjectName(t *testing.T) {
	u := &branchReviewUsecase{}
	for _, commit := range []string{
		"--output=/tmp/pwned",
		"-U9999",
		"HEAD",
		"main..other",
		"abc",
		"zzzz",
		"aaaa bbbb",
		"aaaa;rm -rf /",
		"$(whoami)",
	} {
		_, err := u.resolveScopeRef(context.Background(), domain.Workspace{}, commit)
		require.Errorf(t, err, "commit %q must be refused", commit)
		assert.ErrorIsf(t, err, problem.ErrInvalidArgument, "commit %q must be an invalid argument", commit)
	}
}

func TestIsCommitScoped(t *testing.T) {
	assert.False(t, isCommitScoped(""))
	assert.True(t, isCommitScoped("aaaa1111"))
}
