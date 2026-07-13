package commands_test

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Conformance assertions: verify each command implements asynxModels.Command[domain.AgentChat].
var _ asynxModels.Command[domain.AgentChat] = commands.SetTitle{}

func TestSetTitle_LockedRejectsDerived(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1", Title: "User Title", TitleLocked: true}
	if err := (commands.SetTitle{ChatID: "c1", Title: "derived", Source: "derived"}).Validate(chat); err == nil {
		t.Fatal("a locked title must reject a derived overwrite")
	}
}

func TestSetTitle_ValidateRejectsNil(t *testing.T) {
	if err := (commands.SetTitle{ChatID: "c1", Title: "new", Source: "user"}).Validate(nil); err == nil {
		t.Fatal("Validate(nil) must reject with ErrValidation")
	}
}

func TestSetTitle_UserSourceLocksTitle(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1", Title: "old", TitleLocked: false}
	out := commands.SetTitle{ChatID: "c1", Title: "new", Source: "user"}.EmitEvent(chat)
	if !out.TitleLocked {
		t.Fatal("user source must set TitleLocked=true")
	}
	if out.Title != "new" {
		t.Fatal("Title must be set to new")
	}
}

func TestSetTitle_AgentSourceDoesNotLock(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1", Title: "old", TitleLocked: false}
	out := commands.SetTitle{ChatID: "c1", Title: "new", Source: "agent"}.EmitEvent(chat)
	if out.TitleLocked {
		t.Fatal("agent source must NOT set TitleLocked")
	}
	if out.Title != "new" {
		t.Fatal("Title must be set to new")
	}
}

func TestSetTitle_LockedRejectsAgent(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1", Title: "User Title", TitleLocked: true}
	if err := (commands.SetTitle{ChatID: "c1", Title: "agent", Source: "agent"}).Validate(chat); err == nil {
		t.Fatal("a locked title must reject an agent overwrite")
	}
}

func TestSetTitle_UserCanOverrideLocked(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1", Title: "Locked", TitleLocked: true}
	if err := (commands.SetTitle{ChatID: "c1", Title: "new", Source: "user"}).Validate(chat); err != nil {
		t.Fatalf("user source must be able to override a locked title, got %v", err)
	}
}

func TestSetTitle_EmitEventDoesNotMutateInput(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1", Title: "old", TitleLocked: false}
	originalTitle := chat.Title
	originalTitleLocked := chat.TitleLocked

	commands.SetTitle{ChatID: "c1", Title: "new", Source: "user"}.EmitEvent(chat)

	if chat.Title != originalTitle {
		t.Fatalf("input Title mutated: expected %q, got %q", originalTitle, chat.Title)
	}
	if chat.TitleLocked != originalTitleLocked {
		t.Fatalf("input TitleLocked mutated: expected %v, got %v", originalTitleLocked, chat.TitleLocked)
	}
}

func TestSetTitle_ValidateErrorIsWrapped(t *testing.T) {
	err := (commands.SetTitle{ChatID: "c1", Title: "new", Source: "user"}).Validate(nil)
	if !errors.Is(err, asynxModels.ErrValidation) {
		t.Fatalf("Validate error must wrap ErrValidation, got %v", err)
	}
}
