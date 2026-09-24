//go:build integration

package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The one-off boot upgrades of what the pre-audit daemon left behind.

// A chat minted before Type existed replays with "", which base read as a
// conversation; nothing reads it that way now, so boot writes it down.
func TestRegression_Upgrade_AnUntypedLegacyChatIsAChatAfterBoot(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	imported := importProject(t, h)
	h.Quiesce()
	h.crash()
	now := time.Now().UTC()
	legacy := domain.Chat{ID: uuid.NewString(), WorkspaceID: imported.workspaceID,
		Title: "old chat", CreatedAt: now, LastActivityAt: now}
	seed := openBaseEra(t, home)
	seed.chat(legacy)
	seed.close()

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	got, err := h2.app.Usecases.AgentChat.GetChat(context.Background(), legacy.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ChatTypeChat, got.Type)
	assert.True(t, got.IsChat())
}

// Base minted a project's home on the first GET of /home. A project it saved
// before homes existed, never opened since, has none; reads no longer write,
// so boot creates it under the deterministic id.
func TestRegression_Upgrade_AProjectWithoutAHomeGetsOneAtBoot(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	project := domain.Project{ID: uuid.NewString(), Name: "legacy", Path: t.TempDir(), LastActivity: time.Now()}
	require.NoError(t, h.app.GORM.Projects.Save(context.Background(), project))
	h.crash()
	openBaseEra(t, home).close()

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	var homeWS struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}
	h2.get("/v0/projects/"+project.ID+"/home", &homeWS)
	assert.Equal(t, workspace.ProjectHomeID(project.ID), homeWS.ID)
	assert.Equal(t, "home", homeWS.Kind)
}

// The exactly-once hook journal lived in <chatsDir>/.hook-deliveries; it is
// retired. Boot deletes exactly that directory from Crowbar-managed chats
// directories — and a directory of that name anywhere else is not its to take.
func TestRegression_Upgrade_BootDeletesTheRetiredHookJournal(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	imported := importProject(t, h)
	h.Quiesce()
	chatsDir, err := h.app.Usecases.AgentWorkspaceReader.AgentChatsDir(context.Background(), imported.workspaceID)
	require.NoError(t, err)
	journal := filepath.Join(chatsDir, ".hook-deliveries", "runner-1", "d.json")
	kept := []string{
		filepath.Join(chatsDir, "c1", "attachments", "a.png"),
		filepath.Join(chatsDir, "c1", ".hook-deliveries", "not-the-journal.json"),
		filepath.Join(imported.repoPath, ".hook-deliveries", "users-own.json"),
	}
	for _, f := range append([]string{journal}, kept...) {
		require.NoError(t, os.MkdirAll(filepath.Dir(f), 0o755))
		require.NoError(t, os.WriteFile(f, []byte("{}"), 0o644))
	}
	h.crash()
	// A fresh marker set: the harness boot above already ran every step.
	require.NoError(t, os.RemoveAll(filepath.Join(worktreepath.GlobalStateDir(home), "upgrades")))

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	assert.NoDirExists(t, filepath.Join(chatsDir, ".hook-deliveries"))
	for _, f := range kept {
		assert.FileExists(t, f)
	}
}

// view.db's workspace_paths table (the retired id→path index) is dropped.
func TestRegression_Upgrade_BootDropsTheRetiredWorkspacePathsTable(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	h.crash()
	require.NoError(t, os.RemoveAll(filepath.Join(worktreepath.GlobalStateDir(home), "upgrades")))
	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	require.NoError(t, adapters.GlobalView().Exec(
		"CREATE TABLE workspace_paths (id TEXT PRIMARY KEY, path TEXT)").Error)
	require.NoError(t, adapters.Close())

	h2 := newHarnessAt(t, home)
	h2.shutdown()

	after, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	defer func() { _ = after.Close() }()
	assert.False(t, after.GlobalView().Migrator().HasTable("workspace_paths"))
}
