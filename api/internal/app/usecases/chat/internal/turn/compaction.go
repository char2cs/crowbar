package turn

import (
	"context"
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

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
// armCompaction records turnID as the one turn_stop that belongs to a compaction
// and must therefore be skipped — but ONLY when this chat has no turn of its own
// in flight.
//
// A STANDALONE compaction (the compact button, or codex's own disconnected
// companion PTY) gets a turn envelope to itself, and skipping that envelope's
// stop is the whole point of the latch.
//
// An AUTOMATIC one does not. It fires INSIDE the user's own turn — codex compacts
// to make room to CARRY ON — and codex maps turn_id from the wrapping envelope,
// which is that user turn. Arming on it therefore armed the REAL turn's id, and
// the real turn's own turn/completed was then swallowed as if it were the
// compaction's. Measured live: nothing closed the turn, and the 5s
// provider-idle sweep abandoned it five seconds later instead — logged as
// "closed a turn whose message was cut off", the reply recorded as a partial.
func (t *Turns) armCompaction(chatID, turnID string) {
	if len(t.turns.Inflight(chatID)) > 0 {
		return
	}
	t.compacting.arm(chatID, turnID)
}

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

// manualCompactRequests remembers which chat's NEXT compaction round trip was
// asked for by Crowbar itself — the compact button — for a provider whose own
// wire event carries no trigger at all. codex's contextCompaction item is
// {id, type} only (see compact_pre's own comment in codex.yaml); its canonical
// `trigger` field is therefore always empty, and "auto or manual" would
// otherwise be unanswerable for it. claude needs none of this: its PreCompact
// hook reports a real trigger, mapped straight through.
//
// Armed by chat.Usecase.Compact, the one call site that unambiguously knows a
// compaction was just requested by a person, right before it hands off to
// Runners.Compact. Read by BOTH compact_pre (peek) and compact_post (consume)
// — see peek's own doc for why compact_post, not just compact_pre, needs it
// too: it is a genuine surprise, not a guess, and cost a live repro to find.
//
// One flag per chat, like compactionTurns above. A request that fails
// (Runners.Compact validates the provider, the live connection, etc. AFTER
// this arms) leaves the flag armed with no compact_post coming to consume
// it — the SAME imprecision compactionTurns itself already accepts, for the
// identical reason: nothing else knows any better, and the cost of getting it
// wrong is a later, genuinely automatic compaction's divider reading "manual"
// once, never a correctness bug in the ledger itself.
type manualCompactRequests struct {
	mu     sync.Mutex
	byChat map[string]bool
}

func newManualCompactRequests() *manualCompactRequests {
	return &manualCompactRequests{byChat: make(map[string]bool)}
}

func (m *manualCompactRequests) arm(chatID string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byChat[chatID] = true
}

// peek reports whether chatID's next compaction was Crowbar-requested,
// WITHOUT clearing the flag — compact_pre's own call, live-verified
// necessary: commands.Interrupt's idle-branch (compact_pre always hits it,
// since a bare /compact never opens a tracked turn) never adds the item to
// the aggregate's own open-interruptions map, so commands.ResolveInterruption
// — compact_post's call — always finds `known == false` and REBUILDS the
// interruption from scratch off ITS OWN (also-empty, for codex) detail,
// silently overwriting whatever compact_pre's Interrupt call durably wrote
// moments before. Confirmed live: a curl read caught "manual" in the ~1-2s
// window between the two, then read back empty once compact_post landed. If
// compact_pre alone consumed the flag, compact_post would rebuild with
// nothing to fall back on and clobber it right back to empty regardless.
func (m *manualCompactRequests) peek(chatID string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.byChat[chatID]
}

// consume is peek plus the clear — compact_post's own call, since it is the
// LAST word on the interruption's stored detail (see peek's own doc) and so
// is what actually closes this request out.
func (m *manualCompactRequests) consume(chatID string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	armed := m.byChat[chatID]
	delete(m.byChat, chatID)
	return armed
}

// ArmManualCompaction records that chatID's next compaction was requested by
// a person, not the provider's own automatic policy. See manualCompactRequests'
// own doc for the call site and its limits.
func (t *Turns) ArmManualCompaction(chatID string) { t.manualCompact.arm(chatID) }

// settleCompactDelivery retires this chat's pending prompt delivery ONLY when
// the provider's own compact_start rides the prompt-delivery wire (claude:
// compact_start dispatches "/compact" through compact.go's ordinary
// SubmitPrompt path, so the pending delivery genuinely IS the compaction
// request itself — see SettleDeliveryFor's own doc for why nothing else
// would ever settle it). For any other wire (codex: a direct api-transport
// call that never touches the prompt journal at all), settling here is not a
// harmless no-op — it is a live-reported bug: codex can decide to compact
// automatically, entirely on its own, BEFORE processing a message a person
// already submitted and is still genuinely waiting on. That message's own
// delivery is what SettleDeliveryFor would find pending, and retiring it
// makes the just-sent message vanish from the transcript for the whole
// compaction, reappearing only once the real turn finally opens and the
// ledger catches up — confirmed live (codex, high context usage, a fresh
// prompt that forced an automatic pre-turn compaction).
func (t *Turns) settleCompactDelivery(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	agent engineagents.Agent,
) {
	wire, _, ok := agent.OutboundCall("compact_start", nil)
	if !ok || wire != "prompt" {
		return
	}
	note(ctx, "settle delivery after compaction", t.runners.SettleDeliveryFor(ctx, chat.ID, runner.ID))
}
