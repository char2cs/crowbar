package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	crowbarapi "github.com/char2cs/crowbar/api/internal/api"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/engine"
)

func TestAPI_New_HealthRoute(t *testing.T) {
	ctx := context.Background()
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	a, err := app.New(ctx, eng, adapters)
	require.NoError(t, err)

	c, err := crowbarapi.New(a, eng, nil)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v0/health", nil)
	c.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ok")
}
