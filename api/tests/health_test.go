//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHealth_Enveloped proves GET /v0/health returns a 200 success envelope: the
// liveness check the rest of the suite depends on.
func TestHealth_Enveloped(t *testing.T) {
	h := newHarness(t)

	var data struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	h.get("/v0/health", &data)
	assert.Equal(t, "ok", data.Status)
	assert.NotEmpty(t, data.Version)
}
