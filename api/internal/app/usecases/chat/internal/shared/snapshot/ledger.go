package snapshot

import (
	"context"
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// LedgerFacts are the ledger-derived parts of a chat snapshot.
type LedgerFacts struct {
	Conversations []ledger.Conversation
	// Current is each provider's conversation now; it changes only with Conversations.
	Current map[string]ledger.Conversation
	// LastExit is how the chat's last runner ended; nil while one is live.
	LastExit *ledger.RunnerExit
	// Busy is whether the chat is working: ledger.State.Busy.
	Busy bool
}

// FactsOf is what a ledger State contributes to its chat's snapshot.
func FactsOf(st ledger.State) LedgerFacts {
	facts := LedgerFacts{Conversations: st.Conversations(), Current: map[string]ledger.Conversation{}, Busy: st.Busy()}
	for _, c := range facts.Conversations {
		facts.Current[c.Provider], _ = st.Conversation(c.Provider)
	}
	if _, live := st.LiveRunner(); !live {
		if exit, ok := st.LastExit(); ok {
			facts.LastExit = &exit
		}
	}
	return facts
}

// ApplyLedger installs facts published by the chat's ledger writer, bumps the
// chat's version and emits a snapshot frame, of kind when one is named, else
// when the facts changed. It runs under that writer, so it never reads the
// ledger itself. It reports whether Busy flipped, and the chat's workspace.
func (s *Snapshots) ApplyLedger(ctx context.Context, chatID string, facts LedgerFacts, kind string) (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, gone := s.deleted[chatID]; gone {
		return false, ""
	}
	st := s.stateLocked(chatID)
	flipped := st.facts.Busy != facts.Busy
	changed := flipped || !sameConversations(st.facts.Conversations, facts.Conversations) ||
		!sameExit(st.facts.LastExit, facts.LastExit)
	st.facts, st.hasFacts = facts, true
	if kind == "" && changed {
		kind = KindSnapshot
	}
	if kind != "" {
		s.emitChatLocked(ctx, chatID, kind, "")
	}
	return flipped, st.chat.WorkspaceID
}

// Conversation is the chat's conversation with its current provider: the live
// runner's, else the chat's own.
func (snap Snapshot) Conversation() (ledger.Conversation, bool) {
	provider := snap.Chat.ProviderID
	if snap.Live != nil {
		provider = snap.Live.ProviderID
	}
	c, ok := snap.Ledger.Current[provider]
	return c, ok
}

// preloadLedger fills the facts of every chat in chatIDs that has none under
// the chat's ledger writer, so no publication lands between read and install.
func (s *Snapshots) preloadLedger(ctx context.Context, chatIDs []string) error {
	s.mu.Lock()
	reader := s.reader
	var missing []string
	for _, chatID := range chatIDs {
		_, gone := s.deleted[chatID]
		if st := s.chats[chatID]; !gone && (st == nil || !st.hasFacts) {
			missing = append(missing, chatID)
		}
	}
	s.mu.Unlock()
	if reader == nil {
		return nil
	}
	for _, chatID := range missing {
		err := reader.ViewLedger(ctx, chatID, func(st ledger.State) error {
			s.installFacts(chatID, FactsOf(st))
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Snapshots) installFacts(chatID string, facts LedgerFacts) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, gone := s.deleted[chatID]; gone {
		return
	}
	if st := s.stateLocked(chatID); !st.hasFacts {
		st.facts, st.hasFacts = facts, true
	}
}

func sameExit(a, b *ledger.RunnerExit) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.RunnerID == b.RunnerID && a.Reason == b.Reason && a.ExitCode == b.ExitCode && a.At.Equal(b.At)
}

func sameConversations(a, b []ledger.Conversation) bool {
	return slices.EqualFunc(a, b, func(x, y ledger.Conversation) bool {
		return x.Provider == y.Provider && x.SessionID == y.SessionID &&
			x.State == y.State && x.BoundAt.Equal(y.BoundAt)
	})
}
