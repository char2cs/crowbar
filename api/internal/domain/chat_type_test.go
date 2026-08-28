package domain_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestChatType_ClosedTaxonomy(t *testing.T) {
	want := []domain.ChatType{
		domain.ChatTypeChat,
		domain.ChatTypeBranch,
		domain.ChatTypeFolder,
		domain.ChatTypeWorkflow,
	}
	for _, tc := range want {
		if tc == "" {
			t.Fatalf("chat type constant is empty")
		}
	}
	if domain.ChatTypeBranch == domain.ChatTypeFolder {
		t.Fatalf("branch and folder must be distinct")
	}
}
