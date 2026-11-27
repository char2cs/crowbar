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

func TestCreate_Validate_RejectsFolderType(t *testing.T) {
	c := commands.Create{ID: "chat-1", WorkspaceID: "ws-1", Type: domain.ChatTypeFolder}
	if err := c.Validate(nil); err == nil {
		t.Fatalf("expected error creating a ChatTypeFolder chat -- a folder is a tree.Folder row now")
	}
}

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
