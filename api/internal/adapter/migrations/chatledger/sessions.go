package chatledger

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// bindSessions binds every conversation of each provider when it began, so
// the turns on it mark it turned, then binds the current one again last so
// the chat resumes it. A session another chat owns is a conflict marker.
func (c *conversion) bindSessions(in LegacyChat, owner Owner) {
	c.current, c.bound = map[string]string{}, map[string]bool{}
	byProvider := map[string][]LegacyConversation{}
	for _, conv := range in.Conversations {
		if conv.ProviderID != "" && conv.SessionID != "" {
			byProvider[conv.ProviderID] = append(byProvider[conv.ProviderID], conv)
		}
	}
	for _, provider := range slices.Sorted(maps.Keys(byProvider)) {
		convs := byProvider[provider]
		current := currentSession(convs, in.Turns)
		bindAt := map[string]time.Time{}
		for _, conv := range convs {
			bindAt[conv.SessionID] = earliest(conv.FirstSeenAt, conv.LastActiveAt, firstTurnOn(in.Turns, provider, conv.SessionID, current))
		}
		slices.SortFunc(convs, func(a, b LegacyConversation) int {
			return cmp.Or(bindAt[a.SessionID].Compare(bindAt[b.SessionID]), cmp.Compare(a.SessionID, b.SessionID))
		})
		for _, conv := range convs {
			c.bind(provider, conv.SessionID, bindAt[conv.SessionID], owner, conv.SessionID == current)
		}
		if last := convs[len(convs)-1]; last.SessionID != current && c.current[provider] == current {
			c.bind(provider, current, bindAt[last.SessionID], owner, true)
		}
	}
}

func (c *conversion) bind(provider, sessionID string, at time.Time, owner Owner, current bool) {
	if other, ok := owner(provider, sessionID); ok && other != c.chatID {
		c.emit(ruleSession, at, 0, ledger.KindMarker, "", "", ledger.Marker{Kind: ledger.MarkerSessionConflict, Detail: sessionID})
		return
	}
	c.emit(ruleSession, at, 0, ledger.KindSessionBound, "", "",
		ledger.SessionBound{Provider: provider, SessionID: sessionID, Cause: ledger.CauseAnnounced})
	c.bound[provider+"\x00"+sessionID] = true
	if current {
		c.current[provider] = sessionID
	}
}

// currentSession is the conversation whose newest turn is the latest; with no
// turn naming any of them, the one last active.
func currentSession(convs []LegacyConversation, turns []LegacyTurn) string {
	known := map[string]bool{}
	for _, conv := range convs {
		known[conv.SessionID] = true
	}
	var newest *LegacyTurn
	for i, t := range turns {
		if t.ProviderID != convs[0].ProviderID || !known[t.SessionID] {
			continue
		}
		if newest == nil || t.StartedAt.After(newest.StartedAt) {
			newest = &turns[i]
		}
	}
	if newest != nil {
		return newest.SessionID
	}
	last := slices.MaxFunc(convs, func(a, b LegacyConversation) int {
		return cmp.Or(a.LastActiveAt.Compare(b.LastActiveAt), cmp.Compare(a.SessionID, b.SessionID))
	})
	return last.SessionID
}

// firstTurnOn is when the first turn on provider's sessionID began: one that
// names it, or for the current session one that names no session.
func firstTurnOn(turns []LegacyTurn, provider, sessionID, current string) time.Time {
	var out time.Time
	for _, t := range turns {
		on := t.SessionID == sessionID || t.SessionID == "" && sessionID == current
		if t.ProviderID == provider && on {
			out = earliest(out, t.StartedAt)
		}
	}
	return out
}

// sessionOf is the bound session a legacy turn ran on: the one it names, or
// its provider's current one when it names none.
func (c *conversion) sessionOf(provider, sessionID string) string {
	if sessionID == "" {
		return c.current[provider]
	}
	if c.bound[provider+"\x00"+sessionID] {
		return sessionID
	}
	return ""
}
