package conversation_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func handoffCap(t *testing.T) int {
	t.Helper()
	_, _, kept := agenttools.HandoffWindow("probe", make([]struct{}, 100_000), -1)
	return len(kept)
}

func recordNumberedTurns(t *testing.T, f fixture, chat domain.Chat, prefix string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		say(t, f, chat.ID, fmt.Sprintf("%s-%d", prefix, i))
	}
}

func TestAssembleConversation_UnderTheCapCarriesEveryTurnAndNoNote(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "", "ws-1", "", "")
	require.NoError(t, err)
	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	under := handoffCap(t) - 1
	recordNumberedTurns(t, f, chat, "turn", under)

	blob, err := f.conversations.AssembleHandoff(t.Context(), chatID)

	require.NoError(t, err)
	assert.NotContains(t, blob, "get_chat_log", "an untrimmed handoff must carry no truncation note")
	for i := 0; i < under; i++ {
		assert.Contains(t, blob, fmt.Sprintf("turn-%d", i))
	}
}

func TestAssembleConversation_FreshJoinOverTheCapKeepsTheFirstAskAndTheMostRecent(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "", "ws-1", "", "")
	require.NoError(t, err)
	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	total := handoffCap(t) + 5
	recordNumberedTurns(t, f, chat, "turn", total)

	blob, err := f.conversations.AssembleConversation(t.Context(), chatID, false, time.Time{})

	require.NoError(t, err)
	first := strings.Index(blob, "turn-0\n")
	note := strings.Index(blob, "earlier turns are omitted")
	newest := strings.Index(blob, fmt.Sprintf("turn-%d\n", total-1))
	assert.True(t, first >= 0 && first < note && note < newest,
		"first ask, then the omission note, then the newest turn:\n%s", blob)
	assert.NotContains(t, blob, "turn-1\n", "the turns after the first ask are the ones trimmed")
}

func TestAssembleConversation_ResumeGapOverTheCapKeepsOnlyTheMostRecentOfTheGap(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "", "ws-1", "", "")
	require.NoError(t, err)
	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)

	recordNumberedTurns(t, f, chat, "before", 3)
	turnsBefore, err := f.conversations.ChatTurns(t.Context(), chatID)
	require.NoError(t, err)
	leftAt := turnsBefore[len(turnsBefore)-1].At

	total := handoffCap(t) + 5
	recordNumberedTurns(t, f, chat, "gap", total)

	blob, err := f.conversations.AssembleConversation(t.Context(), chatID, true, leftAt)

	require.NoError(t, err)
	assert.NotContains(t, blob, "before-", "turns from before the gap must never appear in a resume handoff")
	assert.NotContains(t, blob, "gap-0\n",
		"the oldest turn OF THE GAP must be trimmed once the gap itself exceeds the cap")
	assert.Contains(t, blob, fmt.Sprintf("gap-%d", total-1), "the newest gap turn must survive")
	assert.Contains(t, blob, "get_chat_log")
}

func TestAssembleConversation_ACarriesNothingFromBeforeTheConversationWasCleared(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "", "ws-1", "", "")
	require.NoError(t, err)
	say(t, f, chatID, "before-clear")
	st, err := f.ledger.State(t.Context(), chatID)
	require.NoError(t, err)
	cleared, err := ledger.AnnounceCleared(st, "claude", "S2")
	require.NoError(t, err)
	_, _, err = f.ledger.Append(t.Context(), chatID, cleared...)
	require.NoError(t, err)
	say(t, f, chatID, "after-clear")

	blob, err := f.conversations.AssembleHandoff(t.Context(), chatID)

	require.NoError(t, err)
	assert.NotContains(t, blob, "before-clear")
	assert.Contains(t, blob, "after-clear")
}
