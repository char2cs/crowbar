package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/health/handlers"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func TestCheckReturnsEnvelopeOK(
	t *testing.T,
) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/health",
		nil,
	)

	handlers.Check(ctx)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
		Data    struct {
			Status  string `json:"status"`
			Version string `json:"version"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	assert.True(t, body.Success)
	assert.Empty(t, body.Error)
	assert.Equal(t, "ok", body.Data.Status)
	assert.NotEmpty(t, body.Data.Version)
}
