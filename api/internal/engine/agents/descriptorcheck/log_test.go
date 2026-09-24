package descriptorcheck_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

// A descriptor the daemon refuses says why at boot, with its rule and line.
func TestLogAll_ABlockedDescriptorIsAnErrorWithItsRuleAndLine(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(home, "descriptors", "codex.yaml"),
		[]byte("id: codex\nspawn:\n  cmd: codex\n"), 0o600))
	var out bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	descriptorcheck.LogAll(context.Background(), home)

	logged := out.String()
	assert.Contains(t, logged, "level=ERROR")
	assert.Contains(t, logged, `msg="agent: descriptor blocked"`)
	assert.Contains(t, logged, "provider=codex")
	assert.Contains(t, logged, "rule=load.spawn_command")
	assert.Contains(t, logged, "line=2")
	assert.NotContains(t, logged, "provider=claude", "the shipped claude descriptor is clean")
}
