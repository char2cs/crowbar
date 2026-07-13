package agent

import "sync"

// Registry is what is LEFT of the old context-move registry, and what is left is
// the only part that was ever entitled to exist in memory.
//
// It used to hold segToChat / sessionToChat / segToSession — an in-memory shadow
// of state that is also durable. The reducer mutated those maps BEFORE the
// aggregate commands ran, so when a command failed the registry still believed
// the runner had moved: the orphaned CLI's turn hooks were then routed into a chat
// it had left, polluting it (the split brain, spec §1 bug 2). Placement is now read
// from the runner aggregate — one authority, no shadow — and those maps are gone,
// along with OnSessionStart/BindSegment/Seed/ChatFor. The pure reducer that
// replaced OnSessionStart is Decide (reducer.go); it owns no state at all.
//
// segToInjected survives because it is NOT a shadow of anything. It is genuinely
// ephemeral, per-spawn state — the text Crowbar injected into a CLI at spawn, kept
// only long enough to recognise that CLI echoing it back at us — with no durable
// counterpart it could drift from, and no meaning at all once the process is gone.
type Registry struct {
	mu            sync.Mutex
	segToInjected map[string][]string // runner id -> everything Crowbar injected at spawn
}

func NewRegistry() *Registry {
	return &Registry{segToInjected: map[string][]string{}}
}

// SetInjectedContext records everything Crowbar injected into a runner at spawn —
// the {context} document AND the pointer message a provider reachable only through
// a user message receives — so ConsumeInjectedContext can recognise whichever of
// them comes back through the user-prompt hook.
func (r *Registry) SetInjectedContext(runnerID string, docs ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range docs {
		if d != "" {
			r.segToInjected[runnerID] = append(r.segToInjected[runnerID], d)
		}
	}
}

// ConsumeInjectedContext reports whether text is the very context document
// Crowbar injected into this runner at spawn — i.e. the CLI is echoing our own
// handoff back at us through its user-prompt hook, not relaying something the
// user typed. Provider-agnostic on purpose: a provider whose only resume channel
// is a user message (codex) fires user_prompt for the injected document, while
// one with a silent channel (claude) never does, and the registry does not need
// to know which is which.
//
// Recording that echo as a ledger turn is what made handoffs NEST — the blob
// became a "user" turn, and the next handoff embedded it inside itself. The
// match is one-shot (the entry is consumed) so that a user who genuinely retypes
// the same text later is still recorded.
func (r *Registry) ConsumeInjectedContext(runnerID, text string) bool {
	if text == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	docs := r.segToInjected[runnerID]
	for i, d := range docs {
		if d != text {
			continue
		}
		r.segToInjected[runnerID] = append(docs[:i:i], docs[i+1:]...)
		return true
	}
	return false
}

// ForgetRunner drops a dead runner's injected-context entries. It is called when
// the PTY dies: the echo guard exists to recognise a document coming back from a
// LIVE process, so once the process is gone the entry has no meaning and would
// otherwise accumulate — one handoff-sized string per spawn — for the life of the
// daemon. (The retired ForgetChat swept these by CHAT, which it could only do
// because it held a segment→chat map; this keys the sweep on the thing that
// actually owns the entry.)
func (r *Registry) ForgetRunner(runnerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.segToInjected, runnerID)
}
