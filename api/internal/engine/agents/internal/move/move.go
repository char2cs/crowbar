// Package move holds the context-move reducer.
package move

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"

// Decide is the context-move reducer. It is a PURE function of exactly two facts
// and nothing else:
//
//  1. did the conversation id under this runner change?
//  2. is the new id one we already know?
//
// It deliberately takes NO `source` argument, and must never be changed to read
// one. Claude reports source=clear where Codex reports source=startup for the
// very same event, so any branch on that vocabulary is provider-specific and will
// break on the next CLI. Branching only on the two facts above is what keeps this
// provider-agnostic.
//
// Decide also never decides whether a move is ALLOWED — there is no reject
// outcome. By the time a hook fires, the CLI has already switched conversation;
// Crowbar cannot refuse it and cannot push the CLI back. Decide reconciles a fait
// accompli, it does not authorise one.
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
