package libs

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// DiscardWorkHeader is how a client that has shown the user the work a delete
// would destroy asks for it to be destroyed anyway. Anything but "true" keeps it.
const DiscardWorkHeader = "Crowbar-Discard-Work"

// DeleteConsentOf reads a delete request's consent to destroy work at risk.
func DeleteConsentOf(
	c *gin.Context,
) domain.DeleteConsent {
	if c.GetHeader(DiscardWorkHeader) == "true" {
		return domain.DiscardWorkAtRisk
	}
	return domain.KeepWorkAtRisk
}
