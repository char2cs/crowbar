package commands_test

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var _ asynxModels.Command[domain.AgentChat] = commands.SetSelection{}

func TestSetSelection_ValidateRejectsNil(t *testing.T) {
	err := commands.SetSelection{ChatID: "c1", Model: "opus"}.Validate(nil)
	if !errors.Is(err, asynxModels.ErrValidation) {
		t.Fatalf("Validate(nil) must reject with ErrValidation, got %v", err)
	}
}

func TestSetSelection_ValidateAcceptsAnExistingChat(t *testing.T) {
	if err := (commands.SetSelection{ChatID: "c1"}).Validate(&domain.AgentChat{ID: "c1"}); err != nil {
		t.Fatalf("an existing chat must accept a selection: %v", err)
	}
}

func TestSetSelection_WritesBothHalves(t *testing.T) {
	out := commands.SetSelection{ChatID: "c1", Model: "opus", Effort: "high"}.
		EmitEvent(&domain.AgentChat{ID: "c1", Title: "kept"})

	if out.Model != "opus" || out.Effort != "high" {
		t.Fatalf("both halves must be written, got %q/%q", out.Model, out.Effort)
	}
	if out.Title != "kept" {
		t.Fatal("a selection write must not disturb the rest of the aggregate")
	}
}

func TestSetSelection_EmptyClearsRatherThanSkips(t *testing.T) {
	out := commands.SetSelection{ChatID: "c1"}.
		EmitEvent(&domain.AgentChat{ID: "c1", Model: "opus", Effort: "high"})

	if out.Model != "" || out.Effort != "" {
		t.Fatalf("an empty selection must clear, got %q/%q", out.Model, out.Effort)
	}
}

func TestSetSelection_EventNameCarriesTheKindAndID(t *testing.T) {
	cmd := commands.SetSelection{ChatID: "c1"}
	if cmd.AggregateID() != "c1" {
		t.Fatal("the aggregate is the chat")
	}
	if cmd.EventName() != "agentchat.selection_set.c1" {
		t.Fatalf("unexpected event name %q", cmd.EventName())
	}
	if cmd.ShouldSnapshot() {
		t.Fatal("a selection write is not a snapshot point")
	}
}
