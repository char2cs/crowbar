package agents_test

import (
	"strings"
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

// The exit screen holds terminal rows, trimmed of trailing blanks: a refusal the
// terminal wrapped, mid-word or on a space, is still the refusal.
func TestRefusedResume_ALineWrappedAcrossRowsIsStillTheRefusal(t *testing.T) {
	claude := get(t, "claude")
	want := "No conversation found with session ID: abc"
	for name, tc := range map[string]struct{ screen, line string }{
		"80 columns": {
			strings.Repeat("0", 60) + "No conversation foun\nd with session ID: abc\n\n\n",
			strings.Repeat("0", 60) + want,
		},
		"20 columns, one break on a space": {
			"Fatal error: No conv\nersation found with\nsession ID: abc\n",
			"Fatal error: " + want,
		},
	} {
		t.Run(name, func(t *testing.T) {
			line, refused := claude.RefusedResume(1, tc.screen)
			assert.True(t, refused)
			assert.Equal(t, tc.line, line)
		})
	}

	_, refused := claude.RefusedResume(143, "No conversation found\nwith session ID: abc\n")
	assert.False(t, refused, "a needle across two lines under another exit code is not a refusal")
}

func TestSessionAssignArg_IsClaudesAlone(t *testing.T) {
	arg, ok := get(t, "claude").SessionAssignArg()
	assert.True(t, ok)
	assert.Equal(t, "--session-id {id}", arg)

	_, ok = get(t, "codex").SessionAssignArg()
	assert.False(t, ok, "codex announces its own thread id")
}

// codex writes no rollout until a turn completes; claude writes its session
// as soon as a turn starts.
func TestSessionPersists_IsWhenTheCLIHasWrittenItsSession(t *testing.T) {
	assert.Equal(t, "turn_end", get(t, "codex").SessionPersists())
	assert.Equal(t, "turn_start", get(t, "claude").SessionPersists())
}
