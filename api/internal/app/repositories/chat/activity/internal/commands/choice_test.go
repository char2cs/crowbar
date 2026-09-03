package commands_test

import (
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func openTurnState(t *testing.T) domain.ChatActivity {
	t.Helper()
	return commands.OpenTurn{ChatID: chat, TurnID: "t1", ProviderID: "claude", Now: now}.
		EmitEvent(nil)
}

func permissionCmd() commands.OpenChoice {
	return commands.OpenChoice{
		ChatID: chat, ChoiceID: "c1", Kind: domain.ChoiceKindPermission,
		PromptID: "81899da5", ToolName: "Bash", Title: "Bash",
		Options: []domain.ActivityChoiceOption{
			{ID: "allow", Kind: domain.ChoiceOptionAllow, Label: "Allow"},
			{ID: "deny", Kind: domain.ChoiceOptionDeny, Label: "Deny"},
		},
		Now: now,
	}
}

func TestOpenChoice_HoldsThePromptOpenDuringATurn(t *testing.T) {
	state := openTurnState(t)

	got := permissionCmd().EmitEvent(&state)

	require.NotNil(t, got.Last)
	assert.Equal(t, domain.DeltaOpen, got.Last.Phase)
	assert.Equal(t, domain.DeltaChoice, got.Last.Kind)
	require.NotNil(t, got.Last.Choice)
	assert.Equal(t, domain.ChoiceKindPermission, got.Last.Choice.Kind)
	assert.Equal(t, "81899da5", got.Last.Choice.PromptID)
	assert.Equal(t, "t1", got.Last.Choice.TurnID)
	assert.True(t, got.Last.Choice.Pending())
	assert.Len(t, got.Choices, 1, "a prompt waiting on a human is open state")
}

func TestOpenChoice_WithNoTurnOpenIsRecordedAlreadyResolved(t *testing.T) {
	got := permissionCmd().EmitEvent(nil)

	require.NotNil(t, got.Last)
	assert.Equal(t, domain.DeltaClose, got.Last.Phase)
	require.NotNil(t, got.Last.Choice)
	assert.False(t, got.Last.Choice.Pending())
	assert.Equal(t, domain.ChoiceResolutionAbandoned, got.Last.Choice.Resolution)
	assert.Empty(t, got.Choices, "nothing is left open")
	assert.Nil(t, got.Turn, "and no turn is conjured to hold it")
}

func TestOpenChoice_AdoptsTheInFlightCallOfTheSameName(t *testing.T) {
	state := openTurnState(t)
	state = commands.InvokeTool{ChatID: chat, ToolID: "old", Name: "Bash", Now: now}.
		EmitEvent(&state)
	state = commands.InvokeTool{ChatID: chat, ToolID: "new", Name: "Bash", Now: now}.
		EmitEvent(&state)
	state = commands.InvokeTool{ChatID: chat, ToolID: "other", Name: "Edit", Now: now}.
		EmitEvent(&state)

	got := permissionCmd().EmitEvent(&state)

	require.NotNil(t, got.Last.Choice)
	assert.Equal(t, "new", got.Last.Choice.ToolID, "the newest call of that name")
}

func TestOpenChoice_WithNoMatchingCallCarriesOnlyTheName(t *testing.T) {
	state := openTurnState(t)

	got := permissionCmd().EmitEvent(&state)

	require.NotNil(t, got.Last.Choice)
	assert.Empty(t, got.Last.Choice.ToolID)
	assert.Equal(t, "Bash", got.Last.Choice.ToolName)
}

func TestOpenChoice_RejectsTheUnusableCases(t *testing.T) {
	testCases := []struct {
		name string
		cmd  commands.OpenChoice
	}{
		{"no chat", commands.OpenChoice{ChoiceID: "c1", Kind: domain.ChoiceKindPermission}},
		{"no choice id", commands.OpenChoice{ChatID: chat, Kind: domain.ChoiceKindPermission}},
		{"no kind", commands.OpenChoice{ChatID: chat, ChoiceID: "c1"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.ErrorIs(t, tc.cmd.Validate(nil), asynxModels.ErrValidation)
		})
	}
}

func TestCompleteTool_ClosesThePromptThatWasGatingIt(t *testing.T) {
	state := openTurnState(t)
	state = commands.InvokeTool{ChatID: chat, ToolID: "t-1", Name: "Bash", Now: now}.
		EmitEvent(&state)
	state = permissionCmd().EmitEvent(&state)
	require.Len(t, state.Choices, 1)

	got := commands.CompleteTool{ChatID: chat, ToolID: "t-1", Name: "Bash", Now: now}.
		EmitEvent(&state)

	assert.Empty(t, got.Choices, "the question is no longer being asked")
	assert.Equal(t, domain.DeltaTool, got.Last.Kind,
		"the delta stays the tool's; the prompt's row is swept by the projection")
}

func TestCompleteTool_ClosesAPromptThatOnlyKnowsTheToolName(t *testing.T) {
	state := openTurnState(t)
	state = permissionCmd().EmitEvent(&state)
	require.Empty(t, state.Choices["c1"].ToolID)

	got := commands.CompleteTool{ChatID: chat, ToolID: "t-1", Name: "Bash", Now: now}.
		EmitEvent(&state)

	assert.Empty(t, got.Choices)
}

func TestCompleteTool_LeavesAPromptAboutAnotherToolAlone(t *testing.T) {
	state := openTurnState(t)
	state = permissionCmd().EmitEvent(&state)

	got := commands.CompleteTool{ChatID: chat, ToolID: "t-9", Name: "Edit", Now: now}.
		EmitEvent(&state)

	assert.Len(t, got.Choices, 1)
}

func TestCompleteTool_AnAnonymousCompletionClosesNoPrompt(t *testing.T) {
	state := openTurnState(t)
	state = permissionCmd().EmitEvent(&state)

	got := commands.CompleteTool{ChatID: chat, ToolID: "t-2", Now: now}.EmitEvent(&state)

	assert.Len(t, got.Choices, 1)
}

func TestCompleteTool_CarriesAFailuresErrorOntoTheCall(t *testing.T) {
	state := openTurnState(t)
	state = commands.InvokeTool{ChatID: chat, ToolID: "t-1", Name: "Bash", Now: now}.
		EmitEvent(&state)

	got := commands.CompleteTool{
		ChatID: chat, ToolID: "t-1", Name: "Bash",
		Status: domain.ToolStatusError, Error: "exit status 1", Now: now,
	}.EmitEvent(&state)

	require.NotNil(t, got.Last.Tool)
	assert.Equal(t, domain.ToolStatusError, got.Last.Tool.Status)
	assert.Equal(t, "exit status 1", got.Last.Tool.Error)
	require.NotNil(t, got.Last.Tool.EndedAt, "a failed tool is a FINISHED tool")
	assert.Empty(t, got.Tools, "and it stops being in flight")
}

func TestResolveChoice_ClosesTheOpenPrompt(t *testing.T) {
	state := openTurnState(t)
	state = permissionCmd().EmitEvent(&state)

	c := commands.ResolveChoice{
		ChatID: chat, ChoiceID: "c1", Resolution: domain.ChoiceResolutionAnswered, Now: now,
	}
	require.NoError(t, c.Validate(nil))
	got := c.EmitEvent(&state)

	require.NotNil(t, got.Last)
	assert.Equal(t, domain.DeltaClose, got.Last.Phase)
	require.NotNil(t, got.Last.Choice)
	assert.Equal(t, domain.ChoiceResolutionAnswered, got.Last.Choice.Resolution)
	assert.Equal(t, "Bash", got.Last.Choice.ToolName, "the question it asked is preserved")
	assert.Empty(t, got.Choices)
}

func TestResolveChoice_DefaultsToAnsweredWhenNoReasonIsGiven(t *testing.T) {
	state := openTurnState(t)
	state = permissionCmd().EmitEvent(&state)

	got := commands.ResolveChoice{ChatID: chat, ChoiceID: "c1", Now: now}.EmitEvent(&state)

	require.NotNil(t, got.Last.Choice)
	assert.Equal(t, domain.ChoiceResolutionAnswered, got.Last.Choice.Resolution)
}

func TestResolveChoice_ForAnUnknownPromptPublishesNothing(t *testing.T) {
	state := openTurnState(t)

	got := commands.ResolveChoice{ChatID: chat, ChoiceID: "ghost", Now: now}.EmitEvent(&state)

	assert.Nil(t, got.Last)
	assert.Equal(t, state.Seq+1, got.Seq, "the attempt is still recorded in the log")
}

func TestResolveChoice_RejectsTheUnusableCases(t *testing.T) {
	testCases := []struct {
		name string
		cmd  commands.ResolveChoice
	}{
		{"no chat", commands.ResolveChoice{ChoiceID: "c1"}},
		{"no choice id", commands.ResolveChoice{ChatID: chat}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.ErrorIs(t, tc.cmd.Validate(nil), asynxModels.ErrValidation)
		})
	}
}

