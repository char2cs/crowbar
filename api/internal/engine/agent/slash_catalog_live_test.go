package agent

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// This opt-in barrier exercises only deterministic provider subcommands. It
// starts no TUI and no model turn, logs no command output, and exists to catch
// installed-CLI output drift without making ordinary CI depend on local tools.
func TestLiveDeterministicSlashCatalogs(t *testing.T) {
	if os.Getenv("CROWBAR_LIVE_PROVIDER_PROBES") != "1" {
		t.Skip("set CROWBAR_LIVE_PROVIDER_PROBES=1 to exercise installed deterministic CLI probes")
	}
	cwd, err := os.Getwd()
	require.NoError(t, err)
	for _, providerID := range []string{"codex", "claude"} {
		t.Run(providerID, func(t *testing.T) {
			if providerID == "codex" && os.Getenv("CODEX_SANDBOX") != "" {
				// A nested codex process is intentionally denied by Codex's own
				// seatbelt. Run this barrier from a normal developer shell or the
				// packaged daemon to exercise codex itself.
				t.Skip("nested codex deterministic probe is unavailable inside the Codex seatbelt")
			}
			d, err := ResolveDescriptor(t.TempDir(), providerID)
			require.NoError(t, err)
			if !Connected(d.Spawn.Cmd) {
				t.Skipf("%s is not installed", providerID)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			catalog, err := ProbeSlashCatalog(ctx, d, SlashCatalogProbeOptions{Cwd: cwd})
			require.NoError(t, err)
			require.NotEmpty(t, catalog.Items)
			t.Logf("mapped %d %s items (%s)", len(catalog.Items), CatalogItemKindSkill, catalog.Completeness)
		})
	}
}
