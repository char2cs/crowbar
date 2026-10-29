package provision_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/provision"
	"github.com/char2cs/crowbar/api/internal/engine/git"
)

// fakeGit has origin's branch and answers the local tip read with localErr.
type fakeGit struct {
	localErr error
	added    bool
}

func (f *fakeGit) RemoteTrackingBranchExists(context.Context, string, string) (bool, error) {
	return true, nil
}

func (f *fakeGit) FetchRef(context.Context, string, string) error { return nil }

func (f *fakeGit) WorktreeAdd(context.Context, string, string, string) error { return nil }

func (f *fakeGit) WorktreeAddAtRef(context.Context, string, string, string, string) (string, error) {
	f.added = true
	return "origin-tip", nil
}

func (f *fakeGit) SetUpstream(context.Context, string, string) error { return nil }

func (f *fakeGit) RevParse(context.Context, string, string) (string, error) {
	return "", f.localErr
}

func (f *fakeGit) MergeBase(context.Context, string, string, string) (string, error) {
	return "", nil
}

func TestExistingBranch_ARevParseFailureIsReturned(t *testing.T) {
	cause := errors.New("fatal: not a git repository")
	g := &fakeGit{localErr: cause}

	_, err := provision.ExistingBranch(context.Background(), g, "/repo", "feature", "/wt")

	require.ErrorIs(t, err, cause)
	assert.False(t, g.added, "no checkout runs when the local tip cannot be read")
}

func TestExistingBranch_AnAbsentLocalBranchIsNotAFailure(t *testing.T) {
	g := &fakeGit{localErr: git.ErrBranchNotFound}

	sha, err := provision.ExistingBranch(context.Background(), g, "/repo", "feature", "/wt")

	require.NoError(t, err)
	assert.Equal(t, "origin-tip", sha)
}
