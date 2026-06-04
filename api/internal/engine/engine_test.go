package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_ReturnsContainer(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir("/tmp/x"))
	require.NoError(t, err)
	assert.NotNil(t, c)
}
