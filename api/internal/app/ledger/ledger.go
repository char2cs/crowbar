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

// HasTurns reports whether provider recorded any turn in [from, to] — the window
// of one segment's life. It answers "did this CLI's native session ever get any
// CONTENT", which is not the same question as "did it report a session id".
//
// A vendor CLI binds (and reports) a session id the moment it starts, but only
// WRITES the conversation once there is at least one message: claude refuses
// `--resume <id>` for such a session with "No conversation found with session ID".
// Crowbar records a turn from the very same hooks, so an empty window here means
// the CLI has nothing on disk under that id and must be spawned fresh instead of
// resumed. A zero `to` means the segment is still open (window ends now).
func (l *Ledger) HasTurns(provider string, from, to time.Time) (bool, error) {
	names, err := l.entries()
	if err != nil {
		return false, err
	}
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(l.dir, n)) //nolint:gosec // n comes from entries() listing l.dir, not external input
		if err != nil {
			return false, fmt.Errorf("ledger: read %s: %w", n, err)
		}
		var tn Turn
		if err := json.Unmarshal(data, &tn); err != nil {
			return false, fmt.Errorf("ledger: unmarshal %s: %w", n, err)
		}
		if tn.Provider != provider || tn.At.Before(from) {
			continue
		}
		if to.IsZero() || !tn.At.After(to) {
			return true, nil
		}
	}
	return false, nil
}

func (l *Ledger) render(cut time.Time) ([]byte, error) {
	names, err := l.entries()
	if err != nil {
		return nil, err
	}
	var out []byte
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(l.dir, n)) //nolint:gosec // n comes from entries() listing l.dir, not external input
		if err != nil {
			return nil, fmt.Errorf("ledger: read %s: %w", n, err)
		}
		var tn Turn
		if err := json.Unmarshal(data, &tn); err != nil {
			return nil, fmt.Errorf("ledger: unmarshal %s: %w", n, err)
		}
		if !cut.IsZero() && !tn.At.After(cut) {
			continue
		}
		header := tn.Role
		if tn.Role == "assistant" && tn.Provider != "" {
			header = fmt.Sprintf("assistant (%s)", tn.Provider)
		}
		out = append(out, []byte(header+": "+tn.Text+"\n\n")...)
	}
	return out, nil
}
