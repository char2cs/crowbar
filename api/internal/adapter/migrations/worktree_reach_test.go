package migrations

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

// A home an earlier build narrowed owner-only, the way to its checkouts
// included, gets traverse back on that way and nothing else, once.
func TestWorktreeReach_RepairsTheWayToACheckoutAnEarlierBuildNarrowed(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	h := testutil.FixtureHome(t)
	root := h.Root()
	testutil.WriteFile(t, h, "projects/p1/acme-app/main/worktree/README.md", "user")
	testutil.WriteFile(t, h, "projects/p1/acme-app/main/chats/c1/log.json", "{}")
	way := []string{".", "projects", "projects/p1", "projects/p1/acme-app", "projects/p1/acme-app/main"}
	for _, dir := range append(way, "projects/p1/acme-app/main/chats") {
		chmod(t, root, dir, 0o700)
	}

	finish(t, boot(t, h))

	for _, dir := range way {
		assert.Equal(t, os.FileMode(0o711), mode(t, root, dir), dir)
	}
	assert.Equal(t, os.FileMode(0o700), mode(t, root, "projects/p1/acme-app/main/chats"))
	assert.Equal(t, []migrate.Counts{{"reachable": 5}}, migratetest.Applied(t, h)[StepWorktreeReach])
	finish(t, boot(t, h))
	assert.Len(t, migratetest.Applied(t, h)[StepWorktreeReach], 1, "a repaired home owes nothing")
}
