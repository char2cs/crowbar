package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	systemhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/system/handlers"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

// envelope mirrors the {success, data} query envelope every v0 read answers with.
type envelope struct {
	Success bool                             `json:"success"`
	Data    systemhandlers.PrerequisitesData `json:"data"`
}

// doPrerequisites serves GET /v0/system/prerequisites with reqCtx as the request
// context, so a test can decide whether the probes are allowed to run at all.
func doPrerequisites(
	reqCtx context.Context,
) *httptest.ResponseRecorder {
	r := gin.New()
	h := systemhandlers.NewHandler(context.Background())
	r.GET("/v0/system/prerequisites", h.Prerequisites)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/system/prerequisites", http.NoBody)
	r.ServeHTTP(rec, req.WithContext(reqCtx))
	return rec
}

// TestPrerequisites_UnavailableToolsReportNotInstalled pins the degraded answer:
// when no probe can run, the endpoint still answers 200 with every tool reported
// absent rather than erroring. A CANCELLED request context is what makes this
// deterministic — every exec.CommandContext fails immediately, on any machine,
// with no dependency on which CLIs happen to be installed and no network call
// (a real `gh auth status` would make one).
func TestPrerequisites_UnavailableToolsReportNotInstalled(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rec := doPrerequisites(ctx)

	require.Equal(t, http.StatusOK, rec.Code, "a missing toolchain is data, not an error")
	var got envelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.True(t, got.Success)
	assert.False(t, got.Data.Git.Installed)
	assert.Empty(t, got.Data.Git.Version)
	assert.False(t, got.Data.Gh.Installed)
	assert.False(t, got.Data.Gh.Authed)
	assert.False(t, got.Data.Glab.Installed)
	assert.False(t, got.Data.Glab.Authed)
}
