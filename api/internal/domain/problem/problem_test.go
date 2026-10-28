package problem_test

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func TestCodeOf_FindsTheFirstCodedErrorInTheChain(t *testing.T) {
	err := fmt.Errorf("x: %w", problem.New(problem.Conflict, "held"))

	assert.Equal(t, problem.Conflict, problem.CodeOf(err))
}

func TestCodeOf_SentinelIdentityIsKept(t *testing.T) {
	require.ErrorIs(t, fmt.Errorf("load: %w", problem.ErrNotFound), problem.ErrNotFound)
	assert.NotErrorIs(t, problem.New(problem.NotFound, "a"), problem.ErrNotFound)
}

func TestOf_MessageIsTheWholeChain(t *testing.T) {
	got := problem.Of(fmt.Errorf("import: %w", problem.New(problem.Conflict, "held")))

	assert.Equal(t, problem.Problem{Code: problem.Conflict, Message: "import: held"}, got)
}

func TestOf_NilIsAnInternalError(t *testing.T) {
	assert.Equal(t, problem.Problem{Code: problem.Internal, Message: "internal error"}, problem.Of(nil))
}

func TestDataOf_ReturnsTheCodedErrorsData(t *testing.T) {
	coded := &problem.Error{Code: problem.Conflict, Message: "held", Data: map[string]any{"who": "a"}}

	assert.Equal(t, map[string]any{"who": "a"}, problem.DataOf(fmt.Errorf("x: %w", coded)))
	assert.Nil(t, problem.DataOf(problem.ErrConflict))
	assert.Nil(t, problem.DataOf(nil))
}

func TestCodeOf_NilAndPlainErrorsAreInternal(t *testing.T) {
	assert.Equal(t, problem.Internal, problem.CodeOf(nil))
	assert.Equal(t, problem.Internal, problem.CodeOf(errors.New("boom")))
}

// A chain holding several codes answers the one the old status union checked
// first, whatever order the sentinels were wrapped in.
func TestCodeOf_PriorityMatchesStatusGo(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want problem.Code
	}{
		{"not found beats invalid", fmt.Errorf("%w: %w", problem.ErrInvalidArgument, fs.ErrNotExist), problem.NotFound},
		{"invalid beats conflict", fmt.Errorf("%w: %w", problem.ErrConflict, problem.ErrInvalidArgument), problem.InvalidArgument},
		{"unprocessable beats conflict", fmt.Errorf("%w: %w", problem.ErrConflict, problem.ErrUnprocessable), problem.Unprocessable},
		{"failed dependency beats bad gateway", fmt.Errorf("%w: %w", problem.ErrBadGateway, problem.ErrFailedDependency), problem.FailedDependency},
		{"unavailable beats conflict", fmt.Errorf("%w: %w", problem.ErrConflict, problem.ErrUnavailable), problem.Unavailable},
		{"a joined chain counts every branch", errors.Join(errors.New("a"), problem.ErrNotFound), problem.NotFound},
		{"a coded error ranks above an uncoded wrap", fmt.Errorf("%w: %w", errors.New("x"), problem.ErrTimeout), problem.Timeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, problem.CodeOf(tc.err))
		})
	}
}

// A specific code wraps its category, so a caller matching the category still
// matches, and the response carries the specific code.
func TestCodeOf_AWrappedCategoryStillMatches(t *testing.T) {
	busy := &problem.Error{Code: problem.ChatBusy, Message: "agent: chat is busy", Err: problem.ErrConflict}

	require.ErrorIs(t, busy, problem.ErrConflict)
	assert.Equal(t, problem.ChatBusy, problem.CodeOf(fmt.Errorf("submit: %w", busy)))
	assert.Equal(t, "agent: chat is busy: conflict", busy.Error())
}

// A specific code says more than its category, so it wins a same-rank tie
// wherever the category sentinel sits in the chain.
func TestCodeOf_ASpecificCodeBeatsASameRankCategory(t *testing.T) {
	busy := &problem.Error{Code: problem.ChatBusy, Message: "agent: chat is busy", Err: problem.ErrConflict}

	assert.Equal(t, problem.ChatBusy, problem.CodeOf(fmt.Errorf("%w: %w", problem.ErrConflict, busy)))
	assert.Equal(t, problem.ChatBusy, problem.CodeOf(fmt.Errorf("%w: %w", fs.ErrExist, busy)))
	assert.Equal(t, problem.NotFound, problem.CodeOf(fmt.Errorf("%w: %w", busy, problem.ErrNotFound)))
}
