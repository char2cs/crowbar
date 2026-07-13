package agent

import "sync"

// chatGate serialises the USER-INITIATED spawn paths for one chat: SpawnChat,
// SwitchProvider and ResumeChat all take it, and nothing else does.
//
// It exists because starting a CLI is not a database write, and so nothing in the
// persistence layer can order two of them. Two concurrent SwitchProvider calls on one
// chat (a double-clicked provider dropdown) both read the same live runner, both kill
// it, and both spawn — leaving TWO CLIs pointed at one chat. asynx's optimistic
// concurrency cannot catch it, because the two spawns create DIFFERENT aggregates: there
// is no shared row for them to collide on. Displacement makes the READ side coherent
// under that overlap; only serialisation stops the overlap being created.
//
// It is NOT the return of the guard this refactor deleted. OpenSegment.Validate REJECTED
// a move — a fait accompli the CLI had already performed — after a destructive write had
// already committed, and that is what bricked a chat. This rejects nothing, reads no
// aggregate state, and orders only work Crowbar itself initiates on the user's behalf. A
// caller that waits its turn then does exactly what it would have done had the user
// clicked twice slowly.
//
// It is never taken on the HOOK path. A hook must never block and must never fail: by
// the time it arrives the CLI has already acted, and there is nothing useful to do with
// a caller made to wait (spec §3).
//
// (SpawnChat's chat id is a fresh uuid nobody else can name, so its gate is
// uncontended by construction. It takes it anyway, so that "every path that starts a CLI
// on a chat holds that chat's gate" is a rule with no exceptions to audit.)
type chatGate struct {
	mu    sync.Mutex
	gates map[string]*gateEntry
}

// gateEntry is one chat's lock plus a reference count, so the map does not grow without
// bound across the life of a daemon (a long-lived install churns through chats).
type gateEntry struct {
	mu   sync.Mutex
	refs int
}

func newChatGate() *chatGate {
	return &chatGate{gates: map[string]*gateEntry{}}
}

// lock blocks until this chat's gate is free and returns the release func. Callers
// `defer` it. Re-entrant use would deadlock, so the gate is taken at exactly one place
// per path: ResumeChat and SwitchProvider therefore share an already-locked inner
// implementation rather than calling each other.
func (g *chatGate) lock(chatID string) func() {
	g.mu.Lock()
	e, ok := g.gates[chatID]
	if !ok {
		e = &gateEntry{}
		g.gates[chatID] = e
	}
	e.refs++
	g.mu.Unlock()

	e.mu.Lock()

	return func() {
		e.mu.Unlock()

		g.mu.Lock()
		e.refs--
		if e.refs == 0 {
			delete(g.gates, chatID)
		}
		g.mu.Unlock()
	}
}
