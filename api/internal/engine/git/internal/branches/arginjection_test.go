package branches_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/branches"
)

// assertSeparatorBefore asserts that args contains a "--" token immediately
// before operand, so the operand can never be parsed by git as an option.
func assertSeparatorBefore(
	t *testing.T,
	args []string,
	operand string,
) {
	t.Helper()
	for i := 1; i < len(args); i++ {
		if args[i] == operand {
			assert.Equal(t, "--", args[i-1],
				"expected `--` immediately before %q in %v", operand, args)
			return
		}
	}
	t.Fatalf("operand %q not found in %v", operand, args)
}

func lastCall(t *testing.T, recorded *[][]string) []string {
	t.Helper()
	require.NotEmpty(t, *recorded, "no git invocation recorded")
	return (*recorded)[len(*recorded)-1]
}

// TestRegression_GitArgInjection_BranchCreate_PassesSeparator asserts that the
// non-switching create path (`git branch`) puts `--` before the new name.
func TestRegression_GitArgInjection_BranchCreate_PassesSeparator(t *testing.T) {
	recorded := branches.SetRecordingRunner(t.Cleanup)
	require.NoError(t, branches.Create(context.Background(), "/repo", "feature/x", "", false))

	call := lastCall(t, recorded)
	assert.Equal(t, "branch", call[0])
	assertSeparatorBefore(t, call, "feature/x")
}

// TestRegression_GitArgInjection_BranchRename_PassesSeparator asserts that
// `git branch -m` puts `--` before the operands.
func TestRegression_GitArgInjection_BranchRename_PassesSeparator(t *testing.T) {
	recorded := branches.SetRecordingRunner(t.Cleanup)
	require.NoError(t, branches.Rename(context.Background(), "/repo", "old", "new"))

	call := lastCall(t, recorded)
	assert.Equal(t, []string{"branch", "-m", "--", "old", "new"}, call)
}

// TestRegression_GitArgInjection_BranchDelete_PassesSeparator asserts that
// `git branch -d` / `-D` put `--` before the name.
func TestRegression_GitArgInjection_BranchDelete_PassesSeparator(t *testing.T) {
	recorded := branches.SetRecordingRunner(t.Cleanup)

	require.NoError(t, branches.Delete(context.Background(), "/repo", "feature/x"))
	assert.Equal(t, []string{"branch", "-d", "--", "feature/x"}, lastCall(t, recorded))

	require.NoError(t, branches.ForceDelete(context.Background(), "/repo", "feature/x"))
	assert.Equal(t, []string{"branch", "-D", "--", "feature/x"}, lastCall(t, recorded))
}

// TestRegression_GitArgInjection_BranchOps_HappyPath proves the `--` additions
// do not break real branch operations end-to-end against the git binary.
func TestRegression_GitArgInjection_BranchOps_HappyPath(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	makeCommit(t, dir, "f.txt", "x\n", "init")

	require.NoError(t, branches.Create(ctx, dir, "feature/x", "", false))
	require.NoError(t, branches.Rename(ctx, dir, "feature/x", "feature/y"))
	require.NoError(t, branches.Delete(ctx, dir, "feature/y"))

	bs, err := branches.List(ctx, dir)
	require.NoError(t, err)
	assert.Nil(t, findBranch(bs, "feature/y"), "renamed-then-deleted branch must be gone")
}
