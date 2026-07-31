// Package handlers contains the HTTP handler logic for the /v0/settings/ui
// endpoint pair: the daemon-side home for the client's local UI state.
package handlers

import (
	"context"
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// UISettingsStore is the narrow CRUD surface the UI-settings handlers need. The
// generic sqlite store built over the global view.db satisfies it, so these
// handlers share the daemon's existing multi-connection view pool rather than
// opening a database of their own.
type UISettingsStore interface {
	FindByKey(
		ctx context.Context,
		scope string,
	) (*domain.UISettings, error)
	Save(
		ctx context.Context,
		item domain.UISettings,
	) error
}

// Handlers holds the dependencies shared across the UI-settings handlers: the
// backing store and the per-scope write locks that serialise concurrent PUTs.
type Handlers struct {
	store UISettingsStore
	locks sync.Map
}

// New returns an initialised Handlers over the given store.
func New(
	store UISettingsStore,
) *Handlers {
	return &Handlers{store: store}
}

// lockFor returns the mutex guarding writes to one scope, creating it on first
// use. Writes to DIFFERENT scopes never contend; writes to the SAME scope are
// fully serialised, which is what makes a PUT a single indivisible replace even
// though the underlying GORM upsert is an UPDATE followed by a conditional
// INSERT. Two concurrent first-writes to a fresh scope would otherwise both see
// the UPDATE match zero rows and both attempt the INSERT, and one of them would
// lose to a UNIQUE-constraint failure.
func (h *Handlers) lockFor(
	scope string,
) *sync.Mutex {
	existing, _ := h.locks.LoadOrStore(scope, &sync.Mutex{})
	mu, _ := existing.(*sync.Mutex)
	return mu
}
