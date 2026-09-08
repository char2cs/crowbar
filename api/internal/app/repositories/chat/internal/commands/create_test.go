package commands_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestCreate_Validate_RejectsUnknownType(t *testing.T) {
	c := commands.Create{ID: "chat-1", WorkspaceID: "ws-1", Type: domain.ChatType("bogus")}
	if err := c.Validate(nil); err == nil {
		t.Fatalf("expected error for unknown chat type")
	}
}

// ChatTypeFolder is deliberately absent from this list (2026-09-08
// sidebar-placement-unification Task 8 narrows the closed taxonomy a Chat
// aggregate may be minted or retyped as — a folder is a domain.Folder row
// now, home-scoped or repo-scoped alike, never a Chat row) — see
// TestCreate_Validate_RejectsFolderType below for the other half of the
// same change. ChatTypeBranch stays valid here until Task 9 retires it
// alongside owning_rows.go.
func TestCreate_Validate_AcceptsEachKnownType(t *testing.T) {
	for _, ct := range []domain.ChatType{
		domain.ChatTypeChat,
		domain.ChatTypeBranch,
		domain.ChatTypeWorkflow,
	} {
		c := commands.Create{ID: "chat-1", WorkspaceID: "ws-1", Type: ct}
		if err := c.Validate(nil); err != nil {
			t.Fatalf("type %s: unexpected error: %v", ct, err)
		}
	}
}

// TestCreate_Validate_RejectsFolderType is Task 8's own regression: a folder
// can no longer be minted as a Chat aggregate at all.
func TestCreate_Validate_RejectsFolderType(t *testing.T) {
	c := commands.Create{ID: "chat-1", WorkspaceID: "ws-1", Type: domain.ChatTypeFolder}
	if err := c.Validate(nil); err == nil {
		t.Fatalf("expected error creating a ChatTypeFolder chat -- a folder is a domain.Folder row now")
	}
}

func TestCreate_Validate_AllowsEmptyWorkspaceID(t *testing.T) {
	c := commands.Create{ID: "chat-1", Type: domain.ChatTypeChat}
	if err := c.Validate(nil); err != nil {
		t.Fatalf("a bubble chat must be creatable with no workspace: %v", err)
	}
}
