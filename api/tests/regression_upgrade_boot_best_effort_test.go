//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

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

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	row, err := h2.app.Repositories.Workspace.Get(context.Background(), healthy.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.WorkspacePlaceholder, row.Provisioning, "the other rows are still backfilled")
	h2.get("/v0/projects", nil)
}

func TestRegression_Upgrade_AnUnreplayableUntypedChatIsSkippedNotFatal(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	imported := importProject(t, h)
	h.Quiesce()
	h.crash()

	now := time.Now().UTC()
	legacy := func(title string) domain.Chat {
		return domain.Chat{
			ID: uuid.NewString(), WorkspaceID: imported.workspaceID,
			Title: title, CreatedAt: now,
		}
	}
	broken, healthy := legacy("broken"), legacy("healthy")
	seed := openBaseEra(t, home)
	seed.unreplayableChat(broken)
	seed.chat(healthy)
	seed.close()

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	got, err := h2.app.Usecases.AgentChat.GetChat(context.Background(), healthy.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ChatTypeChat, got.Type)
	applied := migratetest.Applied(t, homeAt(t, home))
	assert.Equal(t, []migrate.Counts{{"skipped_rows": 1}}, applied[migrations.StepChatTypes])
}

func TestRegression_Upgrade_AHomeTombstoneIsNotAMissingHome(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	imported := importProject(t, h)
	h.Quiesce()
	tomb, err := h.app.Repositories.Workspace.GetHomeForProject(context.Background(), imported.projectID)
	require.NoError(t, err)
	h.crash()

	tomb.Status = domain.WorkspaceStatusDeleted
	seed := openBaseEra(t, home)
	seed.unreplayableWorkspace(tomb)
	seed.close()

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	got, err := h2.app.Repositories.Workspace.GetHomeForProject(context.Background(), imported.projectID)
	require.NoError(t, err)
	assert.Equal(t, domain.WorkspaceStatusDeleted, got.Status, "the tombstone still holds the home's id")
	assert.Len(t, migratetest.Applied(t, homeAt(t, home))[migrations.StepProjectHomes], 1)
}
