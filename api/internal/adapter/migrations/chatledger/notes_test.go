package chatledger

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func TestConvertChat_InterruptionsAndChoicesAreNotAnswerable(t *testing.T) {
	res := convert(t, chatA(), nobody)

	markers := bodies[ledger.Marker](t, res, ledger.KindMarker)
	assert.Equal(t, []ledger.Marker{{Kind: ledger.MarkerCompactionStarted}, {Kind: ledger.MarkerCompactionEnded}}, markers)
	open, _ := fold(res).Compaction()
	assert.False(t, open)
	assert.Contains(t, res.Skipped, "interruption i-2: permission, restated by its choice")
	asks := bodies[ledger.Ask](t, res, ledger.KindAsk)
	require.Len(t, asks, 1)
	assert.Equal(t, ledger.Ask{
		AskID: "ch-1", Event: kindPermission, Kind: "tool_permission", ToolID: "t-1", ToolName: "Read", Title: "Allow Read?",
		Options: json.RawMessage(`[{"id":"allow","kind":"allow","label":"Allow"}]`),
	}, asks[0], "no CLI holds a legacy ask, so none is answerable")
	assert.Equal(t, []ledger.AskResolved{{AskID: "ch-1", Resolution: ledger.ResolutionAnswered, OptionIDs: []string{"allow"}}},
		bodies[ledger.AskResolved](t, res, ledger.KindAskResolved))
	assert.Empty(t, fold(res).PendingAsks())
}

func TestConvertChat_AnAskNeverResolvedIsAbandonedWithItsTurn(t *testing.T) {
	in := chatA()
	in.Choices[0].ResolvedAt, in.Choices[0].Resolution = nil, ""

	res := convert(t, in, nobody)

	resolved := rowsOf(res, ledger.KindAskResolved)
	require.Len(t, resolved, 1)
	assert.Equal(t, ledger.ResolutionAbandoned, body[ledger.AskResolved](t, resolved[0]).Resolution)
	assert.Equal(t, rowsOf(res, ledger.KindTurnEnded)[0].At, resolved[0].At)
}

func TestConvertChat_UnfinishedPromptsAreSettledNeverReplayed(t *testing.T) {
	in := chatA()
	in.Prompts = append(in.Prompts,
		LegacyPrompt{RequestID: "req-f", Text: "refused", State: "failed", CreatedAt: at(10, 21, 0), UpdatedAt: at(10, 21, 1)},
		LegacyPrompt{RequestID: "req-s", Text: "retired", State: "settled", CreatedAt: at(10, 22, 0), UpdatedAt: at(10, 22, 1)},
		LegacyPrompt{RequestID: "req-a", Text: "no turn", State: promptAccepted, CreatedAt: at(10, 23, 0)},
	)

	res := convert(t, in, nobody)

	assert.Equal(t, []ledger.PromptSettled{
		{RequestID: "req-9", Reason: ledger.ReasonDaemonRestart},
		{RequestID: "req-f", Reason: ledger.ReasonDeliveryFailed},
		{RequestID: "req-s", Reason: ledger.ReasonNotAcknowledged},
	}, bodies[ledger.PromptSettled](t, res, ledger.KindPromptSettled))
	st := fold(res)
	assert.Empty(t, st.PendingPrompts())
	lost, ok := st.Receipt("req-9")
	require.True(t, ok)
	assert.Equal(t, "lost prompt", lost.Text, "the transcript still shows it")
	assert.Equal(t, ledger.PromptStatusSettled, lost.Status)
	accepted, _ := st.Receipt("req-1")
	assert.Equal(t, ledger.PromptStatusDelivered, accepted.Status, "an accepted prompt is its turn")
	assert.Contains(t, res.Skipped, "prompt req-a: accepted, and no turn holds it")
}

// A runner the old daemon never saw exit is left live for the boot reconcile to
// end; nothing else of the chat stays open.
func TestConvertChat_ALegacyLiveRunnerIsAdopted(t *testing.T) {
	in := chatB()
	in.Runners = []LegacyRunner{{ID: "run-b", ChatID: in.ChatID, ProviderID: "codex"}}

	res := convert(t, in, nobody)

	last := res.Rows[len(res.Rows)-1]
	assert.Equal(t, ledger.RunnerStarted{RunnerID: "run-b", Provider: "codex"}, body[ledger.RunnerStarted](t, last))
	st := fold(res)
	require.NoError(t, AtRest(st, map[string]bool{"run-b": true}))
	require.ErrorIs(t, AtRest(st, nil), ErrNotAtRest)
	assert.True(t, st.HasOpenWork())
}

// A record the old daemon wrote with JSON the reader cannot decode keeps the
// rest of its row and is named as skipped; an unknown kind is named too.
func TestConvertChat_UnreadableColumnsAndUnknownKindsAreNamed(t *testing.T) {
	in := chatA()
	in.Tools[0].Locations = "{broken"
	in.Subagents[0].Messages = "{broken"
	in.Turns[1].Plan = "{broken"
	in.Choices[0].AnsweredOptionIDs = "{broken"
	in.Choices[0].Kind = choiceElicit
	in.Interruptions = append(in.Interruptions, LegacyInterruption{ID: "i-9", ChatID: in.ChatID, Kind: "weather", At: at(10, 9, 0)})
	in.Prompts = append(in.Prompts, LegacyPrompt{RequestID: "req-q", State: "queued", CreatedAt: at(10, 25, 0)})

	res := convert(t, in, nobody)

	for _, prefix := range []string{
		"tool t-1: locations unreadable", "subagent sa-1: messages unreadable", "turn req-1: plan unreadable",
		"choice ch-1: answered options unreadable",
	} {
		assert.True(t, slices.ContainsFunc(res.Skipped, func(s string) bool { return strings.HasPrefix(s, prefix) }), prefix)
	}
	assert.Contains(t, res.Skipped, `interruption i-9: unknown kind "weather"`)
	assert.Contains(t, res.Skipped, "prompt req-q: unknown state queued")
	assert.Equal(t, kindElicitation, bodies[ledger.Ask](t, res, ledger.KindAsk)[0].Event)
	assert.Len(t, rowsOf(res, ledger.KindToolCall), 4, "the calls are still restated")
}

func TestAtRest_NamesTheWorkLeftOpen(t *testing.T) {
	row := func(kind ledger.Kind, turnID, scope string, b any) ledger.Row {
		r, err := ledger.NewRow(kind, turnID, scope, b)
		require.NoError(t, err)
		return r
	}
	for name, rows := range map[string][]ledger.Row{
		"turn":     {row(ledger.KindTurnStarted, "T1", "", ledger.TurnStarted{})},
		"prompt":   {row(ledger.KindUserMessage, "T1", "", ledger.UserMessage{RequestID: "r1", Origin: ledger.OriginCrowbar})},
		"subagent": {row(ledger.KindSubagent, "T1", "sa", ledger.SubagentBody{Phase: ledger.SubagentStart})},
	} {
		for i := range rows {
			rows[i].Seq = int64(i + 1)
		}
		err := AtRest(ledger.Fold("c", rows), nil)
		require.ErrorIs(t, err, ErrNotAtRest, name)
		assert.Contains(t, err.Error(), name)
	}
}
