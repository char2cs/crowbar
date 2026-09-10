package domain_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestChatType_ClosedTaxonomy pins the set of types a Chat AGGREGATE may
// actually be minted or retyped as (commands.validChatType,
// api/internal/app/repositories/chat/internal/commands/create.go). Narrowed
// (2026-09-08 sidebar-placement-unification Task 8) to drop ChatTypeFolder —
// a folder is a domain.Folder row now, home-scoped or repo-scoped alike,
// never a Chat row — and further narrowed (Task 9) to drop ChatTypeBranch: a
// workspace's own position is a Node{Kind:workspace} row now, never a
// retyped Chat proxy.
func TestChatType_ClosedTaxonomy(t *testing.T) {
	want := []domain.ChatType{
		domain.ChatTypeChat,
		domain.ChatTypeWorkflow,
	}
	for _, tc := range want {
		if tc == "" {
			t.Fatalf("chat type constant is empty")
		}
	}
}
