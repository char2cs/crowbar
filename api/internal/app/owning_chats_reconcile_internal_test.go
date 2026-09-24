package app

import (
	"context"
	"errors"
	"testing"

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
			{ID: "thread", WorkspaceID: "orphan"}, // a thread, not an owner
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
