package commands_test

import (
	"errors"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Conformance assertions: verify each command implements asynxModels.Command[domain.AgentChat].
var _ asynxModels.Command[domain.AgentChat] = commands.Create{}
var _ asynxModels.Command[domain.AgentChat] = commands.OpenSegment{}
var _ asynxModels.Command[domain.AgentChat] = commands.EndSegment{}

func TestOpenSegment_RejectsSecondActive(t *testing.T) {
	now := time.Unix(0, 0)
	chat := &domain.AgentChat{
		ID:              "c1",
		ActiveSegmentID: "s1",
		Segments:        []domain.AgentSegment{{ID: "s1", Status: "active"}},
	}
	c := commands.OpenSegment{ChatID: "c1", SegmentID: "s2", CrowbarSegmentID: "cs2", ProviderID: "codex", Now: now}
	if err := c.Validate(chat); err == nil {
		t.Fatal("opening a 2nd active segment must fail Validate")
	}
}

// TestOpenSegment_EmitEvent_AppendsWithoutAliasing is the purity guard: EmitEvent
// must return a chat with the new segment appended (active, set as ActiveSegmentID)
// WITHOUT aliasing the caller's backing array. The input slice is given spare
// capacity (len 1, cap 2) and a retained alias extends into the spare slot; a bare
// append(current.Segments, ...) would write the new segment into that shared slot,
// which this test detects. The copy-append production code writes to a fresh array,
// leaving the alias untouched.
func TestOpenSegment_EmitEvent_AppendsWithoutAliasing(t *testing.T) {
	now := time.Unix(20, 0)
	segs := make([]domain.AgentSegment, 1, 2)
	segs[0] = domain.AgentSegment{ID: "s1", Status: "ended"}
	chat := &domain.AgentChat{
		ID:              "c1",
		ActiveSegmentID: "",
		Segments:        segs,
	}
	beforeLen := len(chat.Segments)
	beforeSeg0 := chat.Segments[0] // value snapshot
	// alias shares the backing array; alias[1] is the zero-value spare slot a bare
	// append would clobber.
	alias := chat.Segments[0:2:2]

	out := commands.OpenSegment{ChatID: "c1", SegmentID: "s2", CrowbarSegmentID: "cs2", ProviderID: "codex", Now: now}.EmitEvent(chat)

	// Output has the new segment appended, active, and set as the active id.
	if len(out.Segments) != beforeLen+1 {
		t.Fatalf("expected output len %d, got %d", beforeLen+1, len(out.Segments))
	}
	newSeg := out.Segments[len(out.Segments)-1]
	if newSeg.ID != "s2" || newSeg.Status != "active" {
		t.Fatalf("appended segment must be s2/active, got %+v", newSeg)
	}
	if out.ActiveSegmentID != "s2" {
		t.Fatalf("ActiveSegmentID must be s2, got %q", out.ActiveSegmentID)
	}

	// Aliasing guard: the caller's input slice header and elements are untouched.
	if len(chat.Segments) != beforeLen {
		t.Fatalf("input Segments len mutated: expected %d, got %d", beforeLen, len(chat.Segments))
	}
	if chat.Segments[0] != beforeSeg0 {
		t.Fatalf("input Segments[0] mutated: expected %+v, got %+v", beforeSeg0, chat.Segments[0])
	}
	// The load-bearing check: the shared spare slot must remain zero-value. A bare
	// append into current.Segments would have written the new segment here.
	if alias[1].ID != "" || alias[1].Status != "" {
		t.Fatalf("EmitEvent aliased the input backing array: spare slot clobbered to %+v", alias[1])
	}
}

func TestEndSegment_ClearsActive(t *testing.T) {
	now := time.Unix(10, 0)
	chat := &domain.AgentChat{
		ID:              "c1",
		ActiveSegmentID: "s1",
		Segments:        []domain.AgentSegment{{ID: "s1", Status: "active"}},
	}
	out := commands.EndSegment{ChatID: "c1", SegmentID: "s1", Now: now}.EmitEvent(chat)
	if out.ActiveSegmentID != "" || out.Segments[0].Status != "ended" || out.Segments[0].EndedAt == nil {
		t.Fatalf("EndSegment must end the active segment and clear ActiveSegmentID: %+v", out)
	}

	// Aliasing guard: the caller's input segment must NOT be ended — only out is.
	if chat.Segments[0].Status != "active" {
		t.Fatalf("input Segments[0].Status mutated: expected active, got %q", chat.Segments[0].Status)
	}
	if chat.Segments[0].EndedAt != nil {
		t.Fatalf("input Segments[0].EndedAt mutated: expected nil, got %v", chat.Segments[0].EndedAt)
	}
}

// TestEndSegment_MismatchedSegment_IsNoOp is the anti-corruption guard: a stale
// EndSegment naming the OLD segment ("s1") must be a no-op once a concurrent
// switch has already opened a NEW active segment ("s2"). It must NOT end the
// brand-new active segment (which would orphan its live CLI) and must NOT clear
// ActiveSegmentID — the whole aggregate is returned untouched. This is the
// TOCTOU close: the identity check is evaluated against the fresh event-log
// state inside EmitEvent, not a stale caller read.
func TestEndSegment_MismatchedSegment_IsNoOp(t *testing.T) {
	now := time.Unix(10, 0)
	endedAt := time.Unix(5, 0)
	chat := &domain.AgentChat{
		ID:              "c1",
		ActiveSegmentID: "s2",
		Segments: []domain.AgentSegment{
			{ID: "s1", Status: "ended", EndedAt: &endedAt},
			{ID: "s2", Status: "active"},
		},
	}
	// The caller meant to end "s1", but "s2" is now the active segment.
	out := commands.EndSegment{ChatID: "c1", SegmentID: "s1", Now: now}.EmitEvent(chat)

	// The new active segment and ActiveSegmentID are untouched — no orphaning.
	if out.ActiveSegmentID != "s2" {
		t.Fatalf("mismatched EndSegment must leave ActiveSegmentID as s2, got %q", out.ActiveSegmentID)
	}
	if out.Segments[1].Status != "active" || out.Segments[1].EndedAt != nil {
		t.Fatalf("mismatched EndSegment must not end the new active segment s2: %+v", out.Segments[1])
	}
	// LastActivityAt is not bumped on a no-op.
	if !out.LastActivityAt.Equal(chat.LastActivityAt) {
		t.Fatalf("mismatched EndSegment must not bump LastActivityAt: got %v", out.LastActivityAt)
	}
}

func TestCreate_ValidateAndEmit(t *testing.T) {
	now := time.Unix(5, 0)
	valid := commands.Create{ID: "c1", WorkspaceID: "w1", SegmentID: "s1", CrowbarSegmentID: "cs1", ProviderID: "claude", Now: now}

	// Validate(nil) on a valid command succeeds.
	if err := valid.Validate(nil); err != nil {
		t.Fatalf("Validate(nil) on a valid command must succeed, got %v", err)
	}

	// Validate against an existing chat rejects with ErrValidation.
	existing := &domain.AgentChat{ID: "c1"}
	err := valid.Validate(existing)
	if err == nil || !errors.Is(err, asynxModels.ErrValidation) {
		t.Fatalf("Validate against existing chat must wrap ErrValidation, got %v", err)
	}

	// EmitEvent seeds one active segment with ActiveSegmentID set.
	out := valid.EmitEvent(nil)
	if out.ActiveSegmentID != "s1" {
		t.Fatalf("EmitEvent must set ActiveSegmentID to s1, got %q", out.ActiveSegmentID)
	}
	if len(out.Segments) != 1 || out.Segments[0].ID != "s1" || out.Segments[0].Status != "active" {
		t.Fatalf("EmitEvent must seed one active segment s1, got %+v", out.Segments)
	}
}
