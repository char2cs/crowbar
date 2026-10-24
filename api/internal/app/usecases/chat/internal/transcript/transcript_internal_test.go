package transcript

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// One chat's publication never waits on another chat: while a read of chat a
// holds a's lock, chat b's rows are applied.
func TestProjector_AChatsApplyNeverWaitsOnAnotherChat(t *testing.T) {
	p := New(nil)
	opened, err := ledger.NewRow(ledger.KindTurnStarted, "T1", "", ledger.TurnStarted{RunnerID: "run-1"})
	require.NoError(t, err)
	opened.Seq = 1
	for _, chatID := range []string{"a", "b"} {
		p.install(chatID, []ledger.Row{opened})
	}
	a, ok := p.entry("a")
	require.True(t, ok)
	a.mu.Lock()
	defer a.mu.Unlock()

	said, err := ledger.NewRow(ledger.KindAssistantText, "T1", "", ledger.AssistantText{
		MessageID: "m1", Index: ledger.IndexComplete, Text: "said",
	})
	require.NoError(t, err)
	said.Seq = 2
	changed := p.Apply("b", []ledger.Row{said})

	require.Len(t, changed, 1)
	require.Equal(t, "said", changed[0].Text)
}
