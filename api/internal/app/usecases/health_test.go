package usecases

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHealthCheck_ReturnsOk(t *testing.T) {
	uc := NewHealth()
	status := uc.Check()
	assert.Equal(t, "ok", status.Status)
	assert.NotEmpty(t, status.Version)
}

func TestNewHealth_NotNil(t *testing.T) {
	assert.NotNil(t, NewHealth())
}
