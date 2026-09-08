package turn

import "sync"

// compactionTurns remembers which turn id belongs to a compact_start round
// trip rather than an ordinary assistant reply.
//
// It exists because a provider's turn-closing wire event can be a sum type
// with nothing on the frame itself telling the two apart. Confirmed live
// against codex-cli 0.149.1: thread/compact/start's own turn/started ..
// turn/completed wrapper rides the EXACT SAME turn/completed notification
// turn_stop and turn_failed already consume unconditionally, and its shape
// — items: [], itemsView: "notLoaded", status: "completed" — is not
// distinguishable from a genuinely INTERRUPTED real turn or a genuine turn
// that ran a tool but sent no final text: both of those produce the
// identical empty shape. Gating on itemsView or an empty items list would
// have suppressed those real turns' own close.
//
// What DOES tell them apart is the turn id, and compact_pre already knows it
// — the envelope turnId on the contextCompaction item/started codex.yaml
// maps compact_pre from. Arming it there and consuming it in
// closeTurnFromStop/closeTurnFromFailure keeps a compaction from minting a
// spurious empty turn_stopped event, or a spurious failure notice if the
// compaction itself fails, without either side having to know the other
// exists — the same latch-across-events shape idleLatch already uses.
//
// One id per chat, like idleLatch: a chat compacts one round trip at a time,
// and a later arm simply replaces whatever id is still sitting there
// unconsumed (a compaction whose wrapper turn never closed) rather than
// leaking a growing set.
type compactionTurns struct {
	mu     sync.Mutex
	byChat map[string]string
}

func newCompactionTurns() *compactionTurns {
	return &compactionTurns{byChat: make(map[string]string)}
}

// arm records the turn a compaction is running under. A blank turn id arms
// nothing — see consume's own doc comment for why that matters.
func (c *compactionTurns) arm(chatID, turnID string) {
	if c == nil || turnID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byChat[chatID] = turnID
}

// consume reports whether turnID is the chat's armed compaction turn, and
// clears it ONLY on a match — a miss must leave whatever is genuinely armed
// alone. That matters the instant two turns are ever in flight for one chat
// at once (an ordinary turn's own stop arriving while a different compaction
// is still mid-flight): a miss that deleted regardless would clear the real
// compaction's latch out from under it, and its own turn_stop — arriving
// later — would then read as an ordinary reply instead of being skipped,
// which is the exact bug this file exists to prevent.
//
// A BLANK turnID never matches, even against nothing armed: a provider that
// maps no turn_id onto turn_stop/turn_failed must not be treated as "asking
// about the same empty id" the map's own zero value would otherwise agree
// to, which would silently swallow every one of that provider's ordinary
// stops.
func (c *compactionTurns) consume(chatID, turnID string) bool {
	if c == nil || turnID == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	armed, ok := c.byChat[chatID]
	if !ok || armed != turnID {
		return false
	}
	delete(c.byChat, chatID)
	return true
}
