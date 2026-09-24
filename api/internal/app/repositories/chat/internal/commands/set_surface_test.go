package commands_test

import (
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var _ asynxModels.Command[domain.Chat] = commands.SetSurface{}

func TestSetSurface_ValidateRejectsNil(t *testing.T) {
	err := commands.SetSurface{ChatID: "c1", Surface: "terminal"}.Validate(nil)
	if !errors.Is(err, asynxModels.ErrValidation) {
		t.Fatalf("Validate(nil) must reject with ErrValidation, got %v", err)
	}
}

func TestSetSurface_RejectsASurfaceNobodyCanBeOn(t *testing.T) {
	err := commands.SetSurface{ChatID: "c1", Surface: "hologram"}.Validate(&domain.Chat{ID: "c1"})
	if !errors.Is(err, asynxModels.ErrValidation) {
		t.Fatalf("an unknown surface must be refused, got %v", err)
	}
}

// TestRegression_SetSurfaceMovesTheChatRatherThanRecordingItsBirth is the
// whole point of the command: Surface used to be written once, at Create, and
// meant "born here". It is the CURRENT surface now — birth only seeds it —
// so a switch has to be able to move it.
func TestRegression_SetSurfaceMovesTheChatRatherThanRecordingItsBirth(t *testing.T) {
	out := commands.SetSurface{ChatID: "c1", Surface: domain.SurfaceTerminal}.
		EmitEvent(&domain.Chat{ID: "c1", Surface: domain.SurfaceChat, Title: "kept"})

	if out.Surface != domain.SurfaceTerminal {
		t.Fatalf("the chat must have moved, got %q", out.Surface)
	}
	if out.Title != "kept" {
		t.Fatal("a surface write must not disturb the rest of the aggregate")
	}
}

// "" is the provider's own default landing and a legitimate value to move
// BACK to — the chat surface under another name — so it must not be refused
// the way an unknown one is.
func TestSetSurface_TheProviderDefaultLandingIsAcceptable(t *testing.T) {
	if err := (commands.SetSurface{ChatID: "c1"}).Validate(&domain.Chat{ID: "c1"}); err != nil {
		t.Fatalf("the default landing must be acceptable: %v", err)
	}
	if out := (commands.SetSurface{ChatID: "c1"}).EmitEvent(&domain.Chat{ID: "c1", Surface: domain.SurfaceTerminal}); out.Surface != "" {
		t.Fatalf("an empty surface must clear, got %q", out.Surface)
	}
}

func TestSetSurface_EventNameCarriesTheKindAndID(t *testing.T) {
	cmd := commands.SetSurface{ChatID: "c1", Surface: domain.SurfaceTerminal}
	if cmd.AggregateID() != "c1" {
		t.Fatal("the aggregate is the chat")
	}
	if cmd.EventName() != "agentchat.surface_set.c1" {
		t.Fatalf("unexpected event name %q", cmd.EventName())
	}
	if cmd.ShouldSnapshot() {
		t.Fatal("a surface write is not a snapshot point")
	}
}
