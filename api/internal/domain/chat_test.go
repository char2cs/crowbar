package domain_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestChat_HasType(t *testing.T) {
	c := domain.Chat{Type: domain.ChatTypeBranch}
	if c.Type != domain.ChatTypeBranch {
		t.Fatalf("Type not settable/readable")
	}
}
