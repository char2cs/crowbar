// Package chatlog is the read shape of a chat's conversation record.
//
// It is types only. The record itself lives in the agentactivity aggregate and
// its projection; this package exists so the API layer, the agent tool surface
// and the usecase all speak one vocabulary without any of them importing a
// repository.
//
// It replaced a flat-file ledger. That ledger could hold a role, a provider and a
// blob of text and nothing else, which made it a blocker rather than a component
// to extend: there was no way to record that a tool ran, that a subagent forked,
// or that the agent was blocked waiting for a human.
package chatlog

import (
	"fmt"
	"time"
)

// Turn is one side of the conversation.
type Turn struct {
	// ID is the turn's own identity, which the activity record attaches tool
	// calls, subagents and interruptions to. It crosses the wire so a client can
	// show which activity produced which reply — the association exists in the
	// record either way, and omitting it here would only hide it.
	ID       string `json:"id"`
	Role     string `json:"role"` // "user" | "assistant"
	Provider string `json:"provider"`
	// RunnerID correlates a turn with the Crowbar process that delivered it. It
	// is internal: durable prompt delivery reads it, the client never sees it.
	RunnerID string `json:"runnerId,omitempty"`
	// SessionID attributes the turn to the provider-native conversation current
	// on that runner, which is what resume decisions are made against.
	SessionID string    `json:"sessionId,omitempty"`
	Text      string    `json:"text"`
	Effort    string    `json:"effort,omitempty"`
	At        time.Time `json:"at"`
}

// Speaker is the attribution a turn renders under: the bare role, or
// "assistant (<provider>)" when a vendor CLI produced it and named itself.
//
// It is a method rather than a fmt call at each site because the record has more
// than one consumer — the chat surface and the cross-agent tool surface — and
// two spellings would make the same conversation read as two different ones
// depending on who asked.
//
// A HARNESS turn is spelled out rather than left as its bare role, and that is
// the whole point of it having one. This text is what get_chat_log hands ANOTHER
// agent and what a handoff document hands an incoming CLI, and both of them read
// this rendering as a transcript of a conversation with a person. A row that said
// only "harness:" would be read as some Crowbar-internal label and the sentences
// under it taken at face value; what is under it is a provider's own machinery
// talking to its own model, and the reader's job is to not act on it as if the
// user had asked for it. So the line says so.
func (t Turn) Speaker() string {
	switch t.Role {
	case "assistant":
		if t.Provider != "" {
			return fmt.Sprintf("assistant (%s)", t.Provider)
		}
	case "harness":
		if t.Provider != "" {
			return fmt.Sprintf("%s harness (injected, NOT the user)", t.Provider)
		}
		return "harness (injected, NOT the user)"
	}
	// Every other role renders as itself, including one minted by a newer daemon
	// than this code. That degrades to a label a reader can look up, and — the
	// property that matters — it is never the word "user".
	return t.Role
}

// Message is one turn together with its chat-local sequence. The sequence is
// Crowbar's own counter; a provider payload never gets to choose it.
type Message struct {
	Sequence int
	Turn
}

// Page is a bounded chronological window. Cursor is the greatest sequence in
// Items, OldestCursor the least, and HasMore means more exist in the direction
// of the request: newer for an after page, older for a before or initial page.
type Page struct {
	Cursor       int
	OldestCursor int
	HasMore      bool
	Items        []Message
}
