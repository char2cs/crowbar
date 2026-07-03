package terminal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModelDrivenEnabled_EnvOverridesDefault(t *testing.T) {
	t.Setenv("CROWBAR_TERMINAL_MODEL_DRIVEN", "0")
	assert.False(t, modelDrivenEnabled())
	t.Setenv("CROWBAR_TERMINAL_MODEL_DRIVEN", "false")
	assert.False(t, modelDrivenEnabled())
	t.Setenv("CROWBAR_TERMINAL_MODEL_DRIVEN", "1")
	assert.True(t, modelDrivenEnabled())
	t.Setenv("CROWBAR_TERMINAL_MODEL_DRIVEN", "true")
	assert.True(t, modelDrivenEnabled())
	t.Setenv("CROWBAR_TERMINAL_MODEL_DRIVEN", "")
	assert.Equal(t, modelDrivenBuildDefault, modelDrivenEnabled())
}
