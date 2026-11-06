package migrations

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

// An override an older build's schema accepted shadows a shipped descriptor the daemon
// runs instead, warning at every boot; it is moved aside once, kept for the user to read.
// A custom provider has no shipped fallback, so its findings stay where the user sees them.
func TestRefusedDescriptors_MovesARefusedOverrideAsideButNotACustomProvider(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "descriptors/claude.yaml", "id: claude\nsession:\n  locate: {}\n")
	testutil.WriteFile(t, h, "descriptors/acme.yaml", "id: acme\nnot a descriptor\n")

	finish(t, boot(t, h))

	root := h.Root()
	assert.NoFileExists(t, filepath.Join(root, "descriptors/claude.yaml"))
	moved, err := os.ReadFile(filepath.Join(root, "descriptors/claude.yaml.refused"))
	require.NoError(t, err)
	assert.Contains(t, string(moved), "locate")
	assert.FileExists(t, filepath.Join(root, "descriptors/acme.yaml"))
	assert.Equal(t, []migrate.Counts{{"retired": 1}}, migratetest.Applied(t, h)[StepRefusedDescriptors])
}

func TestRefusedDescriptors_HasNothingToDoWithoutOverrides(t *testing.T) {
	h := testutil.FixtureHome(t)

	finish(t, boot(t, h))

	assert.Equal(t, []migrate.Counts{{"retired": 0}}, migratetest.Applied(t, h)[StepRefusedDescriptors])
}
