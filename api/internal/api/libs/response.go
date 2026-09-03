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
	"net/http"

	"github.com/gin-gonic/gin"
)

// Envelope is the uniform response body returned by every v0 handler.
//
// Success is true for query and mutation responses and false for errors. Error
// holds a non-null human-readable message on failures and is omitted on
// success. Data carries the query payload or the mutation id shape and is
// omitted when empty (for example on errors).
type Envelope struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	// Code is a stable machine-readable failure category for endpoints whose
	// recovery UX must distinguish errors sharing one HTTP status. Existing
	// errors omit it and retain their wire shape.
	Code string `json:"code,omitempty"`
	Data any    `json:"data,omitempty"`
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

// WriteErr writes an error response with the given HTTP status. The envelope
// sets success:false and carries message as a non-null error string; data is
// omitted.
func WriteErr(
	c *gin.Context,
	status int,
	message string,
) {
	c.JSON(
		status,
		Envelope{
			Success: false,
			Error:   message,
		},
	)
}

// WriteErrCode writes a failure with both a human-readable message and a stable
// machine-readable code. Use it only when the client has distinct recovery
// actions for errors sharing an HTTP status (for example prompt busy vs an
// uncertain delivery outcome).
func WriteErrCode(
	c *gin.Context,
	status int,
	code string,
	message string,
) {
	c.JSON(
		status,
		Envelope{
			Success: false,
			Error:   message,
			Code:    code,
		},
	)
}
