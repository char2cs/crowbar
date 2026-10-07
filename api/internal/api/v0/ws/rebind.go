package ws

import (
	"slices"
	"sync/atomic"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

// binding is one client's resolved value for one bound filter (FilterDef.Resolve).
type binding struct {
	filter int
	param  string
	value  atomic.Pointer[string]
}

func (bd *binding) current() string {
	if v := bd.value.Load(); v != nil {
		return *v
	}
	return ""
}

// set stores value and reports whether it changed. Callers hold rebindMu.
func (bd *binding) set(value string) bool {
	if bd.current() == value {
		return false
	}
	bd.value.Store(&value)
	return true
}

// resolveBindings answers every binding with ONE Resolve call per bound filter,
// over the distinct params the bindings carry. A param Resolve left unanswered
// (a failed read) is omitted, so its binding keeps its current value.
func resolveBindings[T any](
	filters []FilterDef[T],
	bindings []*binding,
) map[*binding]string {
	params := make(map[int][]string)
	for _, bd := range bindings {
		if !slices.Contains(params[bd.filter], bd.param) {
			params[bd.filter] = append(params[bd.filter], bd.param)
		}
	}
	answers := make(map[int]map[string]string, len(params))
	for i, values := range params {
		slices.Sort(values)
		answers[i] = filters[i].Resolve(values)
	}
	out := make(map[*binding]string, len(bindings))
	for _, bd := range bindings {
		if value, ok := answers[bd.filter][bd.param]; ok {
			out[bd] = value
		}
	}
	return out
}

func hasBoundFilter[T any](
	def StreamDef[T],
) bool {
	return slices.ContainsFunc(def.Filters, func(f FilterDef[T]) bool { return f.Resolve != nil })
}

// admit builds the client, acquires its scope and registers it under
// rebindMu, so a Rebind either sees it or ran entirely before its bindings
// were resolved. A panicking OnSubscribe leaves nothing registered.
func (b *Broadcaster[T]) admit(
	c *gin.Context,
	snapScope string,
) *filteredClient[T] {
	b.rebindMu.Lock()
	defer b.rebindMu.Unlock()
	predicate, bindings := BuildPredicate(c, b.def)
	cl := &filteredClient[T]{
		client:       newClient(),
		predicate:    predicate,
		bindings:     bindings,
		requestScope: b.scopeKey(c),
	}
	if b.def.Snapshot != nil {
		cl.resnapshot = func() [][]byte { return b.snapshotFor(cl, snapScope) }
	}
	b.onSubscribe(b.scopeOf(cl))
	b.register(cl)
	return cl
}

// scopeOf is what cl's lifecycle hooks refcount: a bound client's binding,
// read live so the held resource follows Rebind, else the request's ScopeKey.
func (b *Broadcaster[T]) scopeOf(
	cl *filteredClient[T],
) string {
	if b.def.ScopeKey == nil {
		return ""
	}
	if len(cl.bindings) > 0 {
		return cl.bindings[0].current()
	}
	return cl.requestScope
}

// moved is a client whose binding changed in a Rebind, with the scope it held.
type moved[T any] struct {
	cl   *filteredClient[T]
	from string
}

// Rebind re-resolves every bound client in one Resolve call per bound filter
// and returns how many clients' bindings changed. Each of those is resent the
// stream's Snapshot, if it has one.
func (b *Broadcaster[T]) Rebind() int {
	b.rebindMu.Lock()
	defer b.rebindMu.Unlock()
	answers := resolveBindings(b.def.Filters, b.allBindings())
	moves := b.applyBindings(answers)
	b.rescope(moves)
	for _, m := range moves {
		m.cl.requestResnapshot()
	}
	return len(moves)
}

// rescope acquires every new scope before releasing any old one, so a scope
// that stays held (two clients trading workspaces) never touches zero.
func (b *Broadcaster[T]) rescope(
	moves []moved[T],
) {
	for _, m := range moves {
		if to := b.scopeOf(m.cl); to != m.from {
			b.onSubscribe(to)
		}
	}
	for _, m := range moves {
		if b.scopeOf(m.cl) != m.from {
			b.onUnsubscribe(m.from)
		}
	}
}

func (b *Broadcaster[T]) allBindings() []*binding {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var all []*binding
	for cl := range b.clients {
		all = append(all, cl.bindings...)
	}
	return all
}

// applyBindings swaps under the write lock so no Push still in flight matched
// against an old binding once a resnapshot is requested.
func (b *Broadcaster[T]) applyBindings(
	answers map[*binding]string,
) []moved[T] {
	b.mu.Lock()
	defer b.mu.Unlock()
	var moves []moved[T]
	for cl := range b.clients {
		from := b.scopeOf(cl)
		changed := false
		for _, bd := range cl.bindings {
			if value, ok := answers[bd]; ok && bd.set(value) {
				changed = true
			}
		}
		if changed {
			moves = append(moves, moved[T]{cl: cl, from: from})
		}
	}
	return moves
}

// RequestRebind asks the rebind worker for a Rebind without blocking. Requests
// made while one is pending collapse into it; one made during a Rebind queues
// exactly one more, which reads the state as of when it runs.
func (b *Broadcaster[T]) RequestRebind() {
	select {
	case b.rebind <- struct{}{}:
	default:
	}
}

// Close stops the rebind worker and waits for it to exit. Idempotent; a
// broadcaster without bound filters has no worker.
func (b *Broadcaster[T]) Close() {
	if b.stop == nil {
		return
	}
	b.closeOnce.Do(func() { close(b.stop) })
	<-b.stopped
}

func (b *Broadcaster[T]) startRebinder() {
	b.rebind = make(chan struct{}, 1)
	b.stop = make(chan struct{})
	b.stopped = make(chan struct{})
	safego.Go("broadcaster.rebind", b.rebindLoop)
}

func (b *Broadcaster[T]) rebindLoop() {
	defer close(b.stopped)
	for {
		select {
		case <-b.stop:
			return
		case <-b.rebind:
			b.safeRebind()
		}
	}
}

// safeRebind keeps the worker alive across a panicking Resolve.
func (b *Broadcaster[T]) safeRebind() {
	defer safego.Recover("broadcaster.Rebind")
	b.Rebind()
}
