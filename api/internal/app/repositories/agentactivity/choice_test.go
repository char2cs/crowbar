package agentactivity_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// openReply starts the assistant turn a prompt can be waiting inside. A prompt
// with no turn open is recorded already resolved, which is the shipped rule for
// the interruption that accompanies it.
func (f fixture) openReply(t *testing.T) {
	t.Helper()
	require.NoError(t, f.repo.OpenTurn(f.ctx, agentactivity.TurnInput{
		ChatID: chat, TurnID: "t1", ProviderID: "claude", RunnerID: "r1", Now: t0,
	}))
}

func (f fixture) permission(t *testing.T, choiceID, toolName string) {
	t.Helper()
	require.NoError(t, f.repo.OpenChoice(f.ctx, agentactivity.ChoiceInput{
		ChatID: chat, ChoiceID: choiceID, Kind: domain.ChoiceKindPermission,
		PromptID: "p-" + choiceID, ToolName: toolName, Title: toolName,
		Question: "may I run this",
		Options: []domain.ActivityChoiceOption{
			{ID: "allow", Kind: domain.ChoiceOptionAllow, Label: "Allow"},
			{ID: "deny", Kind: domain.ChoiceOptionDeny, Label: "Deny"},
		},
		Now: t0,
	}))
}

func TestOpenChoice_IsReadableAsAPendingPrompt(t *testing.T) {
	f := newFixture(t)
	f.openReply(t)
	f.permission(t, "c1", "Bash")
	f.wait()

	got, err := f.repo.PendingChoices(f.ctx, chat)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, domain.ChoiceKindPermission, got[0].Kind)
	assert.Equal(t, "Bash", got[0].ToolName)
	assert.Equal(t, "may I run this", got[0].Question)
	require.Len(t, got[0].Options, 2, "the options survive the round trip through storage")
	assert.Equal(t, "Allow", got[0].Options[0].Label)
	assert.True(t, got[0].Pending())
}

// A permission answered at the PTY reports nothing at all, so the gated work
// proceeding is what has to clear it.
func TestCompleteTool_ClearsThePendingPromptThatGatedIt(t *testing.T) {
	f := newFixture(t)
	f.openReply(t)
	require.NoError(t, f.repo.InvokeTool(f.ctx, agentactivity.ToolInput{
		ChatID: chat, ToolID: "tool-1", Name: "Bash", Now: t0,
	}))
	f.permission(t, "c1", "Bash")
	f.wait()
	require.Len(t, mustPending(t, f), 1)

	require.NoError(t, f.repo.CompleteTool(f.ctx, agentactivity.ToolResultInput{
		ChatID: chat, ToolID: "tool-1", Name: "Bash", Status: domain.ToolStatusOK, Now: t0,
	}))
	f.wait()

	assert.Empty(t, mustPending(t, f))
	all, err := f.repo.Choices(f.ctx, chat)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, domain.ChoiceResolutionProceeded, all[0].Resolution)
}

func TestResolveChoice_ClearsThePromptExplicitly(t *testing.T) {
	f := newFixture(t)
	f.openReply(t)
	f.permission(t, "c1", "Bash")
	f.wait()

	require.NoError(t, f.repo.ResolveChoice(
		f.ctx, chat, "c1", domain.ChoiceResolutionAnswered, t0))
	f.wait()

	assert.Empty(t, mustPending(t, f))
	all, err := f.repo.Choices(f.ctx, chat)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, domain.ChoiceResolutionAnswered, all[0].Resolution)
	assert.Equal(t, "may I run this", all[0].Question,
		"resolving must not blank the question the prompt was asking")
}

// A failure is a COMPLETION. The call stops running, keeps its reason, and the
// prompt it was gating clears with it.
func TestCompleteTool_AFailureEndsTheCallAndCarriesItsError(t *testing.T) {
	f := newFixture(t)
	f.openReply(t)
	require.NoError(t, f.repo.InvokeTool(f.ctx, agentactivity.ToolInput{
		ChatID: chat, ToolID: "tool-1", Name: "Bash", Now: t0,
	}))
	f.wait()

	require.NoError(t, f.repo.CompleteTool(f.ctx, agentactivity.ToolResultInput{
		ChatID: chat, ToolID: "tool-1", Name: "Bash", Status: domain.ToolStatusError,
		Error: "exit status 1", Result: []byte("exit status 1"), Now: t0,
	}))
	f.wait()

	calls, err := f.repo.ToolCalls(f.ctx, chat, 0, 0)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, domain.ToolStatusError, calls[0].Status)
	assert.Equal(t, "exit status 1", calls[0].Error)
	require.NotNil(t, calls[0].EndedAt)
	require.NotEmpty(t, calls[0].ResultRef, "the full failure text is still addressable")
}

// The inline caption is a caption. A tool that fails on a hundred kilobytes of
// compiler output must not put a hundred kilobytes into every later snapshot.
func TestCompleteTool_TruncatesAnEnormousErrorToACaption(t *testing.T) {
	f := newFixture(t)
	f.openReply(t)

	require.NoError(t, f.repo.CompleteTool(f.ctx, agentactivity.ToolResultInput{
		ChatID: chat, ToolID: "tool-1", Name: "Bash", Status: domain.ToolStatusError,
		Error: strings.Repeat("é", 40_000), Now: t0,
	}))
	f.wait()

	calls, err := f.repo.ToolCalls(f.ctx, chat, 0, 0)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.LessOrEqual(t, len(calls[0].Error), 2<<10)
	assert.True(t, utf8.ValidString(calls[0].Error),
		"a multi-byte character straddling the cut is dropped whole, never left broken")
}

func TestForget_DropsAChatsPrompts(t *testing.T) {
	f := newFixture(t)
	f.openReply(t)
	f.permission(t, "c1", "Bash")
	f.wait()

	require.NoError(t, f.repo.Forget(f.ctx, chat))

	all, err := f.repo.Choices(f.ctx, chat)
	require.NoError(t, err)
	assert.Empty(t, all)
}

func mustPending(t *testing.T, f fixture) []domain.ActivityChoice {
	t.Helper()
	got, err := f.repo.PendingChoices(f.ctx, chat)
	require.NoError(t, err)
	return got
}
