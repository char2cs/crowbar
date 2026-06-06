package health_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/health"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func TestRegisterMountsHealthRoute(
	t *testing.T,
) {
	r := gin.New()
	health.Register(r.Group("/v0"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/v0/health",
		nil,
	)
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	assert.True(t, body.Success)
	assert.Equal(t, "ok", body.Data.Status)
}
