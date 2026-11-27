package git_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func TestRegression_GitArgInjection_Reset_RejectsBadMode(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Now()
	git.ResetFn = func(context.Context, string, string, string) error {
		t.Fatal("engine Reset must not be called for an invalid mode")
		return nil
	}

	for _, mode := range []string{"evil", "--exec=touch /tmp/pwn", "", "Hard", "soft "} {
		err := uc.Reset(ctx, "w1", mode, "HEAD", now)
		require.Error(t, err, "mode %q must be rejected", mode)
		require.ErrorIs(t, err, problem.ErrInvalidArgument, "mode %q", mode)
		status := problem.CodeOf(err).Status()
		assert.Equal(t, http.StatusBadRequest, status, "mode %q must map to 400", mode)
	}
	assert.False(t, syncer.Synced, "a rejected reset must not resync")
}

func TestRegression_GitArgInjection_Reset_RejectsBadCommit(t *testing.T) {
	git, _, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Now()
	git.ResetFn = func(context.Context, string, string, string) error {
		t.Fatal("engine Reset must not be called for an invalid commit")
		return nil
	}

	err := uc.Reset(ctx, "w1", "hard", "--output=/tmp/pwn", now)
	require.ErrorIs(t, err, problem.ErrInvalidArgument)
}

func TestRegression_GitArgInjection_Reset_AllowsValidModes(t *testing.T) {
	git, _, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Now()
	git.AnyWriteOK()

	for _, mode := range []string{"soft", "mixed", "hard", "keep", "merge"} {
		var gotMode, gotCommit string
		git.ResetFn = func(_ context.Context, _, m, c string) error {
			gotMode, gotCommit = m, c
			return nil
		}
		require.NoError(t, uc.Reset(ctx, "w1", mode, "HEAD~1", now), "mode %q", mode)
		assert.Equal(t, mode, gotMode)
		assert.Equal(t, "HEAD~1", gotCommit)
	}
}

func TestRegression_GitArgInjection_Merge_RejectsDashBranch(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Now()
	git.MergeFn = func(context.Context, string, string) error {
		t.Fatal("engine Merge must not be called for an injection branch")
		return nil
	}

	for _, branch := range []string{"--exec=touch /tmp/pwn", "-X", "--output=/tmp/x"} {
		err := uc.Merge(ctx, "w1", branch, now)
		require.ErrorIs(t, err, problem.ErrInvalidArgument, "branch %q", branch)
		status := problem.CodeOf(err).Status()
		assert.Equal(t, http.StatusBadRequest, status)
	}
	assert.False(t, syncer.Synced)
}

func TestRegression_GitArgInjection_Merge_AllowsNormalBranch(t *testing.T) {
	git, _, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Now()
	git.AnyWriteOK()

	var gotBranch string
	git.MergeFn = func(_ context.Context, _, b string) error { gotBranch = b; return nil }
	for _, branch := range []string{"feature/x", "main", "release-1.2", "HEAD"} {
		require.NoError(t, uc.Merge(ctx, "w1", branch, now), "branch %q", branch)
		assert.Equal(t, branch, gotBranch)
	}
}

func TestRegression_GitArgInjection_Rebase_RejectsDashOnto(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Now()
	git.RebaseFn = func(context.Context, string, string) error {
		t.Fatal("engine Rebase must not be called for an injection onto")
		return nil
	}

	for _, onto := range []string{"--exec=touch /tmp/pwn", "--onto=evil", "-i"} {
		err := uc.Rebase(ctx, "w1", onto, now)
		require.ErrorIs(t, err, problem.ErrInvalidArgument, "onto %q", onto)
		status := problem.CodeOf(err).Status()
		assert.Equal(t, http.StatusBadRequest, status)
	}
	assert.False(t, syncer.Synced)
}

func TestRegression_GitArgInjection_Rebase_AllowsNormalOnto(t *testing.T) {
	git, _, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Now()
	git.AnyWriteOK()

	var gotOnto string
	git.RebaseFn = func(_ context.Context, _, o string) error { gotOnto = o; return nil }
	require.NoError(t, uc.Rebase(ctx, "w1", "main", now))
	assert.Equal(t, "main", gotOnto)
}

