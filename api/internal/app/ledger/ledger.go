// Package ledger implements the per-chat, append-only, provider-tagged store of
// conversation turns Crowbar derives from vendor-CLI hooks (agentic-engine
// descriptor-v2 §7). Crowbar builds this record itself; it never reads a file
// the vendor CLI wrote.
package ledger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Turn is one conversation turn recorded from a hook.
type Turn struct {
	Role     string    `json:"role"` // "user" | "assistant"
	Provider string    `json:"provider"`
	Text     string    `json:"text"`
	At       time.Time `json:"at"`
}

// Speaker is the attribution this turn is rendered under: the bare role, or
// "assistant (<provider>)" when a vendor CLI produced it and named itself.
//
// It is a method rather than a fmt call at each render site because a ledger has
// more than one consumer now — RenderConversation, and the agent tool surface's
// get_chat_log, which reads Turns directly so it can COUNT turns before
// rendering them. Both must attribute a speaker identically, or the same
// conversation reads as two different ones depending on who asked.
func (t Turn) Speaker() string {
	if t.Role == "assistant" && t.Provider != "" {
		return fmt.Sprintf("assistant (%s)", t.Provider)
	}
	return t.Role
}

// Ledger is a per-chat, append-only store of conversation turns.
type Ledger struct{ dir string }

// Open ensures the ledger directory exists and returns a handle to it.
func Open(dir string) (*Ledger, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("ledger: mkdir: %w", err)
	}
	return &Ledger{dir: dir}, nil
}

// AppendTurn records one conversation turn. Empty text is a no-op (returns
// ("", nil)) so a provider that fires turn_stop with no final message never
// writes a blank entry. The %08d prefix keeps lexical == chronological order.
func (l *Ledger) AppendTurn(role, provider string, at time.Time, text string) (string, error) {
	if text == "" {
		return "", nil
	}
	seq, err := l.nextSeq()
	if err != nil {
		return "", err
	}
	rec, err := json.Marshal(Turn{Role: role, Provider: provider, Text: text, At: at.UTC()})
	if err != nil {
		return "", fmt.Errorf("ledger: marshal turn: %w", err)
	}
	name := fmt.Sprintf("%08d-%s-%s-%s.turn", seq, at.UTC().Format("20060102T150405Z"), role, provider)
	if err := os.WriteFile(filepath.Join(l.dir, name), rec, 0o640); err != nil { //nolint:gosec // ledger entries are group-readable by design; name is ledger-generated
		return "", fmt.Errorf("ledger: write: %w", err)
	}
	return name, nil
}

func (l *Ledger) entries() ([]string, error) {
	des, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, fmt.Errorf("ledger: readdir: %w", err)
	}
	var names []string
	for _, de := range des {
		if !de.IsDir() && filepath.Ext(de.Name()) == ".turn" {
			names = append(names, de.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func (l *Ledger) nextSeq() (int, error) {
	names, err := l.entries()
	if err != nil {
		return 0, err
	}
	return len(names) + 1, nil
}

func (l *Ledger) readTurn(name string) (Turn, error) {
	data, err := os.ReadFile(filepath.Join(l.dir, name)) //nolint:gosec // name comes from entries() listing l.dir, not external input
	if err != nil {
		return Turn{}, fmt.Errorf("ledger: read %s: %w", name, err)
	}
	var tn Turn
	if err := json.Unmarshal(data, &tn); err != nil {
		return Turn{}, fmt.Errorf("ledger: unmarshal %s: %w", name, err)
	}
	return tn, nil
}

// Turns reads every recorded turn, oldest first.
//
// It exists for the agent tool surface's get_chat_log, which CAPS how much of a
// sibling chat it hands a model and therefore has to count turns before it
// renders any. Counting them back out of RenderConversation's text is not
// possible: turns are joined by a blank line and a turn's own body is free-form
// model prose containing blank lines routinely, so a reader splitting the
// rendering apart would report a turn count that is not true.
func (l *Ledger) Turns() ([]Turn, error) {
	names, err := l.entries()
	if err != nil {
		return nil, err
	}
	out := make([]Turn, 0, len(names))
	for _, n := range names {
		tn, err := l.readTurn(n)
		if err != nil {
			return nil, err
		}
		out = append(out, tn)
	}
	return out, nil
}

// RenderConversation reads every turn in order and renders a legible plain-text
// conversation for a receiving model.
func (l *Ledger) RenderConversation() ([]byte, error) {
	return l.render(time.Time{})
}

// RenderConversationAfter renders only the turns recorded strictly after cut —
// the "while you were away" gap handed to a provider being resumed into its OWN
// native session. That session already holds everything up to the moment it was
// switched out, so replaying the whole ledger to it would duplicate its history;
// only what happened under OTHER providers since then is new information. A zero
// cut renders everything (RenderConversation). Returns empty (not an error) when
// nothing happened in the gap — the caller then injects nothing at all.
func (l *Ledger) RenderConversationAfter(cut time.Time) ([]byte, error) {
	return l.render(cut)
}

// LastEntryAt returns the FILE NAME of the last turn recorded at or before cut —
// the last turn a provider being resumed has already seen, and therefore the point
// it should start reading from. Empty when the ledger has nothing that old (the
// provider has seen nothing; it starts at the beginning).
//
// A name, not a body: a resumed provider is POINTED at the ledger rather than
// handed a copy of it, so all it needs is where to start.
func (l *Ledger) LastEntryAt(cut time.Time) (string, error) {
	names, err := l.entries()
	if err != nil {
		return "", err
	}
	last := ""
	for _, n := range names {
		tn, err := l.readTurn(n)
		if err != nil {
			return "", err
		}
		if cut.IsZero() || !tn.At.After(cut) {
			last = n
		}
	}
	return last, nil
}

// LastTurnAt returns when provider last spoke in this chat — the moment it left,
// and therefore the cut for the "while you were away" gap a resumed provider is
// handed. Zero when it never spoke.
//
// A zero return is also the answer to the OTHER question the resume path asks:
// "does this provider's conversation actually EXIST on disk?" A vendor CLI reports
// a session id the instant it starts, but only WRITES that conversation once there
// is at least one message — claude refuses `--resume <id>` for such a session with
// "No conversation found with session ID", which is exactly what killed a chat the
// user had never typed in. Crowbar records a turn from the very same hooks, so a
// provider with no turn here has nothing to resume and must be spawned fresh.
//
// It reads the ledger rather than runner history because the ledger is the record
// of what was actually SAID. Conversation history only knows when a conversation
// OPENED, and a provider that opens a conversation, says nothing, and is switched
// away from must not be resumed into it.
func (l *Ledger) LastTurnAt(provider string) (time.Time, error) {
	names, err := l.entries()
	if err != nil {
		return time.Time{}, err
	}
	var last time.Time
	for _, n := range names {
		tn, err := l.readTurn(n)
		if err != nil {
			return time.Time{}, err
		}
		if tn.Provider == provider && tn.At.After(last) {
			last = tn.At
		}
	}
	return last, nil
}

func (l *Ledger) render(cut time.Time) ([]byte, error) {
	turns, err := l.Turns()
	if err != nil {
		return nil, err
	}
	var out []byte
	for _, tn := range turns {
		if !cut.IsZero() && !tn.At.After(cut) {
			continue
		}
		out = append(out, []byte(tn.Speaker()+": "+tn.Text+"\n\n")...)
	}
	return out, nil
}
