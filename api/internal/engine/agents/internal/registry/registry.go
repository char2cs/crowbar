package registry

import (
	"strings"
	"sync"
)

type Registry struct {
	mu       sync.Mutex
	injected map[string][]string
}

func New() *Registry {
	return &Registry{injected: map[string][]string{}}
}

func (r *Registry) SetInjected(runnerID string, docs ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range docs {
		if d != "" {
			r.injected[runnerID] = append(r.injected[runnerID], d)
		}
	}
}

// ConsumePrefix reports whether text contains the echo of a document this
// runner was handed and, if so, retires that document (one-shot: a user
// retyping the same words later is still recorded as their own) and returns
// text with that document — and the separator mergeLeadingPositional
// (spawnsteps.go) joins it to a real prompt with — removed.
//
// Containment, not equality: what actually reaches the CLI as a positional
// prompt is the DESCRIPTOR's own template rendering of the doc Go computed —
// claude's resume pointer wraps it in <system-reminder> tags Go never
// mentions, and codex's does not. Go registers the bare doc it composed; the
// wire text a hook echoes back is whatever the provider's own template made
// of it. An equality check here would treat every such wrapping as a foreign
// message and start recording the injected handoff as the user's own turn.
//
// A bare echo (nothing else was riding the same spawn) returns an empty
// remainder — the caller's existing "this is not something the user said"
// branch. A remainder left over after removing the doc is the user's own
// real prompt, merged ahead of the injected context into the SAME positional
// argv token: it must be recognized here, not silently swallowed with the
// preamble it happened to travel next to, or the ledger records Crowbar's own
// injected document as what the user typed, the derived title is nonsense,
// and every hash-based "was this prompt actually accepted" check downstream
// (ConfirmPromptAccepted, promptRecordAccepted) can never match the original
// dispatch text again.
// MergedSeparator is mergeLeadingPositional's own exact join string
// (spawnsteps.go) between the rendered context step and the message step —
// the one thing Go DOES get to choose, unlike the wrapping around the doc.
// Exported so the merge side and this consume side can never drift apart.
const MergedSeparator = "\n\n"

func (r *Registry) ConsumePrefix(runnerID, text string) (remainder string, found bool) {
	if text == "" {
		return text, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	docs := r.injected[runnerID]
	for i, d := range docs {
		idx := strings.Index(text, d)
		if idx < 0 {
			continue
		}
		r.injected[runnerID] = append(docs[:i:i], docs[i+1:]...)
		// Whatever the descriptor's own template wrapped the doc in (claude's
		// <system-reminder> tags, say) sits between the doc's own end and
		// mergeLeadingPositional's separator — Go never learns that
		// wrapping's shape, so this locates the ONE separator it DID choose,
		// searched only after the doc's match (never before it, and never
		// inside it: AssembleConversation's own rendering already uses blank
		// lines between turns, so a global search would cut the gap content
		// itself in half). No separator after the match at all means nothing
		// else was riding this spawn — a bare echo, same as before.
		after := text[idx+len(d):]
		if sep := strings.Index(after, MergedSeparator); sep >= 0 {
			return strings.TrimSpace(after[sep+len(MergedSeparator):]), true
		}
		return "", true
	}
	return text, false
}

func (r *Registry) Forget(runnerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.injected, runnerID)
}
