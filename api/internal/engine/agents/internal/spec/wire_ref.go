package spec

import "strings"

const wireRefAlternation = "||"

// WireRef is the provider's own name for one canonical event — or, where a provider
// splits a single conversational fact across several wire methods, every name that
// fact answers to, alternated with `||` exactly as `when:` values and `map:` paths
// already are.
//
// Alternation exists because the canonical vocabulary is CLOSED: a provider cannot
// answer a namespaced wire method by inventing a Crowbar event per namespace. Codex
// asks for tool approval under item/commandExecution/requestApproval for a command
// and item/fileChange/requestApproval for a patch — one canonical `permission` fact,
// two wire names. A descriptor naming only one of them leaves the other matching
// nothing at all: no prompt is raised, no reply is ever sent, and the CLI blocks on
// an answer that cannot arrive until its approval budget expires (measured live
// against codex-cli 0.149.1 — a turn sat waitingOnApproval indefinitely).
//
// Every event that has exactly one wire name still declares it as the plain string
// it always was.
type WireRef string

// Names is every wire name the event answers to, in declared order.
func (w WireRef) Names() []string {
	var out []string
	for _, part := range strings.Split(string(w), wireRefAlternation) {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

// Name is the first declared wire name: what a caller with room for exactly one uses,
// since an outbound call names a single method and a label wants a single word.
func (w WireRef) Name() string {
	names := w.Names()
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// Has reports whether wireMethod is one of the names this event answers to.
func (w WireRef) Has(wireMethod string) bool {
	for _, name := range w.Names() {
		if name == wireMethod {
			return true
		}
	}
	return false
}

// Empty reports whether the event names nothing in this direction.
func (w WireRef) Empty() bool {
	return w.Name() == ""
}
