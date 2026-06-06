package v0

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestDualServeRoutesPlainGETToREST(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var restHit, wsHit bool
	h := dualServe(
		func(_ *gin.Context) { restHit = true },
		func(_ *gin.Context) { wsHit = true },
	)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/workspaces", http.NoBody)

	h(c)

	assert.True(t, restHit)
	assert.False(t, wsHit)
}

func TestDualServeRoutesUpgradeToWS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var restHit, wsHit bool
	h := dualServe(
		func(_ *gin.Context) { restHit = true },
		func(_ *gin.Context) { wsHit = true },
	)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces", http.NoBody)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	c.Request = req

	h(c)

	assert.True(t, wsHit)
	assert.False(t, restHit)
}
