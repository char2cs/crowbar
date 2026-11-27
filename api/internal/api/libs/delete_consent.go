package libs

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/domain"
)

const DiscardWorkHeader = "Crowbar-Discard-Work"

func DeleteConsentOf(
	c *gin.Context,
) domain.DeleteConsent {
	if c.GetHeader(DiscardWorkHeader) == "true" {
		return domain.DiscardWorkAtRisk
	}
	return domain.KeepWorkAtRisk
}
