package ledger

import (
	"slices"
	"time"
)

// Why a session was bound to the chat.
const (
	CauseAssigned  = "assigned"  // Crowbar minted the id and launched the CLI with it
	CauseAnnounced = "announced" // the CLI announced an id of its own
	CauseFresh     = "fresh"     // Crowbar minted the id to replace a lost session
)

// SessionBound is the body of a session_bound row: from now on the chat's
// conversation with Provider is SessionID.
type SessionBound struct {
	Provider  string `json:"provider"`
	SessionID string `json:"sessionId"`
	Cause     string `json:"cause"` // CauseAssigned | CauseAnnounced | CauseFresh
	Previous  string `json:"previous,omitempty"`
}

// SessionLost is the body of a session_lost row: the CLI refused to resume
// SessionID, and Reason is the line it refused with.
type SessionLost struct {
	Provider  string `json:"provider"`
	SessionID string `json:"sessionId"`
	Reason    string `json:"reason"`
}

// ConversationState is where a chat's conversation with one provider stands.
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

// Next reports the state a transition lands in and whether the conversation
// diagram allows it: unbound→bound, bound→lost, lost→bound, bound→bound.
func (s ConversationState) Next(to ConversationState) (ConversationState, bool) {
	if slices.Contains(conversationEdges[s], to) {
		return to, true
	}
	return s, false
}

// Conversation is one (provider, session) the chat has been bound to.
type Conversation struct {
	Provider, SessionID string
	State               ConversationState
	BoundAt             time.Time
	Cause               string // of its latest binding
}

// Conversation is the chat's current conversation with provider.
func (s State) Conversation(provider string) (Conversation, bool) {
	i, ok := s.current[provider]
	if !ok {
		return Conversation{}, false
	}
	return s.conversations[i], true
}

// Conversations is every (provider, session) ever bound, oldest first. A
// session the chat moved off reads unbound, unless it was lost.
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
	if s.current == nil {
		s.current = map[string]int{}
	}
	s.current[body.Provider] = i
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

// Assign is the session_bound row for a session Crowbar minted for provider:
// fresh when it replaces a lost one.
func Assign(st State, provider, sessionID string) (Row, error) {
	body := SessionBound{Provider: provider, SessionID: sessionID, Cause: CauseAssigned}
	if c, ok := st.Conversation(provider); ok && c.State == ConversationLost {
		body.Cause, body.Previous = CauseFresh, c.SessionID
	}
	return NewRow(KindSessionBound, "", "", body)
}

// Announce is what a CLI announcing sessionID adds: nothing for the bound
// session, a binding for an unbound chat, a change, or a conflict when
// ownedElsewhere reports that another chat holds the session.
func Announce(st State, provider, sessionID string, ownedElsewhere func() (bool, error)) ([]Row, error) {
	c, ok := st.Conversation(provider)
	if ok && c.SessionID == sessionID && c.State == ConversationBound {
		return nil, nil
	}
	taken, err := ownedElsewhere()
	if err != nil {
		return nil, err
	}
	if taken {
		r, err := NewRow(KindMarker, "", "", Marker{Kind: MarkerSessionConflict, Detail: sessionID})
		return []Row{r}, err
	}
	bound := SessionBound{Provider: provider, SessionID: sessionID, Cause: CauseAnnounced}
	if ok && c.SessionID != sessionID {
		bound.Previous = c.SessionID
	}
	r, err := NewRow(KindSessionBound, "", "", bound)
	if err != nil || !ok || c.State != ConversationBound {
		return []Row{r}, err
	}
	m, err := NewRow(KindMarker, "", "", Marker{Kind: MarkerSessionChanged, Detail: sessionID})
	return []Row{r, m}, err
}

// Lose is the session_lost row for a resume the CLI refused, plus a settle
// for every pending prompt runnerID carried. Nil when sessionID is not the
// provider's bound session any more.
func Lose(st State, provider, sessionID, reason, runnerID string) ([]Row, error) {
	if c, ok := st.Conversation(provider); !ok || c.SessionID != sessionID || c.State != ConversationBound {
		return nil, nil
	}
	lost, err := NewRow(KindSessionLost, "", "", SessionLost{Provider: provider, SessionID: sessionID, Reason: reason})
	if err != nil {
		return nil, err
	}
	settled, err := SettleCarried(st, runnerID, false, ReasonSessionLost)
	return append([]Row{lost}, settled...), err
}
