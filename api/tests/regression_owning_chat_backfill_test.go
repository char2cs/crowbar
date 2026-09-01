//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wsrepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Every workspace on disk today predates the atomic create-chat path and so
// owns NO chat row at all, which leaves the sidebar's placement machinery
// (resolveOwningChat) with nothing to find. These two tests pin the boot-time
// backfill that mints the missing rows.

// unbackfilledWorkspaces is the three-row fork chain the backfill has to
// reconstruct: a repo home, a locked branch forked off it, and an ordinary
// workspace forked off that. They are written straight to the workspace
// aggregate rather than through the import flow because the shape under test is
// the LINEAGE, not the git provisioning — and because seeding it by hand is the
// only way to state each row's IsDefault/Status/ParentID exactly.
type unbackfilledWorkspaces struct {
	home   string
	locked string
	child  string
}

// seedUnbackfilledWorkspaces boots a daemon over home, writes the three
// workspaces into it with no chats anywhere, and shuts it down again. The rows
// therefore exist BEFORE the boot under test, which is the only way to observe
// a backfill at all: the backfill runs inside app.New, so anything created
// during a daemon's lifetime is by definition created after its own.
func seedUnbackfilledWorkspaces(
	t *testing.T,
	home string,
) unbackfilledWorkspaces {
	t.Helper()
	h := newHarnessAt(t, home)
	ctx := context.Background()
	born := time.Now().UTC()

	seeded := unbackfilledWorkspaces{home: "ws-home", locked: "ws-locked", child: "ws-child"}
	rows := []wsrepo.CreateInput{
		{ID: seeded.home, RepoID: "repo-1", ProjectID: "prj-1", Branch: "main", IsDefault: true},
		{ID: seeded.locked, RepoID: "repo-1", ProjectID: "prj-1", Branch: "release", ParentID: seeded.home, Protected: true},
		{ID: seeded.child, RepoID: "repo-1", ProjectID: "prj-1", Branch: "feature", ParentID: seeded.locked},
	}
	for i, in := range rows {
		_, err := h.app.Repositories.Workspace.Create(ctx, in, born.Add(time.Duration(i)*time.Minute))
		require.NoError(t, err)
	}
	h.Quiesce()

	for _, id := range []string{seeded.home, seeded.locked, seeded.child} {
		chats, err := h.app.Usecases.AgentChat.ListChatsByWorkspace(ctx, id)
		require.NoError(t, err)
		require.Empty(t, chats, "precondition: %s must own no chat before the backfill boot", id)
	}
	h.shutdown()
	return seeded
}

// owningChat returns the single chat row wsID owns, failing when it owns none
// or more than one — "exactly one" is the invariant, so both directions are a
// test failure rather than something to pick a winner from.
func owningChat(
	t *testing.T,
	h *harness,
	wsID string,
) domain.Chat {
	t.Helper()
	chats, err := h.app.Usecases.AgentChat.ListChatsByWorkspace(context.Background(), wsID)
	require.NoError(t, err)
	require.Len(t, chats, 1, "workspace %s must own exactly one chat", wsID)
	return chats[0]
}

// TestRegression_StartupMintsAnOwningChatForEveryPreExistingWorkspace is the
// backfill's core contract: after the daemon starts, every workspace owns one
// chat row, typed by what the workspace IS (a locked branch and a repo home are
// branch rows, an ordinary worktree is a chat row) and parented on its FORK
// PARENT's own owning chat — which only holds if the backfill walked the fork
// chain root-first, since a child's parent row has to exist before the child
// can name it.
func TestRegression_StartupMintsAnOwningChatForEveryPreExistingWorkspace(t *testing.T) {
	home := t.TempDir()
	seeded := seedUnbackfilledWorkspaces(t, home)

	h := newHarnessAt(t, home)
	h.Quiesce()

	homeChat := owningChat(t, h, seeded.home)
	assert.Equal(t, domain.ChatTypeBranch, homeChat.Type, "the repo home is a branch row")
	assert.Equal(t, "", homeChat.ParentID, "a fork-parentless workspace's row sits at the panel root")

	lockedChat := owningChat(t, h, seeded.locked)
	assert.Equal(t, domain.ChatTypeBranch, lockedChat.Type, "a locked workspace is a branch row")
	assert.Equal(t, homeChat.ID, lockedChat.ParentID,
		"the locked row hangs off its fork parent's OWN backfilled row, not off the workspace id")

	childChat := owningChat(t, h, seeded.child)
	assert.Equal(t, domain.ChatTypeChat, childChat.Type, "an unlocked worktree is an ordinary chat row")
	assert.Equal(t, lockedChat.ID, childChat.ParentID,
		"a grandchild resolves through the row minted for its parent in the same pass")
}

