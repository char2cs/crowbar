package projections_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity/internal/store/internal/projections"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity/internal/store/internal/storage"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func openPrompt(chatID string) domain.ActivityChoice {
	return domain.ActivityChoice{
		ID: "c1", TurnID: "t1", ChatID: chatID, Seq: 2,
		Kind: domain.ChoiceKindPermission, PromptID: "81899da5",
		ToolID: "tool-1", ToolName: "Bash", Title: "Bash",
		Options: []domain.ActivityChoiceOption{
			{ID: "allow", Kind: domain.ChoiceOptionAllow, Label: "Allow"},
			{ID: "deny", Kind: domain.ChoiceOptionDeny, Label: "Deny"},
		},
		At: now,
	}
}

func project(t *testing.T, p *projections.Projector, chatID string, delta domain.ActivityDelta) {
	t.Helper()
	require.NoError(t, p.Apply(
		context.Background(),
		domain.AgentActivity{ChatID: chatID, Last: &delta},
	))
}

func pending(t *testing.T, st *storage.Store, chatID string) []domain.ActivityChoice {
	t.Helper()
	got, err := st.PendingChoices(context.Background(), chatID)
	require.NoError(t, err)
	return got
}

func TestApply_APromptIsProjectedWithItsOptions(t *testing.T) {
	p, st := newProjector(t)
	item := openPrompt("c1")

	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaChoice, Choice: &item,
	})

	got := pending(t, st, "c1")
	require.Len(t, got, 1)
	assert.Equal(t, domain.ChoiceKindPermission, got[0].Kind)
	assert.Equal(t, "81899da5", got[0].PromptID)
	require.Len(t, got[0].Options, 2)
	assert.Equal(t, "Allow", got[0].Options[0].Label)
	assert.True(t, got[0].Pending())
}

// The path that actually fires: a human answered at the PTY, nothing reported it,
// and the gated work proceeding is the only evidence there is.
func TestApply_AToolCompletingClosesThePromptThatGatedIt(t *testing.T) {
	p, st := newProjector(t)
	item := openPrompt("c1")
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaChoice, Choice: &item,
	})

	ended := now
	call := domain.ActivityToolCall{
		ID: "tool-1", TurnID: "t1", ChatID: "c1", Seq: 3, Name: "Bash",
		Status: domain.ToolStatusOK, StartedAt: now, EndedAt: &ended,
	}
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTool, Tool: &call,
	})

	assert.Empty(t, pending(t, st, "c1"))
	all, err := st.Choices(context.Background(), "c1")
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, domain.ChoiceResolutionProceeded, all[0].Resolution)
	assert.NotNil(t, all[0].ResolvedAt)
}

// A FAILED tool answers its prompt exactly as a successful one does — the
// question was answered, the work simply went badly afterwards.
func TestApply_AFailedToolAlsoClosesThePromptThatGatedIt(t *testing.T) {
	p, st := newProjector(t)
	item := openPrompt("c1")
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaChoice, Choice: &item,
	})

	ended := now
	call := domain.ActivityToolCall{
		ID: "tool-1", ChatID: "c1", Seq: 3, Name: "Bash",
		Status: domain.ToolStatusError, Error: "exit status 1", StartedAt: now, EndedAt: &ended,
	}
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTool, Tool: &call,
	})

	assert.Empty(t, pending(t, st, "c1"))
	calls, err := st.ToolCalls(context.Background(), "c1", 0, 0)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, "exit status 1", calls[0].Error)
}

// A prompt that never learned a call id — a lost PreToolUse, or a permission that
// beat it — must still clear on the name alone.
func TestApply_AToolCompletingClosesANamedPromptWithNoCallID(t *testing.T) {
	p, st := newProjector(t)
	item := openPrompt("c1")
	item.ToolID = ""
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaChoice, Choice: &item,
	})

	ended := now
	call := domain.ActivityToolCall{
		ID: "tool-9", ChatID: "c1", Seq: 3, Name: "Bash", StartedAt: now, EndedAt: &ended,
	}
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTool, Tool: &call,
	})

	assert.Empty(t, pending(t, st, "c1"))
}

func TestApply_AToolCompletingLeavesAnUnrelatedPromptPending(t *testing.T) {
	p, st := newProjector(t)
	item := openPrompt("c1")
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaChoice, Choice: &item,
	})

	ended := now
	call := domain.ActivityToolCall{
		ID: "tool-9", ChatID: "c1", Seq: 3, Name: "Edit", StartedAt: now, EndedAt: &ended,
	}
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTool, Tool: &call,
	})

	assert.Len(t, pending(t, st, "c1"), 1)
}

// An INVOCATION is not an answer. Only a completion says the work proceeded.
func TestApply_AToolStartingDoesNotClosePrompts(t *testing.T) {
	p, st := newProjector(t)
	item := openPrompt("c1")
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaChoice, Choice: &item,
	})

	call := domain.ActivityToolCall{
		ID: "tool-1", ChatID: "c1", Seq: 3, Name: "Bash",
		Status: domain.ToolStatusRunning, StartedAt: now,
	}
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaTool, Tool: &call,
	})

	assert.Len(t, pending(t, st, "c1"), 1)
}

