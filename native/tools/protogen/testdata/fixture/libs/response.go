// Package libs mirrors the daemon's uniform response envelope: the wrapper
// every handler writes its payload through, and the helper names protogen
// recognises.
package libs

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Envelope is the uniform response body.
type Envelope struct {
	// Success is true for query and mutation responses.
	Success bool `json:"success"`
	// Error carries the message on failure.
	Error string `json:"error,omitempty"`
	// Data carries the payload.
	Data any `json:"data,omitempty"`
}

// mutationData is the fixed data shape of a mutation response.
type mutationData struct {
	// ID is the affected entity.
	ID string `json:"id"`
}

// WriteQueryOK writes a 200 query response carrying data.
func WriteQueryOK(
	c *gin.Context,
	data any,
) {
	WriteQueryWithStatus(c, http.StatusOK, data)
}

// WriteQueryWithStatus writes a query response with an explicit status.
func WriteQueryWithStatus(
	c *gin.Context,
	status int,
	data any,
) {
	c.JSON(status, Envelope{Success: true, Data: data})
}

// WriteMutationOK writes the fixed mutation envelope.
func WriteMutationOK(
	c *gin.Context,
	status int,
	id string,
) {
	c.JSON(status, Envelope{Success: true, Data: mutationData{ID: id}})
}

// WriteAccepted writes a 202 with an empty body.
func WriteAccepted(
	c *gin.Context,
) {
	c.Status(http.StatusAccepted)
}

// WriteErr writes an error envelope.
func WriteErr(
	c *gin.Context,
	status int,
	message string,
) {
	c.JSON(status, Envelope{Success: false, Error: message})
}
