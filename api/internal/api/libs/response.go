package libs

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type apiResponse struct {
	Success bool    `json:"success"`
	Error   *string `json:"error,omitempty"`
	ID      string  `json:"id,omitempty"`
	Data    any     `json:"data,omitempty"`
}

// WriteMutationOK writes a successful mutation response with the given HTTP
// status code and resource ID.
func WriteMutationOK(
	c *gin.Context,
	status int,
	id string,
) {
	c.JSON(status, apiResponse{Success: true, ID: id})
}

// WriteQueryOK writes a successful query response with HTTP 200 and the
// provided data payload.
func WriteQueryOK(
	c *gin.Context,
	data any,
) {
	c.JSON(http.StatusOK, apiResponse{Success: true, Data: data})
}

// WriteErr writes an error response with the given HTTP status, message, and
// optional resource ID.
func WriteErr(
	c *gin.Context,
	status int,
	message string,
	id string,
) {
	c.JSON(status, apiResponse{Success: false, Error: &message, ID: id})
}

// DataResponse returns a success envelope containing data, suitable for use
// with c.JSON when a non-200 status code is required.
func DataResponse(
	data any,
) apiResponse {
	return apiResponse{Success: true, Data: data}
}