// The backstop. An elicitation has no resolving event on any provider, so without
// the turn-boundary sweep a question the agent stopped asking would stay pinned
// over the chat for the rest of its life.
func TestApply_ATurnEndingClosesEveryPromptItLeftOpen(t *testing.T) {
	p, st := newProjector(t)
	item := openPrompt("c1")
	item.ToolID, item.ToolName = "", ""
	item.Kind = domain.ChoiceKindElicitation
	// An elicitation offers a schema, not buttons, so it stores no options at all.
	item.Options = nil
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaChoice, Choice: &item,
	})

	ended := now
	turn := domain.ActivityTurn{
		ID: "t1", ChatID: "c1", Seq: 4, Role: domain.TurnRoleAssistant,
		Text: "done", StartedAt: now, EndedAt: &ended,
	}
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTurn, Turn: &turn,
	})

	assert.Empty(t, pending(t, st, "c1"))
	all, err := st.Choices(context.Background(), "c1")
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, domain.ChoiceResolutionAbandoned, all[0].Resolution)
}

// The turn sweep must not overwrite a resolution the tool already recorded, or a
// prompt that was genuinely answered would read as abandoned.
func TestApply_ATurnEndingDoesNotRewriteAnAlreadyResolvedPrompt(t *testing.T) {
	p, st := newProjector(t)
	item := openPrompt("c1")
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaChoice, Choice: &item,
	})
	ended := now
	call := domain.ActivityToolCall{
		ID: "tool-1", ChatID: "c1", Seq: 3, Name: "Bash", StartedAt: now, EndedAt: &ended,
	}
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTool, Tool: &call,
	})

	turn := domain.ActivityTurn{
		ID: "t1", ChatID: "c1", Seq: 4, Role: domain.TurnRoleAssistant,
		Text: "done", StartedAt: now, EndedAt: &ended,
	}
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTurn, Turn: &turn,
	})

	all, err := st.Choices(context.Background(), "c1")
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, domain.ChoiceResolutionProceeded, all[0].Resolution)
}

// A prompt attaches to the placeholder identity the open turn was minted under;
// the reply lands under the delivery id. Without re-pointing, the UI cannot say
// which answer a question belonged to.
func TestApply_PromptsFollowTheTurnTheyWereRepointedOnto(t *testing.T) {
	p, st := newProjector(t)
	item := openPrompt("c1")
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaChoice, Choice: &item,
	})

	ended := now
	turn := domain.ActivityTurn{
		ID: "delivery-1", ChatID: "c1", Seq: 4, Role: domain.TurnRoleAssistant,
		Text: "done", StartedAt: now, EndedAt: &ended,
	}
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTurn, Turn: &turn,
		SupersededTurnID: "t1",
	})

	all, err := st.Choices(context.Background(), "c1")
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "delivery-1", all[0].TurnID)
}

func TestApply_AChoiceDeltaWithNoPayloadIsANoOp(t *testing.T) {
	p, st := newProjector(t)

	project(t, p, "c1", domain.ActivityDelta{Phase: domain.DeltaOpen, Kind: domain.DeltaChoice})

	assert.Empty(t, pending(t, st, "c1"))
}

// Forgetting a chat must take its prompts with it: a purged conversation that
// still shows a pending question is a conversation that was not purged.
func TestForget_RemovesAChatsPrompts(t *testing.T) {
	p, st := newProjector(t)
	item := openPrompt("c1")
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaChoice, Choice: &item,
	})

	require.NoError(t, p.Forget(context.Background(), "c1"))

	all, err := st.Choices(context.Background(), "c1")
	require.NoError(t, err)
	assert.Empty(t, all)
}

// A completion that names neither a call nor a tool identifies no prompt, and
// sweeping on it would clear every question in the chat — including ones about
// entirely different tools.
func TestApply_AnUnidentifiableCompletionClosesNoPrompt(t *testing.T) {
	p, st := newProjector(t)
	item := openPrompt("c1")
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaChoice, Choice: &item,
	})

	ended := now
	call := domain.ActivityToolCall{ChatID: "c1", Seq: 3, StartedAt: now, EndedAt: &ended}
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTool, Tool: &call,
	})

	assert.Len(t, pending(t, st, "c1"), 1)
}

// A provider that stops reporting tool names must still close the prompts its
// calls were gating, on the call id alone.
func TestApply_AnUnnamedCompletionStillClosesItsOwnPrompt(t *testing.T) {
	p, st := newProjector(t)
	item := openPrompt("c1")
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaChoice, Choice: &item,
	})

	ended := now
	call := domain.ActivityToolCall{ID: "tool-1", ChatID: "c1", Seq: 3, StartedAt: now, EndedAt: &ended}
	project(t, p, "c1", domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaTool, Tool: &call,
	})

	assert.Empty(t, pending(t, st, "c1"))
}
