// Package libs provides the uniform HTTP response envelope shared by every v0
// route handler. The envelope mirrors quiver.core's shape —
// { success: bool, error: string|null, data: any? } — but drops quiver's
// namespace field, because Crowbar addresses entities by UUID path params
// rather than by namespace.
//
// Query responses carry their payload in data. Mutation responses carry the
// affected entity id under a fixed shape: data: { "id": "<uuid>" }. Error
// responses set success:false and a non-null error string, with data omitted.
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

// Envelope is the uniform response body returned by every v0 handler.
//
// Success is true for query and mutation responses and false for errors. Error
// holds a non-null human-readable message on failures and is omitted on
// success. Data carries the query payload or the mutation id shape and is
// omitted when empty (for example on errors).
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

// WriteAccepted writes a 202 Accepted response with an empty body. Use it for
// fail-fast/good-path-async mutations whose outcome is delivered later on the
// entity's WebSocket stream rather than in the HTTP response (00 §4).
func WriteAccepted(
	c *gin.Context,
) {
	c.Status(http.StatusAccepted)
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

// Fail writes err as a failure: the status of its problem code, its whole
// chain as the message, and the data its coded error carries.
func Fail(
	c *gin.Context,
	err error,
) {
	p := problem.Of(err)
	status := p.Code.Status()
	noteFailure(c, status, p.Message)
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