// TestRegression_StartupBackfillCoversAWholeImportedRepo runs the backfill over
// the shape a real install actually has, not a hand-written fork chain: a
// project home with no repo at all, a repo home adopted from the user's own
// checkout, the protected branch's managed worktree, and an unlocked child cut
// off it. Every one of the four is left with no chat by the import, and every
// one has to come back typed for what it IS.
//
// It asserts no PARENTING, and the reason is worth recording: an import records
// no fork parent on any of these rows — Workspace.ParentID is resolved from the
// chat forest's own fork-parent walk, which on a repo with no chats resolves
// nothing — so all four are genuine roots and a lineage assertion here would
// pass over an empty set. The lineage the backfill reconstructs is pinned on
// the seeded chain above instead, where the parentage actually exists.
func TestRegression_StartupBackfillCoversAWholeImportedRepo(t *testing.T) {
	home := t.TempDir()
	first := newHarnessAt(t, home)
	importWritableWorkspace(t, first)
	first.shutdown()

	second := newHarnessAt(t, home)
	second.Quiesce()

	workspaces, err := second.app.Repositories.Workspace.List(context.Background())
	require.NoError(t, err)

	typed := map[string]domain.ChatType{}
	for _, ws := range workspaces {
		if ws.Status == domain.WorkspaceStatusDeleted {
			continue
		}
		// owningChat carries the "exactly one row" half of the assertion for
		// every live workspace, whatever it turns out to be.
		typed[describeWorkspace(ws)] = owningChat(t, second, ws.ID).Type
	}

	assert.Equal(t, map[string]domain.ChatType{
		"project-home":    domain.ChatTypeBranch,
		"repo-home":       domain.ChatTypeBranch,
		"locked-worktree": domain.ChatTypeBranch,
		"open-worktree":   domain.ChatTypeChat,
	}, typed, "each of the import's four workspaces owns a row typed for what that workspace is")
}

// describeWorkspace names an imported workspace by its role, so the assertion
// above reads as the four rows an import leaves rather than as four uuids.
func describeWorkspace(
	ws domain.Workspace,
) string {
	switch {
	case ws.Kind == domain.WorkspaceKindHome:
		return "project-home"
	case ws.IsDefault:
		return "repo-home"
	case ws.Status == domain.WorkspaceStatusLocked:
		return "locked-worktree"
	default:
		return "open-worktree"
	}
}

// TestRegression_StartupBackfillDoesNotDoubleMintOnASecondBoot is the other half:
// the backfill runs on EVERY boot, so a second one over already-backfilled rows
// must mint nothing. Without the per-workspace gate each restart would add
// another owning row, and resolveOwningChat would start picking between them.
func TestRegression_StartupBackfillDoesNotDoubleMintOnASecondBoot(t *testing.T) {
	home := t.TempDir()
	seeded := seedUnbackfilledWorkspaces(t, home)

	first := newHarnessAt(t, home)
	first.Quiesce()
	minted := map[string]string{
		seeded.home:   owningChat(t, first, seeded.home).ID,
		seeded.locked: owningChat(t, first, seeded.locked).ID,
		seeded.child:  owningChat(t, first, seeded.child).ID,
	}
	first.shutdown()

	second := newHarnessAt(t, home)
	second.Quiesce()

	for wsID, chatID := range minted {
		// owningChat itself asserts the "exactly one" half; this pins the stronger
		// fact that it is the SAME row, not a replacement.
		assert.Equal(t, chatID, owningChat(t, second, wsID).ID,
			"the second boot must leave %s's existing owning chat alone", wsID)
	}
}
