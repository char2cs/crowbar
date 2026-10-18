package agents_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRefusedResume_NeedsTheExitCodeAndTheLine(t *testing.T) {
	claude := get(t, "claude")
	screen := "loading\n  No conversation found with session ID: abc  \nbye\n"

	line, refused := claude.RefusedResume(1, screen)
	assert.True(t, refused)
	assert.Equal(t, "No conversation found with session ID: abc", line)

	_, refused = claude.RefusedResume(1, "Killed")
	assert.False(t, refused, "an exit without the line is not a refusal")
	_, refused = claude.RefusedResume(143, screen)
	assert.False(t, refused, "the line under another exit code is not a refusal")

	_, refused = get(t, "codex").RefusedResume(1, "Error: no rollout found for thread id 019a")
	assert.True(t, refused)
	_, refused = stubAgent(t, "sh").RefusedResume(1, "No conversation found with session ID: abc")
	assert.False(t, refused, "a descriptor declaring no refusal never refuses")
}

func TestSessionAssignArg_IsClaudesAlone(t *testing.T) {
	arg, ok := get(t, "claude").SessionAssignArg()
	assert.True(t, ok)
	assert.Equal(t, "--session-id {id}", arg)

	_, ok = get(t, "codex").SessionAssignArg()
	assert.False(t, ok, "codex announces its own thread id")
}
