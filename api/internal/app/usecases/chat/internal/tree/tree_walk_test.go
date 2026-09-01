package tree_test

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	apptree "github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// stubListChats answers tree.Chats.ListChats and LoadChat from a fixed row
// set — the two methods ResolveForkParent's freshForest calls (walk.go), so
// nothing else on the interface needs a body.
//
// LoadChat and ListChats are DELIBERATELY separate answers rather than one
// derived from the other: freshForest's whole point is that they can disagree
// (loaded is the log-true row, rows is what the list projection still
// carries), and TestResolveForkParent_UsesTheLogFoldedRowNotTheStaleProjection
// exercises exactly that gap.
type stubListChats struct {
	tree.Chats
	rows    []domain.Chat
	err     error
	loadErr error
	// loaded overrides LoadChat's answer for the ids it names, standing in for
	// the log fold when it must read AHEAD of rows. An id not named here falls
	// back to searching rows, which is enough for every test that doesn't care
	// about the staleness gap.
	loaded map[string]domain.Chat
}

func (s stubListChats) ListChats(context.Context) ([]domain.Chat, error) {
	return s.rows, s.err
}

func (s stubListChats) LoadChat(_ context.Context, id string) (domain.Chat, error) {
	if s.loadErr != nil {
		return domain.Chat{}, s.loadErr
	}
	if c, ok := s.loaded[id]; ok {
		return c, nil
	}
	for _, row := range s.rows {
		if row.ID == id {
			return row, nil
		}
	}
	return domain.Chat{}, apperr.ErrNotFound
}

func treeFrom(
	chats map[string]domain.Chat,
) apptree.Tree {
	nodes := make([]apptree.Node, 0, len(chats))
	for _, c := range chats {
		nodes = append(nodes, apptree.Node{
			ID:        c.ID,
			ParentID:  c.ParentID,
			Order:     c.Order,
			CreatedAt: c.CreatedAt,
		})
	}
	return apptree.New(nodes)
}

func TestCwdWorkspaceID_SkipsUnprovisionedAncestor(t *testing.T) {
	chats := map[string]domain.Chat{
		"root":    {ID: "root", Type: domain.ChatTypeBranch, WorkspaceID: "ws-root"},
		"folder":  {ID: "folder", Type: domain.ChatTypeFolder, ParentID: "root"},
		"blocked": {ID: "blocked", Type: domain.ChatTypeBranch, ParentID: "folder", WorkspaceID: ""},
		"leaf":    {ID: "leaf", Type: domain.ChatTypeChat, ParentID: "blocked"},
	}
	tr := treeFrom(chats)

	got, ok := tree.CwdWorkspaceID(tr, chats, "leaf")

	if !ok || got != "ws-root" {
		t.Fatalf("want ws-root (walk past the unprovisioned blocked row), got %q ok=%v", got, ok)
	}
}

func TestForkParentID_ExcludesSelf(t *testing.T) {
	chats := map[string]domain.Chat{
		"root": {ID: "root", Type: domain.ChatTypeBranch, WorkspaceID: "ws-root"},
		"self": {ID: "self", Type: domain.ChatTypeBranch, ParentID: "root", WorkspaceID: "ws-self"},
	}
	tr := treeFrom(chats)

	got, ok := tree.ForkParentID(tr, chats, "self")

	if !ok || got != "ws-root" {
		t.Fatalf("fork parent must exclude self's own workspace, want ws-root, got %q ok=%v", got, ok)
	}
}

func TestResolveForkParent_ReadsEveryRowAndWalksLikeForkParentID(t *testing.T) {
	rows := []domain.Chat{
		{ID: "root", Type: domain.ChatTypeBranch, WorkspaceID: "ws-root"},
		{ID: "self", Type: domain.ChatTypeBranch, ParentID: "root", WorkspaceID: "ws-self"},
	}
	chats := stubListChats{rows: rows}

	got, ok, err := tree.ResolveForkParent(context.Background(), chats, "self")

	if err != nil || !ok || got != "ws-root" {
		t.Fatalf("want (ws-root, true, nil), got (%q, %v, %v)", got, ok, err)
	}
}

