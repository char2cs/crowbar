//go:build integration

package tests

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	wsrepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Every workspace on disk today predates the atomic create-chat path and so
// owns NO chat row at all, which leaves the sidebar's placement machinery
// (resolveOwningChat) with nothing to find. The tests below pin the boot-time
// backfill that mints the missing rows — and, for a workspace whose own
// character has changed since, adopts the row it already has.

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
	seeded := writeUnbackfilledWorkspaces(t, h)
	h.shutdown()
	return seeded
}

// writeUnbackfilledWorkspaces plants the fork chain into an ALREADY BOOTED
// daemon, so a caller that also has to plant pre-backfill chats can do both in
// the same daemon's life. A second boot would not do: it runs the backfill on
// the way up, and the rows would be owned before the plant landed.
func writeUnbackfilledWorkspaces(
	t *testing.T,
	h *harness,
) unbackfilledWorkspaces {
	t.Helper()
	ctx := context.Background()
	born := time.Now().UTC()

	seeded := unbackfilledWorkspaces{home: "ws-home", locked: "ws-locked", child: "ws-child"}
	rows := []wsrepo.CreateInput{
		{ID: seeded.home, RepoID: "repo-1", ProjectID: "prj-1", Branch: "main", IsDefault: true},
		{ID: seeded.locked, RepoID: "repo-1", ProjectID: "prj-1", Branch: "release", ParentID: seeded.home, Protected: true},
		{ID: seeded.child, RepoID: "repo-1", ProjectID: "prj-1", Branch: "feature", ParentID: seeded.locked},
	}
	for i, in := range rows {
		// Every managed git workspace has a worktree, and the lock command
		// refuses one that does not — so the seed carries a path even though no
		// test here touches the disk behind it.
		in.WorktreePath = filepath.Join(h.home, "seeded", in.ID, "worktree")
		_, err := h.app.Repositories.Workspace.Create(ctx, in, born.Add(time.Duration(i)*time.Minute))
		require.NoError(t, err)
	}
	h.Quiesce()

	for _, id := range []string{seeded.home, seeded.locked, seeded.child} {
		chats, err := h.app.Usecases.AgentChat.ListChatsByWorkspace(ctx, id)
		require.NoError(t, err)
		require.Empty(t, chats, "precondition: %s must own no chat before the backfill boot", id)
	}
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

// TestRegression_StartupMintsABranchRowALegacyChatDoesNotStandInFor is the case
// a real install is full of and a fresh one never produces: a locked branch that
// has been CHATTED IN before this backfill existed.
//
// An ordinary conversation started inside a workspace carries that workspace's
// id, exactly as an owning row does, so "this workspace has a chat" cannot be
// the signal for a branch-destined workspace — none of those conversations is
// drawn as a branch row, and until Task 2 none was even an acceptable parent.
// The branch row is owed regardless, and the child forked off the branch has to
// hang off THAT row rather than off the older conversation.
func TestRegression_StartupMintsABranchRowALegacyChatDoesNotStandInFor(t *testing.T) {
	home := t.TempDir()
	first := newHarnessAt(t, home)
	seeded := writeUnbackfilledWorkspaces(t, first)
	plantLegacyChat(t, first, seeded.locked)
	first.shutdown()

	h := newHarnessAt(t, home)
	h.Quiesce()

	rows, err := h.app.Usecases.AgentChat.ListChatsByWorkspace(context.Background(), seeded.locked)
	require.NoError(t, err)
	require.Len(t, rows, 2, "the legacy conversation is kept and the branch row joins it")

	branches := make([]domain.Chat, 0, 1)
	for _, row := range rows {
		if row.Type == domain.ChatTypeBranch {
			branches = append(branches, row)
		}
	}
	require.Len(t, branches, 1, "the locked workspace is owed exactly one branch row")
	assert.NotEqual(t, legacyChatID, branches[0].ID)

	assert.Equal(t, branches[0].ID, owningChat(t, h, seeded.child).ParentID,
		"the child hangs off the branch row, not off the conversation that predates it")
}

// legacyChatID names the pre-backfill conversation the test above plants, so the
// assertion can say which row it must NOT have been mistaken for.
const legacyChatID = "legacy-conversation"

// plantLegacyChat writes an ordinary conversation against wsID — the row any
// workspace that has actually been used carries, in the same shape the daemon's
// own spawn path writes it (MintChat stamps the workspace id onto every chat
// started inside one) — and SPEAKS in it.
//
// The turn is not decoration. It is the only thing separating a conversation
// from a structural owning row, which are otherwise the same shape, and it is
// what the backfill refuses to adopt: a branch row does not open as a
// conversation, so a row with words in it gets a branch row minted beside it
// rather than being turned into one.
func plantLegacyChat(
	t *testing.T,
	h *harness,
	wsID string,
) {
	t.Helper()
	ctx := context.Background()
	_, err := h.app.Repositories.AgentChat.Create(ctx, agentchat.CreateInput{
		ID:          legacyChatID,
		WorkspaceID: wsID,
		Type:        domain.ChatTypeChat,
		Now:         time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, h.app.Repositories.AgentActivity.AppendTurn(ctx, activity.TurnInput{
		ChatID: legacyChatID, TurnID: "turn-1", Role: "user", ProviderID: "claude",
		RunnerID: "r1", SessionID: "s1", Text: "something the user said", Now: time.Now().UTC(),
	}))
	h.Quiesce()
}

// TestRegression_StartupAdoptsTheOwningRowOfAWorkspaceThatBecameLocked is the
// three-boot sequence a running install actually goes through: a worktree is
// backfilled as an ordinary chat row, the branch is later locked (the user
// locks it, or a provider poll reports it protected), and the daemon restarts
// again.
//
// The workspace is branch-destined from the lock onwards, but it is not
// UNOWNED — it has the row the first boot gave it. That row must become the
// branch row, keeping its id and its placement, because minting a second one
// would leave two rows claiming a single workspace, which is the invariant this
// whole backfill exists to establish.
func TestRegression_StartupAdoptsTheOwningRowOfAWorkspaceThatBecameLocked(t *testing.T) {
	home := t.TempDir()
	seeded := seedUnbackfilledWorkspaces(t, home)

	first := newHarnessAt(t, home)
	first.Quiesce()
	before := owningChat(t, first, seeded.child)
	require.Equal(t, domain.ChatTypeChat, before.Type, "precondition: an unlocked worktree owns a chat row")
	lockWorkspace(t, first, seeded.child)
	first.shutdown()

	second := newHarnessAt(t, home)
	second.Quiesce()

	after := owningChat(t, second, seeded.child)
	assert.Equal(t, before.ID, after.ID, "the SAME row, adopted — not a second one minted beside it")
	assert.Equal(t, domain.ChatTypeBranch, after.Type, "and it is a branch row now that the workspace is locked")
	assert.Equal(t, before.ParentID, after.ParentID, "the retype keeps the placement the row already had")
	assert.Equal(t, before.Order, after.Order)
}

// lockWorkspace flips a workspace to locked through the workspace aggregate's
// own lock command — the same write the user's lock toggle makes.
func lockWorkspace(
	t *testing.T,
	h *harness,
	wsID string,
) {
	t.Helper()
	locked := true
	_, err := h.app.Repositories.Workspace.SetLock(context.Background(), wsID, &locked, false)
	require.NoError(t, err)
	h.Quiesce()
}

// TestRegression_LockingAWorkspaceGivesItABranchRowWithoutARestart closes the
// gap the boot pass alone leaves open.
//
// A workspace's character can change while the daemon is RUNNING: the user
// locks a branch, or a provider poll reports one protected. From that instant it
// is branch-destined, and everything reading it — the sidebar above all, which
// resolves a locked row's identity from its owning chat and has no fallback by
// design — is entitled to find a branch row there. Reconciling only at startup
// would leave that true as of the LAST boot and false until the next one.
//
// Driven through the real HTTP lock route, not the aggregate, because the
// guarantee is about what a client sees after the call it actually makes.
func TestRegression_LockingAWorkspaceGivesItABranchRowWithoutARestart(t *testing.T) {
	home := t.TempDir()
	first := newHarnessAt(t, home)
	imported := importWritableWorkspace(t, first)
	first.shutdown()

	// The boot pass gives the imported open worktree the ordinary chat row an
	// unlocked workspace owns. That is the row the lock has to find and change.
	h := newHarnessAt(t, home)
	h.Quiesce()
	before := owningChat(t, h, imported.workspaceID)
	require.Equal(t, domain.ChatTypeChat, before.Type, "precondition: an unlocked worktree owns a chat row")

	locked := true
	resp := h.raw(http.MethodPost, wsBase(imported)+"/lock",
		map[string]*bool{"locked": &locked}, http.StatusNoContent)
	require.NoError(t, resp.Body.Close())

	// No restart, and deliberately no Quiesce: the retype is on the SendWait
	// path, so the row is right ON THE READ MODEL by the time the client holds
	// its 204. A barrier here would hide exactly what this test is for — under
	// load, an async retype loses that race and the client re-reads the row it
	// locked and finds it unchanged.
	after := owningChat(t, h, imported.workspaceID)
	assert.Equal(t, before.ID, after.ID, "the row it already had, adopted in place")
	assert.Equal(t, domain.ChatTypeBranch, after.Type, "and it is a branch row the moment the lock lands")
}

// TestRegression_LockingInsideARepoImportedSinceBootParentsTheRowForReal is the
// runtime trigger's own edge, and the damage it would do is permanent.
//
// A repo imported while the daemon is running brings in a whole chain no boot
// pass has seen. Locking a branch inside it plans a row whose parent is a row
// the SAME plan mints for its fork parent — so a reconcile that wrote only the
// locked workspace's own row would file it under an id nothing answers to.
// Nothing would ever notice: SetPlacement validates no parent, and the workspace
// then reads as owned on every later boot, so no backfill repairs it.
func TestRegression_LockingInsideARepoImportedSinceBootParentsTheRowForReal(t *testing.T) {
	h := newHarnessAt(t, t.TempDir())
	ctx := context.Background()

	// Written while the daemon RUNS, so the boot pass never saw either of them —
	// what an import lands mid-session.
	born := time.Now().UTC()
	for i, in := range []wsrepo.CreateInput{
		{ID: "ws-home", RepoID: "repo-1", ProjectID: "prj-1", Branch: "main", IsDefault: true},
		{ID: "ws-child", RepoID: "repo-1", ProjectID: "prj-1", Branch: "release", ParentID: "ws-home"},
	} {
		in.WorktreePath = filepath.Join(h.home, "imported", in.ID, "worktree")
		_, err := h.app.Repositories.Workspace.Create(ctx, in, born.Add(time.Duration(i)*time.Minute))
		require.NoError(t, err)
	}
	h.Quiesce()
	require.Empty(t, ownedChats(t, h, "ws-home"), "precondition: the parent owns no row yet")

	locked := true
	_, err := h.app.Usecases.Workspace.SetLock(ctx, "ws-child", &locked)
	require.NoError(t, err)
	h.Quiesce()

	child := owningChat(t, h, "ws-child")
	assert.Equal(t, domain.ChatTypeBranch, child.Type)
	require.NotEmpty(t, child.ParentID, "the locked branch hangs off its repo home, not the panel root")
	assert.Equal(t, owningChat(t, h, "ws-home").ID, child.ParentID,
		"and off a row that EXISTS — a parent id naming nothing is written happily and never repaired")
}

// ownedChats is every chat row carrying wsID, for the preconditions that need to
// see none rather than exactly one.
func ownedChats(
	t *testing.T,
	h *harness,
	wsID string,
) []domain.Chat {
	t.Helper()
	chats, err := h.app.Usecases.AgentChat.ListChatsByWorkspace(context.Background(), wsID)
	require.NoError(t, err)
	return chats
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
