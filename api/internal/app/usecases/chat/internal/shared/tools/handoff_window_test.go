package tools_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
)

func rowsForTest(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("turn-%d", i)
	}
	return out
}

func TestRecentHandoffWindow_EmptyInputIsUnchangedAndUnnoted(t *testing.T) {
	kept, note := tools.RecentHandoffWindow("chat-1", []string(nil))

	require.Empty(t, kept)
	require.Empty(t, note)
}

// A gap that never reaches the cap must cost nothing beyond what it already
// cost: the SAME slice, no note text — byte-identical to the un-capped
// behaviour this window replaces.
func TestRecentHandoffWindow_UnderTheCapIsUnchanged(t *testing.T) {
	rows := rowsForTest(tools.DefaultChatLogTurnsForTest - 1)

	kept, note := tools.RecentHandoffWindow("chat-1", rows)

	require.Equal(t, rows, kept)
	require.Empty(t, note)
}

// The boundary itself must not count as "cut" — exactly the cap is still the
// whole conversation.
func TestRecentHandoffWindow_ExactlyAtTheCapIsUnchanged(t *testing.T) {
	rows := rowsForTest(tools.DefaultChatLogTurnsForTest)

	kept, note := tools.RecentHandoffWindow("chat-1", rows)

	require.Equal(t, rows, kept)
	require.Empty(t, note)
}

func TestRecentHandoffWindow_OverTheCapKeepsOnlyTheMostRecent(t *testing.T) {
	total := tools.DefaultChatLogTurnsForTest + 7
	rows := rowsForTest(total)

	kept, note := tools.RecentHandoffWindow("the-chat-id", rows)

	require.Equal(t, rows[total-tools.DefaultChatLogTurnsForTest:], kept,
		"the trimmed end must be the OLDEST rows, not the newest")
	require.NotEmpty(t, note)
	require.Contains(t, note, fmt.Sprintf("%d", tools.DefaultChatLogTurnsForTest), "shown count")
	require.Contains(t, note, fmt.Sprintf("%d", total), "total")
	require.Contains(t, note, fmt.Sprintf("%d", total-tools.DefaultChatLogTurnsForTest), "omitted count")
	require.Contains(t, note, "get_chat_log")
	require.Contains(t, note, "the-chat-id")
}
