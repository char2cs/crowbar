package libs

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// DiscardWorkHeader is how a client that has shown the user the work a delete
// would destroy asks for it to be destroyed anyway. Anything but "true" keeps it.
const DiscardWorkHeader = "Crowbar-Discard-Work"

// WorkAtRiskCode marks a delete refused over work at risk; data.workAtRisk
// lists it, so the client can ask the user and retry with DiscardWorkHeader.
const WorkAtRiskCode = "work_at_risk"

// DeleteConsentOf reads a delete request's consent to destroy work at risk.
func DeleteConsentOf(
	c *gin.Context,
) domain.DeleteConsent {
	if c.GetHeader(DiscardWorkHeader) == "true" {
		return domain.DiscardWorkAtRisk
	}
	return domain.KeepWorkAtRisk
}

// WriteDeleteErr writes a delete's failure. A refusal over work at risk is a
// 409 carrying the structured list; anything else maps as StatusAndMessage does.
func WriteDeleteErr(
	c *gin.Context,
	err error,
) {
	var risk *domain.WorkAtRiskError
	if !errors.As(err, &risk) {
		status, msg := StatusAndMessage(err)
		WriteErr(c, status, msg)
		return
	}
	c.JSON(http.StatusConflict, Envelope{
		Success: false,
		Error:   risk.Error(),
		Code:    WorkAtRiskCode,
		Data:    gin.H{"workAtRisk": risk.Workspaces},
	})
}
