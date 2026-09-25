//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// One legacy row whose history cannot be replayed used to fail app.New: the
// boot backfill returned the first non-validation error and the daemon never
// started. Boot writers are best-effort per row — the bad row is logged and
// skipped, every other row is still upgraded, and the daemon serves.
func TestRegression_Upgrade_AnUnreplayableRowNeverBlocksBoot(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	imported := importProject(t, h)
	h.Quiesce()
	h.crash()

	now := time.Now().UTC()
	legacy := func(branch string) domain.Workspace {
		return domain.Workspace{
			ID: uuid.NewString(), RepoID: imported.repoID, ProjectID: imported.projectID,
			Branch: branch, ParentID: imported.workspaceID, MergeStrategy: "merge",
			CreatedAt: now, LastActivity: now,
		}
	}
	broken, healthy := legacy("broken"), legacy("healthy")
	seed := openBaseEra(t, home)
	seed.unreplayableWorkspace(broken)
	seed.workspace(healthy)
	seed.close()

	h2 := newHarnessAt(t, home) // requires app.New to succeed
	h2.Quiesce()

	row, err := h2.app.Repositories.Workspace.Get(context.Background(), healthy.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.WorkspacePlaceholder, row.Provisioning, "the other rows are still backfilled")
	h2.get("/v0/projects", nil)
}
