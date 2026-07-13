package agent_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestAllDescriptors_EnumeratesEmbeddedProviders(t *testing.T) {
	got, err := agent.AllDescriptors(t.TempDir()) // empty home → embedded only
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, d := range got {
		ids[d.ID] = true
	}
	require.True(t, ids["claude"], "claude descriptor must be enumerated")
	require.True(t, ids["codex"], "codex descriptor must be enumerated")
}

func TestAllDescriptors_OnDiskOverrideWinsAndAddsNewIds(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	// A brand-new on-disk-only provider id (future user-managed provider).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.yaml"), []byte(`id: extra
display_name: Extra
spawn:
  cmd: "true"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: message }
`), 0o644))

	got, err := agent.AllDescriptors(home)
	require.NoError(t, err)
	byID := map[string]*agent.Descriptor{}
	for _, d := range got {
		byID[d.ID] = d
	}
	require.Contains(t, byID, "extra", "an on-disk-only provider must appear in the enumeration")
	require.Equal(t, "Extra", byID["extra"].DisplayName)
	require.Contains(t, byID, "claude", "embedded providers still enumerate alongside on-disk ones")
	// Sorted by id, deterministic.
	for i := 1; i < len(got); i++ {
		require.Less(t, got[i-1].ID, got[i].ID, "AllDescriptors must be sorted by id")
	}
}
