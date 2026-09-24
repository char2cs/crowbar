package move_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/move"
)

func TestDecide_BranchesOnlyOnChangedAndKnown(t *testing.T) {
	testCases := []struct {
		name       string
		current    string
		announced  string
		knownID    string
		known      bool
		originated bool
		want       models.Decision
	}{
		{
			name:    "same conversation is a no-op",
			current: "s1", announced: "s1",
			want: models.Decision{Kind: models.MoveNoop},
		},
		{
			name:    "first announcement binds where it is",
			current: "", announced: "s1",
			want: models.Decision{Kind: models.MoveBind},
		},
		{
			name:    "first announcement binds even for an already-known id",
			current: "", announced: "s1", knownID: "chat-9", known: true,
			want: models.Decision{Kind: models.MoveBind},
		},
		{
			name:    "moving to an unseen id mints a chat",
			current: "s1", announced: "s2",
			want: models.Decision{Kind: models.MoveToNew},
		},
		{
			name:    "moving to a known id re-points the runner",
			current: "s1", announced: "s2", knownID: "chat-2", known: true,
			want: models.Decision{Kind: models.MoveToKnown, ChatID: "chat-2"},
		},
		{
			// A conversation Crowbar's own driver opened. Identical on the wire to
			// the /clear two cases up; only this flag tells them apart.
			name:    "a conversation Crowbar originated binds where it is",
			current: "s1", announced: "s2", originated: true,
			want: models.Decision{Kind: models.MoveBind},
		},
		{
			name:    "both empty is a no-op, not a bind",
			current: "", announced: "",
			want: models.Decision{Kind: models.MoveNoop},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := move.Decide(tc.current, tc.announced, tc.knownID, tc.known, tc.originated)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDecide_HasNoRejectOutcome(t *testing.T) {
	for _, d := range []models.Decision{
		move.Decide("", "", "", false, false),
		move.Decide("a", "a", "", false, false),
		move.Decide("", "a", "", false, false),
		move.Decide("a", "b", "", false, false),
		move.Decide("a", "b", "c", true, false),
		move.Decide("a", "b", "", false, true),
	} {
		assert.Contains(t,
			[]models.MoveKind{models.MoveNoop, models.MoveBind, models.MoveToNew, models.MoveToKnown},
			d.Kind)
	}
}
