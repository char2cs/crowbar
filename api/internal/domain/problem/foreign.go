package problem

import (
	"context"
	"io/fs"

	asynxmodels "github.com/char2cs/asynx/models"
)

var foreign = []struct {
	err  error
	code Code
}{
	{fs.ErrNotExist, NotFound},
	{fs.ErrExist, Conflict},
	{fs.ErrInvalid, InvalidArgument},
	{fs.ErrPermission, Forbidden},
	{asynxmodels.ErrNotFound, NotFound},
	{asynxmodels.ErrValidation, Unprocessable},
	{asynxmodels.ErrPipelineFailed, Conflict},
	{asynxmodels.ErrQueueFull, Unavailable},
	{asynxmodels.ErrShuttingDown, Unavailable},
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
