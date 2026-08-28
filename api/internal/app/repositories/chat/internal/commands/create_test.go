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
		domain.ChatTypeBranch,
		domain.ChatTypeFolder,
		domain.ChatTypeWorkflow,
	} {
		c := commands.Create{ID: "chat-1", WorkspaceID: "ws-1", Type: ct}
		if err := c.Validate(nil); err != nil {
			t.Fatalf("type %s: unexpected error: %v", ct, err)
		}
	}
}
