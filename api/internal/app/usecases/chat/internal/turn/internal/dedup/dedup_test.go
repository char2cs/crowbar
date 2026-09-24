package dedup_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn/internal/dedup"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func TestSet_ARetriedDeliveryIsDone(t *testing.T) {
	s := dedup.New(time.Minute, 8, nil)
	h := dedup.Hash("r", "claude", "tool_pre", []byte(`{}`))

	done, err := s.Done("d1", h)
	require.NoError(t, err)
	assert.False(t, done, "a first sighting is new")

	s.Complete("d1", h)
	done, err = s.Done("d1", h)
	require.NoError(t, err)
	assert.True(t, done, "the relay reuses one id across retries: the retry must be absorbed")
}

func TestSet_AReusedIDWithADifferentPayloadIsRefused(t *testing.T) {
	s := dedup.New(time.Minute, 8, nil)
	s.Complete("d1", dedup.Hash("r", "claude", "tool_pre", []byte(`{"a":1}`)))

	_, err := s.Done("d1", dedup.Hash("r", "claude", "tool_pre", []byte(`{"a":2}`)))
	require.ErrorIs(t, err, dedup.ErrPayloadMismatch)
}

func TestSet_EntriesExpireAfterTheTTL(t *testing.T) {
	c := &clock{t: time.Unix(0, 0)}
	s := dedup.New(time.Minute, 8, c.now)
	s.Complete("old", "h")
	c.t = c.t.Add(30 * time.Second)
	s.Complete("new", "h")

	c.t = c.t.Add(31 * time.Second)
	done, _ := s.Done("old", "h")
	assert.False(t, done, "an id past its TTL is forgotten")
	done, _ = s.Done("new", "h")
	assert.True(t, done)
	assert.Equal(t, 1, s.Len())
}

// BenchmarkSet_DoneComplete is one relayed delivery's whole dedup cost. The
// fsync'd journal it replaced measured ~7-9ms per delivery on the same box.
func BenchmarkSet_DoneComplete(b *testing.B) {
	s := dedup.New(dedup.DefaultTTL, dedup.DefaultMax, nil)
	h := dedup.Hash("r", "claude", "tool_pre", []byte(`{"a":1}`))
	b.ReportAllocs()
	for i := range b.N {
		id := fmt.Sprint(i)
		if done, _ := s.Done(id, h); done {
			b.Fatal("fresh id reported done")
		}
		s.Complete(id, h)
	}
}

func TestSet_NeverHoldsMoreThanItsCap(t *testing.T) {
	s := dedup.New(time.Hour, 16, nil)
	for i := range 1000 {
		s.Complete(fmt.Sprint(i), "h")
		require.LessOrEqual(t, s.Len(), 16)
	}
	done, _ := s.Done("999", "h")
	assert.True(t, done, "the newest ids survive")
	done, _ = s.Done("0", "h")
	assert.False(t, done, "the oldest are evicted first")
}
