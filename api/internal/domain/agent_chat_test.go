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
	if c.CurrentTurnStarted != nil {
		t.Fatal("zero-value AgentChat must have no open turn")
	}
}
