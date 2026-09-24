// Package dedup is the hook ingress's delivery-id deduplication: a bounded,
// in-memory set of delivery ids whose effects already committed.
//
// Crowbar's hook relay retries a failed POST a few times, a few hundred
// milliseconds apart, reusing one delivery id (cmd/crowbar/hook_delivery.go).
// This set is what makes those retries idempotent. It is deliberately NOT
// durable (owner decision 5, spec §6.5): the relay's retry window is seconds,
// so a record that must outlive a daemon restart is guarding nothing — and an
// fsync per hook was the most expensive thing on the ingest hot path. Memory is
// bounded twice: entries expire after a TTL far longer than any retry window,
// and the set never holds more than a fixed cap.
package dedup

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

const (
	// DefaultTTL is how long a completed delivery id is remembered. The relay
	// gives up after ~1s of retries; a permission hook can hold its POST open
	// while the user decides, so the margin is generous.
	DefaultTTL = 10 * time.Minute
	// DefaultMax caps the set whatever the TTL: a burst of hooks evicts the
	// oldest ids first, which are the ones least likely to be retried.
	DefaultMax = 4096
)

// ErrPayloadMismatch means one delivery id arrived twice carrying different
// payloads, so deduplicating it would drop a distinct event.
var ErrPayloadMismatch = errors.New("agent: hook delivery id reused with different payload")

// Hash binds a delivery id to the exact payload it carried, so a reused id
// carrying different content is caught instead of deduplicated.
func Hash(runnerID, provider, event string, raw []byte) string {
	h := sha256.New()
	for _, part := range [][]byte{[]byte(runnerID), []byte(provider), []byte(event)} {
		_, _ = h.Write(part)
		_, _ = h.Write([]byte{0})
	}
	_, _ = h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}

type entry struct {
	id   string
	hash string
	at   time.Time
}

// Set is the bounded set of completed delivery ids. The zero value is not
// usable; build one with New.
type Set struct {
	mu   sync.Mutex
	ttl  time.Duration
	max  int
	now  func() time.Time
	byID map[string]string
	// fifo is insertion order, which is also expiry order: every entry has the
	// same TTL and is stamped on insertion. head is the oldest live index.
	fifo []entry
	head int
}

// New returns an empty set remembering ids for ttl, holding at most max.
func New(ttl time.Duration, max int, now func() time.Time) *Set {
	if now == nil {
		now = time.Now
	}
	return &Set{ttl: ttl, max: max, now: now, byID: map[string]string{}}
}

// Done reports whether deliveryID already completed with this payload hash,
// and ErrPayloadMismatch when it completed with a different one.
func (s *Set) Done(deliveryID, hash string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked()
	got, ok := s.byID[deliveryID]
	if !ok {
		return false, nil
	}
	if got != hash {
		return false, ErrPayloadMismatch
	}
	return true, nil
}

// Complete records that deliveryID's effects committed.
func (s *Set) Complete(deliveryID, hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[deliveryID]; ok {
		return
	}
	s.byID[deliveryID] = hash
	s.fifo = append(s.fifo, entry{id: deliveryID, hash: hash, at: s.now()})
	for len(s.byID) > s.max {
		s.popLocked()
	}
	s.expireLocked()
}

// Len is how many ids the set holds.
func (s *Set) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byID)
}

func (s *Set) expireLocked() {
	cutoff := s.now().Add(-s.ttl)
	for s.head < len(s.fifo) && !s.fifo[s.head].at.After(cutoff) {
		s.popLocked()
	}
}

func (s *Set) popLocked() {
	delete(s.byID, s.fifo[s.head].id)
	s.fifo[s.head] = entry{}
	s.head++
	// Compact once the dead prefix dominates, so the backing array stays
	// proportional to the live set rather than to all-time traffic.
	if s.head > len(s.fifo)/2 {
		s.fifo = append(s.fifo[:0], s.fifo[s.head:]...)
		s.head = 0
	}
}
