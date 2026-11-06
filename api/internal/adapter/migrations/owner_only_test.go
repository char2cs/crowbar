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

func mode(t *testing.T, root, rel string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(filepath.Join(root, rel))
	require.NoError(t, err)
	return info.Mode().Perm()
}

func chmod(t *testing.T, root, rel string, m os.FileMode) {
	t.Helper()
	require.NoError(t, os.Chmod(filepath.Join(root, rel), m))
}

func TestOwnerOnly_NarrowsWhatAnOlderBuildLeftOpenButNotACheckout(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	h := testutil.FixtureHome(t)
	root := h.Root()
	testutil.WriteFile(t, h, "model-manifest-cache/m.json", "{}")
	testutil.WriteFile(t, h, "projects/p1/acme-app/main/worktree/README.md", "user")
	chmod(t, root, "model-manifest-cache/m.json", 0o644)
	chmod(t, root, "model-manifest-cache", 0o750)
	chmod(t, root, "projects/p1/acme-app/main/worktree/README.md", 0o644)
	opened := []string{
		"projects",
		"projects/p1",
		"projects/p1/acme-app",
		"projects/p1/acme-app/main",
		"projects/p1/acme-app/main/worktree",
	}
	for _, dir := range opened {
		chmod(t, root, dir, 0o755)
	}

	finish(t, boot(t, h))

	assert.Equal(t, os.FileMode(0o600), mode(t, root, "model-manifest-cache/m.json"))
	assert.Equal(t, os.FileMode(0o700), mode(t, root, "model-manifest-cache"))
	assert.Equal(t, os.FileMode(0o700), mode(t, root, "projects/p1/acme-app/main"), "Crowbar's scaffolding above a checkout")
	assert.Equal(t, os.FileMode(0o755), mode(t, root, "projects/p1/acme-app/main/worktree"), "a checkout is git's")
	assert.Equal(t, os.FileMode(0o644), mode(t, root, "projects/p1/acme-app/main/worktree/README.md"))
	assert.Equal(t, []migrate.Counts{{"narrowed": 6}}, migratetest.Applied(t, h)[StepOwnerOnly])
}

func TestOwnerOnly_HasNothingToDoOnAHomeThatIsOwnerOnly(t *testing.T) {
	h := testutil.FixtureHome(t)

	finish(t, boot(t, h))

	assert.Equal(t, []migrate.Counts{{"narrowed": 0}}, migratetest.Applied(t, h)[StepOwnerOnly])
}
