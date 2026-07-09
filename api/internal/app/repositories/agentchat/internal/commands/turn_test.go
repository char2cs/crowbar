package commands_test

import (
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Conformance assertions: verify each command implements asynxModels.Command[domain.AgentChat].
var _ asynxModels.Command[domain.AgentChat] = commands.StartTurn{}
var _ asynxModels.Command[domain.AgentChat] = commands.StopTurn{}
var _ asynxModels.Command[domain.AgentChat] = commands.BindSession{}

func TestStartStopTurn_TogglesWorking(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1"}
	started := commands.StartTurn{ChatID: "c1", Now: time.Unix(1, 0)}.EmitEvent(chat)
	if !started.Working || started.CurrentTurnStarted == nil {
		t.Fatal("StartTurn must set Working + CurrentTurnStarted")
	}
	stopped := commands.StopTurn{ChatID: "c1", Now: time.Unix(2, 0)}.EmitEvent(&started)
	if stopped.Working || stopped.CurrentTurnStarted != nil {
		t.Fatal("StopTurn must clear Working + CurrentTurnStarted")
	}
}

// TestStartTurn_EmitEvent_InputUnmutated is the purity guard: EmitEvent must return a
// chat with Working=true and CurrentTurnStarted set WITHOUT mutating the input chat.
func TestStartTurn_EmitEvent_InputUnmutated(t *testing.T) {
	chat := &domain.AgentChat{ID: "c1", Working: false, CurrentTurnStarted: nil}
	beforeWorking := chat.Working
	beforeTurnStarted := chat.CurrentTurnStarted

	out := commands.StartTurn{ChatID: "c1", Now: time.Unix(1, 0)}.EmitEvent(chat)

	// Output has Working=true and CurrentTurnStarted set.
	if !out.Working || out.CurrentTurnStarted == nil {
		t.Fatalf("StartTurn output must have Working=true and CurrentTurnStarted set")
	}

	// Input is unmutated.
	if chat.Working != beforeWorking {
		t.Fatalf("input Working mutated: expected %v, got %v", beforeWorking, chat.Working)
	}
	if chat.CurrentTurnStarted != beforeTurnStarted {
		t.Fatalf("input CurrentTurnStarted mutated: expected %v, got %v", beforeTurnStarted, chat.CurrentTurnStarted)
	}
}

// TestStopTurn_EmitEvent_InputUnmutated is the purity guard: EmitEvent must return a
// chat with Working=false and CurrentTurnStarted=nil WITHOUT mutating the input chat.
func TestStopTurn_EmitEvent_InputUnmutated(t *testing.T) {
	turnStarted := time.Unix(1, 0)
	chat := &domain.AgentChat{ID: "c1", Working: true, CurrentTurnStarted: &turnStarted}
	beforeWorking := chat.Working
	beforeTurnStarted := chat.CurrentTurnStarted

	out := commands.StopTurn{ChatID: "c1", Now: time.Unix(2, 0)}.EmitEvent(chat)

	// Output has Working=false and CurrentTurnStarted=nil.
	if out.Working || out.CurrentTurnStarted != nil {
		t.Fatalf("StopTurn output must have Working=false and CurrentTurnStarted=nil")
	}

	// Input is unmutated.
	if chat.Working != beforeWorking {
		t.Fatalf("input Working mutated: expected %v, got %v", beforeWorking, chat.Working)
	}
	if chat.CurrentTurnStarted != beforeTurnStarted {
		t.Fatalf("input CurrentTurnStarted mutated: expected %v, got %v", beforeTurnStarted, chat.CurrentTurnStarted)
	}
}

func TestBindSession_SetsSessionOnSegment(t *testing.T) {
	chat := &domain.AgentChat{
		ID:       "c1",
		Segments: []domain.AgentSegment{{ID: "s1", CrowbarSegmentID: "cs1", Status: "active"}},
	}
	out := commands.BindSession{ChatID: "c1", CrowbarSegmentID: "cs1", ProviderSessionID: "claude-sess-1"}.EmitEvent(chat)
	if out.Segments[0].ProviderSessionID != "claude-sess-1" {
		t.Fatal("BindSession must set ProviderSessionID on the matching segment")
	}
}

// TestBindSession_EmitEvent_InputUnmutated is the purity guard: EmitEvent must return a
// chat with ProviderSessionID set on the matching segment WITHOUT mutating the input chat
// or aliasing its backing array.
func TestBindSession_EmitEvent_InputUnmutated(t *testing.T) {
	segs := make([]domain.AgentSegment, 1, 2)
	segs[0] = domain.AgentSegment{ID: "s1", CrowbarSegmentID: "cs1", Status: "active", ProviderSessionID: ""}
	chat := &domain.AgentChat{ID: "c1", Segments: segs}
	beforeSeg := chat.Segments[0] // value snapshot
	alias := chat.Segments[0:2:2] // alias with spare slot

	out := commands.BindSession{
		ChatID:            "c1",
		CrowbarSegmentID:  "cs1",
		ProviderSessionID: "claude-sess-1",
	}.EmitEvent(chat)

	// Output has ProviderSessionID set on the matching segment.
	if out.Segments[0].ProviderSessionID != "claude-sess-1" {
		t.Fatalf("BindSession output must have ProviderSessionID set, got %q", out.Segments[0].ProviderSessionID)
	}

	// Input is unmutated.
	if chat.Segments[0].ProviderSessionID != beforeSeg.ProviderSessionID {
		t.Fatalf("input Segments[0].ProviderSessionID mutated: expected %q, got %q", beforeSeg.ProviderSessionID, chat.Segments[0].ProviderSessionID)
	}

	// Aliasing guard: the spare slot must remain zero-value.
	if alias[1].ID != "" || alias[1].Status != "" {
		t.Fatalf("BindSession aliased the input backing array: spare slot clobbered to %+v", alias[1])
	}
}
