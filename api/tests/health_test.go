//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
