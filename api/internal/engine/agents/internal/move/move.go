package move

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"

// Decide places a conversation a CLI has announced.
//
// MoveToNew is an inference from ABSENCE: a conversation nobody has a record of
// is one the user opened themselves (/clear), so it gets a chat of its own. That
// inference is only sound while Crowbar knows every conversation Crowbar itself
// originates — crowbarOriginated is that knowledge. Without it, a session the
// api driver opened to recover a lost one read as a /clear and stole the user's
// message into a brand new chat. Confirmed live.
func Decide(
	currentSession string,
	announcedSession string,
	knownChatID string,
	known bool,
	crowbarOriginated bool,
) models.Decision {
	switch {
	case announcedSession == currentSession:
		return models.Decision{Kind: models.MoveNoop}
	case currentSession == "" || crowbarOriginated:
		return models.Decision{Kind: models.MoveBind}
	case known:
		return models.Decision{Kind: models.MoveToKnown, ChatID: knownChatID}
	default:
		return models.Decision{Kind: models.MoveToNew}
	}
}
