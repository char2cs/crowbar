package commands

import (
	"github.com/char2cs/crowbar/api/internal/domain"
)

// foldWorking is the ONE definition of domain.Chat.Working, kept in one
// place so the commands that touch live state cannot drift apart on what "busy"
// means: a chat is working while a turn is open OR while the CLI's own last
// report said async work was still outstanding.
//
// Callers set the inputs (CurrentTurnStarted, AsyncWork) and then ask this for
// the answer; none of them decides Working for itself. The FRONTEND does not
// re-implement it either — it reads the answer off the wire frame, because a
// second copy of this rule in TypeScript is a second copy that can disagree, and
// did.
func foldWorking(next *domain.Chat) bool {
	return next.CurrentTurnStarted != nil || next.AsyncWork > 0
}
