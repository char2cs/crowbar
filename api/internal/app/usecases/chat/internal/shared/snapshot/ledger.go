package snapshot

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

type LedgerFacts struct {
	Conversations []ledger.Conversation
	Current       map[string]ledger.Conversation
	LastExit      *ledger.RunnerExit
	Busy          bool
	PendingMove   *ledger.Move
}

func FactsOf(st ledger.State) LedgerFacts {
	facts := LedgerFacts{Conversations: st.Conversations(), Current: map[string]ledger.Conversation{}, Busy: st.Busy()}
	for _, c := range facts.Conversations {
		facts.Current[c.Provider], _ = st.Conversation(c.Provider)
	}
	if m, ok := st.PendingMove(); ok {
		facts.PendingMove = &m
	}
	if _, live := st.LiveRunner(); !live {
		if exit, ok := st.LastExit(); ok {
			facts.LastExit = &exit
		}
	}
	return facts
}

func (s *Snapshots) ApplyLedger(ctx context.Context, chatID string, facts LedgerFacts, moved bool) (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, gone := s.deleted[chatID]; gone {
		return false, ""
	}
	st := s.stateLocked(chatID)
	flipped := st.facts.Busy != facts.Busy
	changed := flipped || !sameConversations(st.facts.Conversations, facts.Conversations) ||
		!sameExit(st.facts.LastExit, facts.LastExit) || pendingMoveOf(st.facts) != pendingMoveOf(facts)
	st.facts, st.hasFacts = facts, true
	if moved || changed {
		s.emitChatLocked(ctx, chatID)
	}
	return flipped, st.chat.WorkspaceID
}

func (snap Snapshot) Conversation() (ledger.Conversation, bool) {
	provider := snap.Chat.ProviderID
	if snap.Live != nil {
		provider = snap.Live.ProviderID
	}
	c, ok := snap.Ledger.Current[provider]
	return c, ok
}

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
		if errors.Is(err, ledger.ErrUnreadableRow) {
			slog.ErrorContext(ctx, "agent: chat snapshot: ledger unreadable", "chat_id", chatID, "err", err)
			s.installFacts(chatID, LedgerFacts{})
			continue
		}
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

func pendingMoveOf(f LedgerFacts) string {
	if f.PendingMove == nil {
		return ""
	}
	return f.PendingMove.RequestID
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
