package chat

import "sync"

// ChoiceWatch relays "this chat's pending prompts may have changed" from the two
// places that know — the activity ledger and the answer desk — to the one
// publisher that tells clients.
//
// It exists because the ledger is built before the usecase that publishes: the
// composition root hands the same ChoiceWatch to both, and New binds the
// publisher to it. Until then, and in a daemon built without one, a notification
// is dropped, which is correct: nobody is listening yet.
type ChoiceWatch struct {
	mu      sync.RWMutex
	publish func(chatID string)
}

// NewChoiceWatch returns a watch with no publisher bound.
func NewChoiceWatch() *ChoiceWatch { return &ChoiceWatch{} }

// Notify announces that chatID's pending prompts may have changed. Safe on nil.
func (w *ChoiceWatch) Notify(chatID string) {
	if w == nil {
		return
	}
	w.mu.RLock()
	publish := w.publish
	w.mu.RUnlock()
	if publish != nil {
		publish(chatID)
	}
}

func (w *ChoiceWatch) bind(publish func(chatID string)) {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.publish = publish
	w.mu.Unlock()
}
