package domain_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestAgentChat_ZeroValueIsInactiveNotWorking(t *testing.T) {
	var c domain.AgentChat
	if c.Working {
		t.Fatal("zero-value AgentChat must not be Working")
	}
	if len(c.Segments) != 0 {
		t.Fatal("zero-value AgentChat must have no segments")
	}
}

func TestAgentChatStatus_Values(t *testing.T) {
	if domain.AgentChatStatusActive != "active" ||
		domain.AgentChatStatusDeleted != "deleted" {
		t.Fatal("unexpected status literals")
	}
}
