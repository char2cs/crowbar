package move

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"

func Decide(
	currentSession string,
	announcedSession string,
	knownChatID string,
	known bool,
) models.Decision {
	switch {
	case announcedSession == currentSession:
		return models.Decision{Kind: models.MoveNoop}
	case currentSession == "":
		return models.Decision{Kind: models.MoveBind}
	case known:
		return models.Decision{Kind: models.MoveToKnown, ChatID: knownChatID}
	default:
		return models.Decision{Kind: models.MoveToNew}
	}
}
