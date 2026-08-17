package env_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/env"
)

func TestClear_RemovesNamedVariablesOnly(t *testing.T) {
	environ := []string{"A=1", "CLAUDECODE=1", "B=2", "CLAUDE_CODE_CHILD_SESSION=x"}

	got := env.Clear(environ, []string{"CLAUDECODE", "CLAUDE_CODE_CHILD_SESSION"})

	assert.Equal(t, []string{"A=1", "B=2"}, got)
}

func TestClear_WithNoNamesCopiesRatherThanAliases(t *testing.T) {
	environ := []string{"A=1"}

	got := env.Clear(environ, nil)

	assert.Equal(t, environ, got)
	got[0] = "MUTATED=1"
	assert.Equal(t, []string{"A=1"}, environ, "the caller's slice must not be aliased")
}

func TestClear_HandlesEntriesWithNoEquals(t *testing.T) {
	got := env.Clear([]string{"BARE", "A=1"}, []string{"BARE"})

	assert.Equal(t, []string{"A=1"}, got)
}

func TestReplace_LeavesExactlyOneEntry(t *testing.T) {
	got := env.Replace([]string{"PWD=/old", "A=1", "PWD=/older"}, "PWD", "/new")

	assert.Equal(t, []string{"A=1", "PWD=/new"}, got)
}

func TestReplace_DoesNotMatchOnPrefix(t *testing.T) {
	got := env.Replace([]string{"PWDX=keep"}, "PWD", "/new")

	assert.Equal(t, []string{"PWDX=keep", "PWD=/new"}, got)
}

func TestLookup_LastEntryWins(t *testing.T) {
	value, ok := env.Lookup([]string{"PATH=/a", "PATH=/b"}, "PATH")

	assert.True(t, ok)
	assert.Equal(t, "/b", value, "exec resolves duplicates last-wins; Lookup must agree")
}

func TestLookup_MissingIsNotFound(t *testing.T) {
	_, ok := env.Lookup([]string{"A=1"}, "PATH")

	assert.False(t, ok)
}

func TestLookup_EmptyValueIsStillPresent(t *testing.T) {
	value, ok := env.Lookup([]string{"PATH="}, "PATH")

	assert.True(t, ok)
	assert.Empty(t, value)
}
