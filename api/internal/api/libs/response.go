package libs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

const IdempotencyKeyHeader = "Idempotency-Key"

type Envelope struct {
	Success bool         `json:"success"`
	Error   string       `json:"error,omitempty"`
	Code    problem.Code `json:"code,omitempty"`
	Data    any          `json:"data,omitempty"`
}

type mutationData struct {
	ID string `json:"id"`
}

func WriteQueryOK(
	c *gin.Context,
	data any,
) {
	WriteQueryWithStatus(
		c,
		http.StatusOK,
		data,
	)
}

func WriteQueryWithStatus(
	c *gin.Context,
	status int,
	data any,
) {
	c.JSON(
		status,
		Envelope{
			Success: true,
			Data:    data,
		},
	)
}

func WriteMutationOK(
	c *gin.Context,
	status int,
	id string,
) {
	c.JSON(
		status,
		Envelope{
			Success: true,
			Data: mutationData{
				ID: id,
			},
		},
	)
}

func WriteNoContent(
	c *gin.Context,
) {
	c.Status(http.StatusNoContent)
	c.Writer.WriteHeaderNow()
}

type operationData struct {
	Operation domain.Operation `json:"operation"`
}

func WriteOperation(
	c *gin.Context,
	op domain.Operation,
) {
	c.JSON(
		http.StatusAccepted,
		Envelope{
			Success: true,
			Data:    operationData{Operation: op},
		},
	)
}

func IdempotencyKey(
	c *gin.Context,
) string {
	return c.GetHeader(IdempotencyKeyHeader)
}

type Replayer interface {
	Replay(
		ctx context.Context, key string, kind domain.OperationKind, subject domain.EntityRef,
	) (domain.Operation, bool, error)
}

func Replayed(
	c *gin.Context,
	ops Replayer,
	kind domain.OperationKind,
	subject domain.EntityRef,
) bool {
	op, found, err := ops.Replay(c.Request.Context(), IdempotencyKey(c), kind, subject)
	if err != nil {
		Fail(c, err)
		return true
	}
	if found {
		WriteOperation(c, op)
	}
	return found
}

func Fail(
	c *gin.Context,
	err error,
) {
	p := problem.Of(err)
	status := p.Code.Status()
	if status >= http.StatusInternalServerError {
		NoteError(c, err)
	}
	c.JSON(
		status,
		Envelope{
			Success: false,
			Error:   p.Message,
			Code:    p.Code,
			Data:    problem.DataOf(err),
		},
	)
}

func WriteErr(
	c *gin.Context,
	status int,
	message string,
) {
	noteFailure(c, status, message)
	c.JSON(
		status,
		Envelope{
			Success: false,
			Error:   message,
			Code:    problem.CategoryFor(status),
		},
	)
}

func noteFailure(
	c *gin.Context,
	status int,
	message string,
) {
	if status >= http.StatusInternalServerError {
		NoteError(c, errors.New(message))
	}
}

func NoteError(
	c *gin.Context,
	err error,
) {
	c.Errors = append(c.Errors, &gin.Error{Err: err, Type: gin.ErrorTypePrivate})
}

func CloseLogged(
	ctx context.Context,
	what string,
	c io.Closer,
) {
	err := c.Close()
	if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, os.ErrClosed) {
		slog.WarnContext(ctx, what, "err", err)
	}
}
