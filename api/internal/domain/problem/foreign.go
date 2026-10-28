package problem

import (
	"context"
	"io/fs"

	asynxmodels "github.com/char2cs/asynx/models"
)

// foreign codes the errors Crowbar does not own and so cannot declare coded.
var foreign = []struct {
	err  error
	code Code
}{
	{fs.ErrNotExist, NotFound},
	{fs.ErrExist, Conflict},
	{fs.ErrInvalid, InvalidArgument},
	{asynxmodels.ErrNotFound, NotFound},
	{asynxmodels.ErrValidation, Unprocessable},
	{asynxmodels.ErrPipelineFailed, Conflict},
	{asynxmodels.ErrQueueFull, Unavailable},
	{asynxmodels.ErrShuttingDown, Unavailable},
	// A parked aggregate refuses until the daemon restarts; one being forgotten
	// is going away under a concurrent delete.
	{asynxmodels.ErrDeliveryParked, Unavailable},
	{asynxmodels.ErrForgetInProgress, Conflict},
	{context.DeadlineExceeded, Timeout},
}

func foreignCode(node error) (Code, bool) {
	for _, f := range foreign {
		if is(node, f.err) {
			return f.code, true
		}
	}
	return "", false
}
