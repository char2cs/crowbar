package git_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/git"
)

func findCall(
	t *testing.T,
	calls [][]string,
	sub string,
) []string {
	t.Helper()
	for _, c := range calls {
		if len(c) > 0 && c[0] == sub {
			return c
		}
	}
	t.Fatalf("no recorded %q call in %v", sub, calls)
	return nil
}

func assertSeparatorBeforeOperand(
	t *testing.T,
	args []string,
	operand string,
) {
	t.Helper()
	for i := 1; i < len(args); i++ {
		if args[i] == operand {
			require.GreaterOrEqual(t, i, 1, "operand %q at index 0 of %v", operand, args)
			assert.Equal(t, "--", args[i-1],
				"expected `--` immediately before operand %q in %v", operand, args)
			return
		}
	}
	t.Fatalf("operand %q not found in %v", operand, args)
}

func TestRegression_GitArgInjection_Merge_PassesSeparator(t *testing.T) {
	e, recorded := git.NewWithRecordingExec()
	require.NoError(t, e.Merge(context.Background(), "/repo", "feature/x"))

	call := findCall(t, *recorded, "merge")
	assertSeparatorBeforeOperand(t, call, "feature/x")
}

func TestRegression_GitArgInjection_Rebase_PassesSeparator(t *testing.T) {
	e, recorded := git.NewWithRecordingExec()
	require.NoError(t, e.Rebase(context.Background(), "/repo", "main"))

	call := findCall(t, *recorded, "rebase")
	assertSeparatorBeforeOperand(t, call, "main")
}

func TestRegression_GitArgInjection_Reset_NoSeparatorButFlagFromMode(t *testing.T) {
	e, recorded := git.NewWithRecordingExec()
	require.NoError(t, e.Reset(context.Background(), "/repo", "hard", "HEAD"))

	call := findCall(t, *recorded, "reset")
	require.GreaterOrEqual(t, len(call), 3)
	assert.Equal(t, "reset", call[0])
	assert.Equal(t, "--hard", call[1])
	assert.Equal(t, "HEAD", call[2])
}