func TestTurnBoundaries_ClearEveryPendingPrompt(t *testing.T) {
	testCases := []struct {
		name string
		next func(domain.ChatActivity) domain.ChatActivity
	}{
		{"close", func(s domain.ChatActivity) domain.ChatActivity {
			return commands.CloseTurn{ChatID: chat, TurnID: "t2", Text: "done", Now: now}.EmitEvent(&s)
		}},
		{"abandon", func(s domain.ChatActivity) domain.ChatActivity {
			return commands.Abandon{ChatID: chat, Now: now}.EmitEvent(&s)
		}},
		{"superseded by a new turn", func(s domain.ChatActivity) domain.ChatActivity {
			return commands.OpenTurn{ChatID: chat, TurnID: "t2", Now: now}.EmitEvent(&s)
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			state := openTurnState(t)
			state = permissionCmd().EmitEvent(&state)
			require.Len(t, state.Choices, 1)

			assert.Empty(t, tc.next(state).Choices)
		})
	}
}

func TestOpenChoice_CannotGrowTheAggregatePastItsCeiling(t *testing.T) {
	state := openTurnState(t)
	for i := range domain.MaxOpenPerTurn + 20 {
		cmd := permissionCmd()
		cmd.ChoiceID = "c" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		state = cmd.EmitEvent(&state)
	}

	assert.LessOrEqual(t, state.OpenCount(), domain.MaxOpenPerTurn)
	assert.NotNil(t, state.Last, "and every one of them is still reported")
}
