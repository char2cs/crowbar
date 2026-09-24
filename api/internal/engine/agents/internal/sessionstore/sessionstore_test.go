package sessionstore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/sessionstore"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func touch(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte("{}"), 0o600))
}

func TestExists_FindsACodexRolloutInItsDatedDirectory(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "sessions/2026/09/24/rollout-2026-09-24T16-28-14-thread-1.jsonl"))
	touch(t, filepath.Join(root, "sessions/2026/09/23/rollout-2026-09-23T10-00-00-thread-0.jsonl"))
	loc := &spec.SessionLocateSpec{Root: root, Glob: []string{"sessions/*/*/*/rollout-*-{id}.jsonl"}}
	f := sessionstore.New()

	exists, declared := f.Exists("codex", loc, "thread-1")
	assert.True(t, declared)
	assert.True(t, exists)

	exists, _ = f.Exists("codex", loc, "thread-9")
	assert.False(t, exists, "a session the provider never wrote is not resumable")
}

func TestExists_FindsAClaudeTranscriptUnderAnyProject(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "projects/-work-tree-a/sess-1.jsonl"))
	loc := &spec.SessionLocateSpec{Root: root, Glob: []string{"projects/*/{id}.jsonl"}}

	exists, _ := sessionstore.New().Exists("claude", loc, "sess-1")

	assert.True(t, exists, "found whichever project directory the session was written under")
}

func TestExists_TheRootEnvironmentVariableWins(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "s/sess-1.jsonl"))
	t.Setenv("FAKE_PROVIDER_HOME", root)
	loc := &spec.SessionLocateSpec{RootEnv: "FAKE_PROVIDER_HOME", Root: "/nonexistent", Glob: []string{"s/{id}.jsonl"}}

	exists, _ := sessionstore.New().Exists("p", loc, "sess-1")

	assert.True(t, exists)
}

func TestExists_ARemovedSessionIsNotServedFromTheCache(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "s/sess-1.jsonl")
	touch(t, path)
	loc := &spec.SessionLocateSpec{Root: root, Glob: []string{"s/{id}.jsonl"}}
	f := sessionstore.New()
	exists, _ := f.Exists("p", loc, "sess-1")
	require.True(t, exists)

	require.NoError(t, os.Remove(path))
	exists, _ = f.Exists("p", loc, "sess-1")

	assert.False(t, exists)
}

func TestExists_AnIDThatCouldEscapeItsPatternIsNeverFound(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "secret.jsonl"))
	loc := &spec.SessionLocateSpec{Root: root, Glob: []string{"s/{id}.jsonl"}}

	for _, id := range []string{"../secret", "*", "a/b", ""} {
		exists, declared := sessionstore.New().Exists("p", loc, id)
		assert.True(t, declared)
		assert.False(t, exists, "id %q", id)
	}
}

func TestExists_NoLocateMeansUndeclared(t *testing.T) {
	_, declared := sessionstore.New().Exists("p", nil, "sess-1")

	assert.False(t, declared)
}
