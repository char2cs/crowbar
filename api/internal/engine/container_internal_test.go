package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunsDir_EmptyBase(t *testing.T) {
	assert.Equal(t, "run-123", runsDir("", "run-123"))
}

func TestRunsDir_WithBase(t *testing.T) {
	assert.Equal(t, "/home/runs/run-123", runsDir("/home/runs", "run-123"))
}
