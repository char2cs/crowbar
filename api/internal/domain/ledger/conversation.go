package ledger

import (
	"slices"
	"time"
)

const (
	CauseAssigned  = "assigned"
	CauseAnnounced = "announced"
	CauseFresh     = "fresh"
	CauseMoved     = "moved"
	CauseCleared   = "cleared"
)

type SessionBound struct {
	Provider  string `json:"provider"`
	SessionID string `json:"sessionId"`
	Cause     string `json:"cause"`
	Previous  string `json:"previous,omitempty"`
	From      string `json:"from,omitempty"`
	RelayID   string `json:"relayId,omitempty"`
}

type SessionLost struct {
	Provider  string `json:"provider"`
	SessionID string `json:"sessionId"`
	Reason    string `json:"reason"`
	MovedTo   string `json:"movedTo,omitempty"`
}

const ReasonMovedAway = "resumed in another chat"

type ConversationState string

const (
	ConversationUnbound ConversationState = "unbound"
	ConversationBound   ConversationState = "bound"
	ConversationLost    ConversationState = "lost"
)

var conversationEdges = map[ConversationState][]ConversationState{
	ConversationUnbound: {ConversationBound},
	ConversationBound:   {ConversationLost, ConversationBound},
	ConversationLost:    {ConversationBound},
}

func (s ConversationState) Next(to ConversationState) (ConversationState, bool) {
	if slices.Contains(conversationEdges[s], to) {
		return to, true
	}
	return s, false
}

type Conversation struct {
	Provider, SessionID string
	State               ConversationState
	BoundAt             time.Time
	Cause               string
	RelayID             string
	Turned              bool
}

func (c Conversation) Resumable() bool {
	return c.State == ConversationBound && c.Turned
}

func (s State) Conversation(provider string) (Conversation, bool) {
	i, ok := s.current[provider]
	if !ok {
		return Conversation{}, false
	}
	return s.conversations[i], true
}

func (s State) RelayConversation(relayID string) (Conversation, bool) {
	if relayID == "" {
		return Conversation{}, false
	}
	for _, i := range s.current {
		if c := s.conversations[i]; c.State == ConversationBound && c.RelayID == relayID {
			return c, true
		}
	}
	return Conversation{}, false
}

func (s State) ClearedAt() time.Time { return s.clearedAt }

func (s State) Conversations() []Conversation {
	return slices.Clone(s.conversations)
}

func (s *State) applySessionBound(r Row) {
	body, err := DecodeBody[SessionBound](r)
	if err != nil {
		return
	}
	if i, ok := s.current[body.Provider]; ok && s.conversations[i].State == ConversationBound {
		s.conversations[i].State = ConversationUnbound
	}
	i := slices.IndexFunc(s.conversations, func(c Conversation) bool {
		return c.Provider == body.Provider && c.SessionID == body.SessionID
	})
	if i < 0 {
		s.conversations = append(s.conversations, Conversation{Provider: body.Provider, SessionID: body.SessionID})
		i = len(s.conversations) - 1
	}
	s.conversations[i].State, s.conversations[i].BoundAt, s.conversations[i].Cause = ConversationBound, r.At, body.Cause
	s.conversations[i].RelayID = body.RelayID
	if body.Cause == CauseCleared {
		s.clearedAt = r.At
	}
	if s.current == nil {
		s.current = map[string]int{}
	}
	s.current[body.Provider] = i
}

func (s *State) markTurned(provider, sessionID string) {
	i := slices.IndexFunc(s.conversations, func(c Conversation) bool {
		return c.Provider == provider && c.SessionID == sessionID
	})
	if sessionID != "" && i >= 0 {
		s.conversations[i].Turned = true
	}
}

func (s *State) applySessionLost(r Row) {
	body, err := DecodeBody[SessionLost](r)
	if err != nil {
		return
	}
	i, ok := s.current[body.Provider]
	if !ok || s.conversations[i].SessionID != body.SessionID {
		return
	}
	s.conversations[i].State, _ = s.conversations[i].State.Next(ConversationLost)
}

func Assign(st State, provider, sessionID string) (Row, error) {
	body := SessionBound{Provider: provider, SessionID: sessionID, Cause: CauseAssigned}
	if c, ok := st.Conversation(provider); ok {
		body.Previous = c.SessionID
		if c.State == ConversationLost || (c.Cause == CauseFresh && !c.Turned) {
			body.Cause = CauseFresh
		}
	}
	return NewRow(KindSessionBound, "", "", body)
}

func Announce(st State, provider, sessionID, owner, relayID string) ([]Row, error) {
	return announce(st, provider, sessionID, owner, relayID, CauseAnnounced)
}

func AnnounceCleared(st State, provider, sessionID string) ([]Row, error) {
	return announce(st, provider, sessionID, "", "", CauseCleared)
}

func announce(st State, provider, sessionID, owner, relayID, cause string) ([]Row, error) {
	c, ok := st.Conversation(provider)
	if ok && c.SessionID == sessionID && c.State == ConversationBound {
		return nil, nil
	}
	holds := ok && c.Resumable()
	if owner != "" && holds {
		r, err := NewRow(KindMarker, "", "", Marker{Kind: MarkerSessionConflict, Detail: sessionID, Owner: owner})
		return []Row{r}, err
	}
	bound := SessionBound{Provider: provider, SessionID: sessionID, Cause: cause, RelayID: relayID}
	if owner != "" {
		bound.Cause, bound.From = CauseMoved, owner
	}
	if ok && c.SessionID != sessionID {
		bound.Previous = c.SessionID
	}
	r, err := NewRow(KindSessionBound, "", "", bound)
	if err != nil || !holds {
		return []Row{r}, err
	}
	m, err := NewRow(KindMarker, "", "", Marker{Kind: MarkerSessionChanged, Detail: sessionID})
	return []Row{r, m}, err
}

func Lose(st State, provider, sessionID, reason, runnerID string) ([]Row, error) {
	return lose(st, SessionLost{Provider: provider, SessionID: sessionID, Reason: reason}, runnerID)
}

func MoveAway(st State, provider, sessionID, toChat, runnerID string) ([]Row, error) {
	return lose(st, SessionLost{Provider: provider, SessionID: sessionID, Reason: ReasonMovedAway, MovedTo: toChat}, runnerID)
}

func lose(st State, body SessionLost, runnerID string) ([]Row, error) {
	if c, ok := st.Conversation(body.Provider); !ok || c.SessionID != body.SessionID || c.State != ConversationBound {
		return nil, nil
	}
	lost, err := NewRow(KindSessionLost, "", "", body)
	if err != nil {
		return nil, err
	}
	settled, err := SettleCarried(st, runnerID, false, ReasonSessionLost)
	return append([]Row{lost}, settled...), err
}