func TestResolveForkParent_NoAncestorAtAll_ReportsNotFound(t *testing.T) {
	rows := []domain.Chat{
		{ID: "root", Type: domain.ChatTypeChat},
	}
	chats := stubListChats{rows: rows}

	_, ok, err := tree.ResolveForkParent(context.Background(), chats, "root")

	if err != nil || ok {
		t.Fatalf("a row with no ancestor has no fork parent, got ok=%v err=%v", ok, err)
	}
}

func TestResolveForkParent_PropagatesTheListError(t *testing.T) {
	wantErr := context.Canceled
	// "any" must resolve via LoadChat before ListChats ever runs (freshForest,
	// walk.go), so it needs a row to be found in — otherwise LoadChat's own
	// not-found would be the error this test observes, not ListChats'.
	chats := stubListChats{rows: []domain.Chat{{ID: "any"}}, err: wantErr}

	_, _, err := tree.ResolveForkParent(context.Background(), chats, "any")

	if err != wantErr {
		t.Fatalf("want the list error propagated, got %v", err)
	}
}

// TestResolveForkParent_PropagatesTheLoadFailure is TestResolveForkParent_PropagatesTheListError's
// counterpart for freshForest's OTHER read: LoadChat runs first, so its own
// failure must surface without ever reaching ListChats.
func TestResolveForkParent_PropagatesTheLoadFailure(t *testing.T) {
	wantErr := context.Canceled
	chats := stubListChats{loadErr: wantErr, err: errors.New("must never be reached")}

	_, _, err := tree.ResolveForkParent(context.Background(), chats, "any")

	if err != wantErr {
		t.Fatalf("want the load error propagated, got %v", err)
	}
}

// TestResolveForkParent_UsesTheLogFoldedRowNotTheStaleProjection is the
// direct unit-level pin for the race freshForest (walk.go) exists to close:
// "self" was minted AND placed microseconds before this call, in the SAME
// request — CreateChat's ownWorktree path (own_worktree.go) mints, places,
// then immediately resolves the new row's fork parent — so the list
// projection (rows) can still be serving "self" exactly as it stood BEFORE
// that placement (no parent at all) while the log fold (loaded) already has
// the truth. Before this fix, ResolveForkParent read straight off rows and
// would report no fork parent at all here — the production bug the
// integration suite (api/tests/regression_create_own_worktree_test.go) caught.
func TestResolveForkParent_UsesTheLogFoldedRowNotTheStaleProjection(t *testing.T) {
	chats := stubListChats{
		rows: []domain.Chat{
			{ID: "root", Type: domain.ChatTypeBranch, WorkspaceID: "ws-root"},
			{ID: "self", Type: domain.ChatTypeBranch}, // stale: the pre-placement row, no parent
		},
		loaded: map[string]domain.Chat{
			"self": {ID: "self", Type: domain.ChatTypeBranch, ParentID: "root"},
		},
	}

	got, ok, err := tree.ResolveForkParent(context.Background(), chats, "self")

	if err != nil || !ok || got != "ws-root" {
		t.Fatalf("must resolve through the log-folded parent, not the stale projection; got (%q, %v, %v)", got, ok, err)
	}
}

func TestChatLineage_StopsAtNonChatRow(t *testing.T) {
	chats := map[string]domain.Chat{
		"folder": {ID: "folder", Type: domain.ChatTypeFolder},
		"parent": {ID: "parent", Type: domain.ChatTypeChat, ParentID: "folder"},
		"child":  {ID: "child", Type: domain.ChatTypeChat, ParentID: "parent"},
	}
	tr := treeFrom(chats)

	got := tree.ChatLineage(tr, chats, "child")

	if len(got) != 1 || got[0] != "parent" {
		t.Fatalf("lineage must only include Type==chat ancestors, got %v", got)
	}
}
