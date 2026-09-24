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
// It scans THREE timestamped sources and takes the newest, because all three are
// projections of the same runner activity and their timestamps are directly
// comparable:
//
//   - conversations — the chat's conversation history, or just its single
//     most-recently-active row when the caller already has that from a store's
//     own LastConversation. Written only by a provider that ANNOUNCES a
//     conversation.
//   - interruptions — the chat's interruption ledger, of which only
//     InterruptProviderSwitched entries are read, Detail carrying the target
//     provider id (see turn.Turns.RecordChatSwitch). This is a provider's only
//     trace when it binds via its OWN connection identity and so writes no
//     conversation row — but only if the chat was ever SWITCHED to it.
//   - placements — the chat's placement history: every provider a runner has been
//     pointed at this chat on, stamped with when it arrived. Recorded for EVERY
//     runner at the moment it is placed, so it is the only source that also
//     covers a chat BORN on a provider that announces nothing and never switched.
//
// Whichever source is NEWER is who was actually running when the chat went
// dormant. Ties go to the earlier-listed source, so adding placements can only
// change an answer when a provider demonstrably ARRIVED on the chat later than
// anything the other two know about — which is the correct answer by definition,
// since a placement is Crowbar's own record of pointing a CLI at the chat.
//
// conversations is scanned for the max LastActiveAt rather than trusting slice
// order: a chat switched back to a provider it already ran re-activates that
// provider's own EARLIER row, whose position in a history slice never moves even
// though it is once again current. placements is scanned the same way and for the
// same reason.
//
// found is false only when all three sources are empty — no runner has ever been
// placed on this chat at all.
func ActiveProviderID(
	conversations []ChatConversation,
	interruptions []domain.ActivityInterruption,
	placements []ChatPlacement,
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
	for _, p := range placements {
		if !found || p.LastPlacedAt.After(newest) {
			providerID, newest, found = p.ProviderID, p.LastPlacedAt, true
		}
	}
	return providerID, found
}

// ResolveProviderID is the WHOLE dormant-side answer, and what every caller
// asking "as whom does this chat come back" should use: ActiveProviderID's scan
// of the three runner projections, else stored — the chat's own durable choice
// (domain.Chat.ProviderID).
//
// stored is LAST on purpose. The scan is the incumbent, with three live-reported
// orderings baked into it, so it keeps answering everything it can already
// answer; stored only has to cover a chat with no runner history whatsoever —
// one minted but never started, which is the single case the scan cannot see.
//
// Placement history is what makes the scan answer for a chat BORN on a provider
// that binds by its own connection identity: no conversation row, and no switch
// marker because it was never switched. That gap is what made the resolvers
// answer "no provider has ever run here" for a perfectly ordinary chat, and what
// the UI then resolved by starting whichever provider happened to be first in the
// catalogue. stored closes the same gap for every chat created from now on
// (chat.ProviderID is written at BIRTH); placements close it for every chat that
// already exists, because the daemon has been recording them all along.
//
// found is false only when a chat has never had a CLI placed on it and carries
// no durable provider either, so "nothing has ever run here" stays a
// distinguishable answer rather than a guess.
func ResolveProviderID(
	conversations []ChatConversation,
	interruptions []domain.ActivityInterruption,
	placements []ChatPlacement,
	stored string,
) (providerID string, found bool) {
	if providerID, found := ActiveProviderID(conversations, interruptions, placements); found {
		return providerID, true
	}
	return stored, stored != ""
}
