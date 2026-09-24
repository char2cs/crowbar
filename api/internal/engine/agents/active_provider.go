package agents

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ActiveProviderID is THE single answer to "which provider is this chat running as
// right now" once no LIVE runner is placed on it — the caller decides that part
// first (a live runner always outranks history, since mid-switch the incoming
// runner is already the truth) and calls this only for the dormant fallback.
//
// conversations is the chat's conversation history — or just its single
// most-recently-active row, when the caller already has that from a store's own
// LastConversation — and interruptions is its interruption ledger, of which only
// InterruptProviderSwitched entries are read, Detail carrying the target provider
// id (see turn.Turns.RecordChatSwitch). Both sources exist because a provider that
// binds via its OWN connection identity, rather than firing a session-bind, never
// writes a conversation row at all: its activity is visible ONLY through the switch
// interruption. Whichever source is NEWER is who was actually running when the chat
// went dormant.
//
// conversations is scanned for the max LastActiveAt rather than trusting slice
// order: a chat switched back to a provider it already ran re-activates that
// provider's own EARLIER row, whose position in a history slice never moves even
// though it is once again current.
//
// found is false only when neither source has anything at all — no provider has
// ever run on this chat.
func ActiveProviderID(
	conversations []ChatConversation,
	interruptions []domain.ActivityInterruption,
) (providerID string, found bool) {
	var newest time.Time
	for _, c := range conversations {
		if !found || c.LastActiveAt.After(newest) {
			providerID, newest, found = c.ProviderID, c.LastActiveAt, true
		}
	}
	for _, i := range interruptions {
		if i.Kind != InterruptProviderSwitched {
			continue
		}
		if !found || i.At.After(newest) {
			providerID, newest, found = i.Detail, i.At, true
		}
	}
	return providerID, found
}

// ResolveProviderID is the WHOLE dormant-side answer, and what every caller
// asking "as whom does this chat come back" should use: ActiveProviderID's scan
// of the two runner projections, else stored — the chat's own durable choice
// (domain.Chat.ProviderID).
//
// stored is LAST on purpose. The scan is the incumbent, with two live-reported
// orderings baked into it, so it keeps answering everything it can already
// answer; stored only has to cover what BOTH projections are blind to, which is
// every chat BORN on a provider that binds by its own connection identity: no
// conversation row, and no switch marker because it was never switched. That
// gap is what made the resolvers answer "no provider has ever run here" for a
// perfectly ordinary codex chat, and what the UI then resolved by starting
// whichever provider happened to be first in the catalogue.
//
// A chat minted before that field carries "" and is still reported not-found,
// so "nothing has ever run here" stays a distinguishable answer.
func ResolveProviderID(
	conversations []ChatConversation,
	interruptions []domain.ActivityInterruption,
	stored string,
) (providerID string, found bool) {
	if providerID, found := ActiveProviderID(conversations, interruptions); found {
		return providerID, true
	}
	return stored, stored != ""
}
