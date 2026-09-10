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

// ChatTypeFolder and ChatTypeBranch are deliberately absent from this list
// (2026-09-08 sidebar-placement-unification: Task 8 narrowed the closed
// taxonomy a Chat aggregate may be minted or retyped as to drop
// ChatTypeFolder — a folder is a domain.Folder row now, home-scoped or
// repo-scoped alike, never a Chat row; Task 9 narrowed it further to drop
// ChatTypeBranch — a workspace's own position is a Node{Kind:workspace} row
// now, never a retyped Chat proxy) — see TestCreate_Validate_RejectsFolderType
// and TestCreate_Validate_RejectsBranchType below for the other half of each.
func TestCreate_Validate_AcceptsEachKnownType(t *testing.T) {
	for _, ct := range []domain.ChatType{
		domain.ChatTypeChat,
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

// TestCreate_Validate_RejectsBranchType is Task 9's own regression: nothing
// mints a ChatTypeBranch row any more — a workspace's own position is
// carried by its Node{Kind:workspace} row, never a Chat proxy retyped into
// standing for it.
func TestCreate_Validate_RejectsBranchType(t *testing.T) {
	c := commands.Create{ID: "chat-1", WorkspaceID: "ws-1", Type: domain.ChatTypeBranch}
	if err := c.Validate(nil); err == nil {
		t.Fatalf("expected error creating a ChatTypeBranch chat -- a workspace's position is a Node row now")
	}
}

func TestCreate_Validate_AllowsEmptyWorkspaceID(t *testing.T) {
	c := commands.Create{ID: "chat-1", Type: domain.ChatTypeChat}
	if err := c.Validate(nil); err != nil {
		t.Fatalf("a bubble chat must be creatable with no workspace: %v", err)
	}
}
