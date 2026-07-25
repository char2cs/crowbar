package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConnected proves the install-only probe: a cmd that resolves to an
// executable file on PATH reads connected, and one that resolves nowhere reads
// disconnected. It drives binpath.Resolve's PATH lookup by prepending a temp dir
// holding a fake executable, so the result never depends on whether the host has
// claude/codex installed.
func TestConnected(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fakecli")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	require.True(t, Connected("fakecli"), "an on-PATH executable is connected")
	require.False(t, Connected("definitely-not-a-real-cli-xyz"), "a missing cli is not connected")
}
