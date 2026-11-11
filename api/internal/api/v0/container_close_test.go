//go:build integration

package v0_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
)

func TestAppClose_IdempotentAndEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	v0.New(tc.app, tc.eng, "")

	require.NotPanics(t, tc.app.Close) // no live resources: no-op
	assert.NotPanics(t, tc.app.Close)  // second call is safe
}
