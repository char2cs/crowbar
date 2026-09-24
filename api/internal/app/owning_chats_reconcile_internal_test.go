package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type fakeMinter struct {
	minted    []string // parent workspace ids
	attached  map[string]string
	discarded []string
	attachErr error
}

func (f *fakeMinter) MintOwningChat(_ context.Context, parent string) (string, error) {
	f.minted = append(f.minted, parent)
	return "chat-" + parent, nil
}

func (f *fakeMinter) AttachOwningWorkspace(_ context.Context, chatID string, ws domain.Workspace) error {
	if f.attachErr != nil {
		return f.attachErr
	}
	if f.attached == nil {
		f.attached = map[string]string{}
	}
	f.attached[ws.ID] = chatID
	return nil
}

func (f *fakeMinter) DiscardOwningChat(_ context.Context, chatID string) error {
	f.discarded = append(f.discarded, chatID)
	return nil
}

// A crash between writing a workspace and attaching its minted chat leaves the
// workspace owned by nothing. Boot gives exactly those an owner (D4) — never a
// workspace that already has one, and never a tombstone.
func TestReconcileOwningChats_OwnsEveryLiveUnownedWorkspace(t *testing.T) {
	m := &fakeMinter{}
	reconcileOwningChats(context.Background(),
		[]domain.Workspace{
			{ID: "owned", ParentID: "p"},
			{ID: "orphan", ParentID: "parent-ws"},
			{ID: "gone", Status: domain.WorkspaceStatusDeleted},
		},
		[]domain.Chat{
			{ID: "c1", WorkspaceID: "owned", OwnsWorkspace: true},
			{ID: "thread", WorkspaceID: "orphan", ParentID: "orphan"}, // a thread, not an owner
		},
		m)

	assert.Equal(t, []string{"parent-ws"}, m.minted, "the new owner is placed under its git parent")
	assert.Equal(t, map[string]string{"orphan": "chat-parent-ws"}, m.attached)
}

// A failed attach takes the minted chat back out rather than leaving it
// pointing at nothing.
func TestReconcileOwningChats_FailedAttachDiscardsTheChat(t *testing.T) {
	m := &fakeMinter{attachErr: errors.New("boom")}
	reconcileOwningChats(context.Background(),
		[]domain.Workspace{{ID: "orphan"}}, nil, m)
	assert.Equal(t, []string{"chat-"}, m.discarded)
}

// The pre-audit daemon recorded an owner only when a read resolved one, by
// heuristic. A workspace it never listed still has that owner, unrecorded: boot
// records the same row base would have picked instead of minting an empty one.
func TestReconcileOwningChats_RecordsTheOwnerThePreAuditHeuristicPicks(t *testing.T) {
	t0 := time.Unix(1000, 0)
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	fork := domain.Workspace{ID: "fork", RepoID: "r", ParentID: "main"}
	locked := domain.Workspace{ID: "locked", RepoID: "r", Status: domain.WorkspaceStatusLocked}
	def := domain.Workspace{ID: "def", RepoID: "r", IsDefault: true}
	empty := domain.Workspace{ID: "empty", RepoID: "r", ParentID: "main"}
	chats := []domain.Chat{
		// fork: the thread filed under its forker never owns; the forker,
		// titled and chatted in, is the earliest remaining row.
		{ID: "forker", WorkspaceID: "fork", Title: "Fix login", Type: domain.ChatTypeChat, CreatedAt: at(1)},
		{ID: "thread", WorkspaceID: "fork", ParentID: "forker", Type: domain.ChatTypeChat, CreatedAt: at(0)},
		{ID: "anchored", WorkspaceID: "fork", ParentID: "fork", Type: domain.ChatTypeChat, CreatedAt: at(0)},
		{ID: "later", WorkspaceID: "fork", Type: domain.ChatTypeChat, CreatedAt: at(2)},
		// locked: a legacy branch row beats an earlier conversation.
		{ID: "conv", WorkspaceID: "locked", Type: domain.ChatTypeChat, CreatedAt: at(0)},
		{ID: "branch-row", WorkspaceID: "locked", Type: domain.ChatTypeBranch, CreatedAt: at(5)},
		// default checkout: shared ground, where a titled row is a user's chat.
		{ID: "titled", WorkspaceID: "def", Title: "Ideas", Type: domain.ChatTypeChat, CreatedAt: at(0)},
		{ID: "untitled", WorkspaceID: "def", Type: "", CreatedAt: at(3)},
	}
	m := &fakeMinter{}

	reconcileOwningChats(context.Background(), []domain.Workspace{fork, locked, def, empty}, chats, m)

	assert.Equal(t, map[string]string{
		"fork":   "forker",
		"locked": "branch-row",
		"def":    "untitled",
		"empty":  "chat-main",
	}, m.attached)
	assert.Equal(t, []string{"main"}, m.minted, "only a workspace with no candidate gets a fresh chat")
}

// On shared ground with only titled conversations there is no owner to
// record, so one is minted — never a user's conversation taken over.
func TestReconcileOwningChats_SharedGroundNeverTakesATitledConversation(t *testing.T) {
	m := &fakeMinter{}
	reconcileOwningChats(context.Background(),
		[]domain.Workspace{{ID: "def", RepoID: "r", IsDefault: true, ParentID: "p"}},
		[]domain.Chat{{ID: "titled", WorkspaceID: "def", Title: "Ideas", Type: domain.ChatTypeChat}},
		m)
	assert.Equal(t, map[string]string{"def": "chat-p"}, m.attached)
}
