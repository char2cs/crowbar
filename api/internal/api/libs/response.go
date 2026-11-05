// Package libs provides the uniform response envelope every v0 handler returns:
// { success, error, data }, quiver.core's shape minus its namespace field, since
// Crowbar addresses entities by UUID path params. Queries carry their payload in
// data, mutations carry data: { "id": "<uuid>" }, errors set success:false and omit data.
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

// IdempotencyKeyHeader carries the client-minted id of the operation a request starts.
const IdempotencyKeyHeader = "Idempotency-Key"

// Envelope is the uniform response body returned by every v0 handler. Error is set
// only on failure; Data carries the query payload or the mutation id and is omitted
// when empty.
type Envelope struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	// Code is the failure's problem code, the one thing a client branches on.
	Code problem.Code `json:"code,omitempty"`
	Data any          `json:"data,omitempty"`
}

// mutationData is the fixed data shape returned by mutation responses: the id
// of the affected entity.
type mutationData struct {
	ID string `json:"id"`
}

// WriteQueryOK writes a 200 OK query response whose envelope carries data.
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

// WriteQueryWithStatus writes a query response with the given HTTP status whose
// envelope carries data. Use it for non-200 success codes such as 206.
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

// WriteMutationOK writes a mutation response with the given HTTP status whose
// envelope carries the affected entity id under data: { "id": id }. Use status
// 201 for creates and 200 for in-place updates.
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

// WriteNoContent answers 204 for a mutation that finished inside the request;
// its outcome rides the entity's stream.
func WriteNoContent(
	c *gin.Context,
) {
	c.Status(http.StatusNoContent)
	c.Writer.WriteHeaderNow()
}

// operationData is the data shape of a route that hands its work to an operation.
type operationData struct {
	Operation domain.Operation `json:"operation"`
}

// WriteOperation answers 202 with the operation the request started.
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

// IdempotencyKey is the operation id the client minted, or "" for none.
func IdempotencyKey(
	c *gin.Context,
) string {
	return c.GetHeader(IdempotencyKeyHeader)
}

// Replayer finds the operation a retried request already started.
type Replayer interface {
	Replay(
		ctx context.Context, key string, kind domain.OperationKind, subject domain.EntityRef,
	) (domain.Operation, bool, error)
}

// Replayed answers a retry of a kind-starting request with its first operation
// and reports whether it answered: the retry of a finished delete must not
// reach the lookup of the entity it removed.
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

// Fail writes err as a failure: the status of its problem code, its user-facing
// message, and the data its coded error carries. A 5xx's whole chain goes to the
// access log only.
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

// WriteErr writes a failure a handler decided on itself, with no error behind
// it; its code is the category of status.
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

// noteFailure hands a 5xx's message to gin's error list so the access log can
// print the cause; the envelope is otherwise the only place it ever existed.
func noteFailure(
	c *gin.Context,
	status int,
	message string,
) {
	if status >= http.StatusInternalServerError {
		NoteError(c, errors.New(message))
	}
}

// NoteError puts err on gin's error list for the access log: once a response is
// under way, that is the one place a failure can still go.
func NoteError(
	c *gin.Context,
	err error,
) {
	c.Errors = append(c.Errors, &gin.Error{Err: err, Type: gin.ErrorTypePrivate})
}

// CloseLogged closes c, which nobody is left to hand a failure to, and logs one;
// a connection or file something else already closed is not a failure.
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
