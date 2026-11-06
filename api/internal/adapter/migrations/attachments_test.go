package migrations

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

const (
	managedChats = "projects/p1/github.com/acme/app/main/chats"
	homeChats    = "projects/p1/acme-app/default/chats"
)

func TestChatAttachments_MovesEachChatsFilesUnderItsOwnDirectory(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, managedChats+"/chat-a/attachments/a1-shot.png", "a1")
	testutil.WriteFile(t, h, homeChats+"/chat-b/attachments/b1-doc.pdf", "b1")
	testutil.WriteFile(t, h, "projects/p1/github.com/acme/app/main/worktree/chats/x/attachments/user.txt", "user")

	finish(t, boot(t, h))

	assert.Equal(t, "a1", readHome(t, h.Root(), "chats/chat-a/attachments/a1-shot.png"))
	assert.Equal(t, "b1", readHome(t, h.Root(), "chats/chat-b/attachments/b1-doc.pdf"))
	assert.NoDirExists(t, filepath.Join(h.Root(), managedChats, "chat-a"), "the emptied chat directory goes")
	assert.FileExists(t, filepath.Join(h.Root(), "projects/p1/github.com/acme/app/main/worktree/chats/x/attachments/user.txt"),
		"a user's checkout is not Crowbar's to move")
	assert.Equal(t, []migrate.Counts{{"moved_files": 2}}, migratetest.Applied(t, h)[StepChatAttachments])
}

func TestChatAttachments_KeepsAFileWhoseNameIsTakenAndSaysSo(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, managedChats+"/chat-a/attachments/a1-shot.png", "old")
	testutil.WriteFile(t, h, managedChats+"/chat-a/attachments/a2-shot.png", "moved")
	testutil.WriteFile(t, h, "chats/chat-a/attachments/a1-shot.png", "new")

	finish(t, boot(t, h))

	assert.Equal(t, "new", readHome(t, h.Root(), "chats/chat-a/attachments/a1-shot.png"))
	assert.Equal(t, "moved", readHome(t, h.Root(), "chats/chat-a/attachments/a2-shot.png"))
	assert.Equal(t, "old", readHome(t, h.Root(), managedChats+"/chat-a/attachments/a1-shot.png"), "never overwritten")
	note := migratetest.Notes(t, h)[StepChatAttachments][managedChats+"/chat-a/attachments"]
	assert.Contains(t, note.Detail, "a1-shot.png")
}

func TestChatAttachments_PlansNothingWithoutAttachments(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, managedChats+"/runners/r1-claude/settings.json", "{}")

	plan, err := planChatAttachments(context.Background(), migrate.Env{Home: h})

	require.NoError(t, err)
	assert.True(t, plan.Empty())
	assert.False(t, plan.RewritesState, "attachments live outside state/")
}

func readHome(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel)) //nolint:gosec // G304: a path under the test's own temp home
	require.NoError(t, err)
	return string(b)
}
