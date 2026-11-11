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

func TestHandoffWindow_EmptyInputIsUnchangedAndUnnoted(t *testing.T) {
	head, note, recent := tools.HandoffWindow("chat-1", []string(nil), -1)

	require.Empty(t, head)
	require.Empty(t, note)
	require.Empty(t, recent)
}

// Up to and including the cap, the whole list comes back as it was, unnoted:
// the boundary itself does not count as cut.
func TestHandoffWindow_AtOrUnderTheCapIsUnchanged(t *testing.T) {
	for _, n := range []int{tools.DefaultChatLogTurnsForTest - 1, tools.DefaultChatLogTurnsForTest} {
		rows := rowsForTest(n)

		head, note, recent := tools.HandoffWindow("chat-1", rows, 0)

		require.Empty(t, head)
		require.Empty(t, note)
		require.Equal(t, rows, recent)
	}
}

func TestHandoffWindow_OverTheCapUnpinnedKeepsOnlyTheMostRecent(t *testing.T) {
	limit := tools.DefaultChatLogTurnsForTest
	total := limit + 7
	rows := rowsForTest(total)

	head, note, recent := tools.HandoffWindow("the-chat-id", rows, -1)

	require.Empty(t, head)
	require.Equal(t, rows[total-limit:], recent, "the trimmed end must be the OLDEST rows")
	require.Contains(t, note, fmt.Sprintf("[%d earlier turns are omitted", total-limit))
	require.Contains(t, note, fmt.Sprintf("most recent %d of %d", limit, total))
	require.Contains(t, note, `get_chat_log with chatId="the-chat-id"`)
}

// A pinned ask outside the recent window leads, and costs one recent slot, so
// the handoff never grows past the cap.
func TestHandoffWindow_OverTheCapKeepsThePinnedAskFirst(t *testing.T) {
	limit := tools.DefaultChatLogTurnsForTest
	total := limit + 7
	rows := rowsForTest(total)

	head, note, recent := tools.HandoffWindow("chat-1", rows, 2)

	require.Equal(t, []string{"turn-2"}, head)
	require.Equal(t, rows[total-limit+1:], recent)
	require.Contains(t, note, fmt.Sprintf("[%d earlier turns are omitted", total-limit))
}

// A pin already inside the recent window is not repeated ahead of it.
func TestHandoffWindow_PinInsideTheRecentWindowIsNotRepeated(t *testing.T) {
	limit := tools.DefaultChatLogTurnsForTest
	total := limit + 7
	rows := rowsForTest(total)

	head, _, recent := tools.HandoffWindow("chat-1", rows, total-3)

	require.Empty(t, head)
	require.Equal(t, rows[total-limit:], recent)
}
