package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/health/handlers"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
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
			PID     int    `json:"pid"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	assert.True(t, body.Success)
	assert.Empty(t, body.Error)
	assert.Equal(t, "ok", body.Data.Status)
	assert.NotEmpty(t, body.Data.Version)
	// The desktop supervisor reads the daemon's pid from here — asking the
	// tauri-shell child handle for it deadlocks against the plugin's blocking
	// wait thread, so the daemon self-reports instead.
	assert.Equal(t, os.Getpid(), body.Data.PID)
}

// The desktop shell's tests parse this fixture, which the boot gate's tests also
// own; a changed spelling of the ready state has to change it, and them, with it.
func TestCheckMatchesTheDesktopFixture(
	t *testing.T,
) {
	want, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "core", "bootgate", "testdata", "health-ok.json"))
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)

	handlers.Check(ctx)

	body := strings.Replace(rec.Body.String(), `"pid":`+strconv.Itoa(os.Getpid()), `"pid":4242`, 1)
	body = strings.Replace(body, `"version":"`+metadata.GetVersion()+`"`, `"version":"0.1.0"`, 1)
	assert.Equal(t, string(want), body)
}
