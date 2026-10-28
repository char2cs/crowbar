package problem_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"testing"

	asynxmodels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func TestCodeOf_ForeignErrors(t *testing.T) {
	cases := []struct {
		err  error
		want problem.Code
	}{
		{fs.ErrNotExist, problem.NotFound},
		{fs.ErrExist, problem.Conflict},
		{fs.ErrInvalid, problem.InvalidArgument},
		{asynxmodels.ErrNotFound, problem.NotFound},
		{asynxmodels.ErrValidation, problem.Unprocessable},
		{asynxmodels.ErrPipelineFailed, problem.Conflict},
		{asynxmodels.ErrQueueFull, problem.Unavailable},
		{asynxmodels.ErrShuttingDown, problem.Unavailable},
		{context.DeadlineExceeded, problem.Timeout},
	}
	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			assert.Equal(t, tc.want, problem.CodeOf(fmt.Errorf("op: %w", tc.err)))
		})
	}
}

// The fs engine hands up the os error itself; its errno is what says "missing".
func TestCodeOf_AnOSPathErrorIsNotFound(t *testing.T) {
	_, err := os.Lstat(t.TempDir() + "/ghost")

	assert.Equal(t, problem.NotFound, problem.CodeOf(fmt.Errorf("mutate: copy: %w", err)))
}
