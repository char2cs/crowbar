package chatledger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func TestConvertChat_BindsTheNewestSessionThatHasTurns(t *testing.T) {
	res := convert(t, chatA(), nobody)

	assert.Equal(t, []ledger.SessionBound{
		{Provider: "claude", SessionID: "S-old", Cause: ledger.CauseAnnounced},
		{Provider: "claude", SessionID: "S-new", Cause: ledger.CauseAnnounced},
		{Provider: "claude", SessionID: "S-old", Cause: ledger.CauseAnnounced},
	}, bodies[ledger.SessionBound](t, res, ledger.KindSessionBound), "each when it began, the current one again last")
	st := fold(res)
	current, ok := st.Conversation("claude")
	require.True(t, ok)
	assert.Equal(t, "S-old", current.SessionID, "S-new was active later, but no turn ran on it")
	assert.True(t, current.Resumable(), "its turns ran on it, so the next prompt resumes it")
	assert.Equal(t, []string{"S-old", "S-new"}, sessionsOf(st.Conversations()), "the history keeps both, oldest first")
}

func TestConvertChat_FallsBackToTheLastActiveConversation(t *testing.T) {
	in := chatB()
	in.Conversations = append(in.Conversations, LegacyConversation{
		ChatID: in.ChatID, ProviderID: "codex", SessionID: "S-b0", FirstSeenAt: at(8, 0, 0), LastActiveAt: at(9, 45, 0),
	})

	st := fold(convert(t, in, nobody))

	current, ok := st.Conversation("codex")
	require.True(t, ok)
	assert.Equal(t, "S-b0", current.SessionID, "no turn names a session, so the one last active is current")
	assert.True(t, current.Resumable(), "the turns that name no session ran on the current one")
}

func TestConvertChat_ASessionAnotherChatOwnsIsAConflictMarker(t *testing.T) {
	owner := func(provider, sessionID string) (string, bool) {
		return "chat-other", provider == "codex" && sessionID == "S-b"
	}

	res := convert(t, chatB(), owner)

	assert.Empty(t, rowsOf(res, ledger.KindSessionBound), "a conflict never reaches the session index")
	assert.Contains(t, bodies[ledger.Marker](t, res, ledger.KindMarker),
		ledger.Marker{Kind: ledger.MarkerSessionConflict, Detail: "S-b"})
	_, bound := fold(res).Conversation("codex")
	assert.False(t, bound)
	for _, started := range bodies[ledger.TurnStarted](t, res, ledger.KindTurnStarted) {
		assert.Empty(t, started.SessionID, "no turn claims the other chat's session")
	}
}

// A chat the session index already names as the owner is this chat itself: an
// import retried over its own claim binds as before.
func TestConvertChat_ASessionThisChatOwnsStaysBound(t *testing.T) {
	owner := func(string, string) (string, bool) { return "chat-b", true }

	res := convert(t, chatB(), owner)

	assert.Len(t, rowsOf(res, ledger.KindSessionBound), 1)
}

func sessionsOf(convs []ledger.Conversation) []string {
	var out []string
	for _, c := range convs {
		out = append(out, c.SessionID)
	}
	return out
}