func TestRegression_GitArgInjection_BranchOps_RejectDashOperands(t *testing.T) {
	git, _, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Now()
	git.CreateBranchFn = func(context.Context, string, string, string, bool) error {
		t.Fatal("engine CreateBranch must not be called")
		return nil
	}
	git.RenameBranchFn = func(context.Context, string, string, string) error {
		t.Fatal("engine RenameBranch must not be called")
		return nil
	}
	git.DeleteBranchFn = func(context.Context, string, string) error {
		t.Fatal("engine DeleteBranch must not be called")
		return nil
	}
	git.SwitchBranchFn = func(context.Context, string, string) error {
		t.Fatal("engine SwitchBranch must not be called")
		return nil
	}

	cases := []struct {
		name string
		run  func() error
	}{
		{"create name", func() error { return uc.CreateBranch(ctx, "w1", "-D", "", true, now) }},
		{"create source", func() error { return uc.CreateBranch(ctx, "w1", "ok", "--evil", false, now) }},
		{"rename old", func() error { return uc.RenameBranch(ctx, "w1", "-x", "ok", now) }},
		{"rename new", func() error { return uc.RenameBranch(ctx, "w1", "ok", "--force", now) }},
		{"delete", func() error { return uc.DeleteBranch(ctx, "w1", "--all", now) }},
		{"switch", func() error { return uc.SwitchBranch(ctx, "w1", "-f", now) }},
	}
	for _, c := range cases {
		err := c.run()
		require.ErrorIs(t, err, problem.ErrInvalidArgument, c.name)
	}
}

func TestRegression_GitArgInjection_CommitDiff_RejectsBadSha(t *testing.T) {
	git, _, uc := newGitUsecase(t)
	ctx := context.Background()
	git.CommitDiffFn = func(context.Context, string, string) (gitdomain.MultiFileDiff, error) {
		t.Fatal("engine CommitDiff must not be called for an injection sha")
		return gitdomain.MultiFileDiff{}, nil
	}

	for _, sha := range []string{"--output=/tmp/pwn", "-O/tmp/x", "HEAD;rm -rf /"} {
		_, err := uc.CommitDiff(ctx, "w1", sha)
		require.ErrorIs(t, err, problem.ErrInvalidArgument, "sha %q", sha)
		status := problem.CodeOf(err).Status()
		assert.Equal(t, http.StatusBadRequest, status)
	}
}

func TestRegression_GitArgInjection_CommitDiff_AllowsValidSha(t *testing.T) {
	git, _, uc := newGitUsecase(t)
	ctx := context.Background()
	var gotSha string
	git.CommitDiffFn = func(_ context.Context, _, sha string) (gitdomain.MultiFileDiff, error) {
		gotSha = sha
		return gitdomain.MultiFileDiff{}, nil
	}
	_, err := uc.CommitDiff(ctx, "w1", "0a1b2c3d4e5f")
	require.NoError(t, err)
	assert.Equal(t, "0a1b2c3d4e5f", gotSha)
}

func TestRegression_GitArgInjection_Operand_RejectsMetacharacters(t *testing.T) {
	git, _, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Now()
	git.MergeFn = func(context.Context, string, string) error {
		t.Fatal("engine Merge must not be called for an invalid operand")
		return nil
	}

	bad := []string{
		"foo bar",
		"foo;rm -rf /",
		"foo`whoami`",
		"$(touch x)",
		"foo..bar",
		"foo@{0}",
		"refs/heads/x/",
		".hidden",
		"foo.lock",
	}
	for _, b := range bad {
		err := uc.Merge(ctx, "w1", b, now)
		require.ErrorIs(t, err, problem.ErrInvalidArgument, "operand %q must be rejected", b)
	}
}

func TestRegression_GitArgInjection_Operand_AllowsLegitimateRefs(t *testing.T) {
	git, _, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Now()
	git.AnyWriteOK()

	good := []string{
		"main",
		"feature/JIRA-123_fix",
		"refs/heads/release",
		"0a1b2c3d4e5f6071829384950617283940516273",
		"HEAD",
		"HEAD^",
		"HEAD~3",
		"v1.2.3",
	}
	for _, g := range good {
		require.NoError(t, uc.Merge(ctx, "w1", g, now), "ref %q must be accepted", g)
	}
}
