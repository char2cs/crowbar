package registry

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/session"
)

func newTestSession(
	t *testing.T,
	id string,
) *session.Session {
	t.Helper()
	dir := t.TempDir()
	s, err := session.New(id, "/bin/sh", dir, os.Environ())
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	return s
}

func TestRegistry_AddGet(t *testing.T) {
	r := New()
	s := newTestSession(t, "s1")

	r.Add("s1", s)

	got, ok := r.Get("s1")
	assert.True(t, ok)
	assert.Equal(t, s, got)
}

func TestRegistry_GetMissing(t *testing.T) {
	r := New()
	_, ok := r.Get("nope")
	assert.False(t, ok)
}

func TestRegistry_Remove(t *testing.T) {
	r := New()
	s := newTestSession(t, "s2")
	r.Add("s2", s)
	r.Remove("s2")

	_, ok := r.Get("s2")
	assert.False(t, ok)
}

func TestRegistry_RemoveMissing(t *testing.T) {
	r := New()
	// Must not panic.
	r.Remove("ghost")
}

func TestRegistry_List(t *testing.T) {
	r := New()
	s1 := newTestSession(t, "a")
	s2 := newTestSession(t, "b")
	r.Add("a", s1)
	r.Add("b", s2)

	ids := r.List()
	assert.ElementsMatch(t, []string{"a", "b"}, ids)
}

func TestRegistry_ListEmpty(t *testing.T) {
	r := New()
	assert.Empty(t, r.List())
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := New()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "concurrent"
			s := newTestSession(t, id)
			r.Add(id, s)
			_, _ = r.Get(id)
			r.Remove(id)
		}(i)
	}
	wg.Wait()
}

func TestRegistry_ErrSessionNotFound(t *testing.T) {
	assert.EqualError(t, ErrSessionNotFound, "registry: session not found")
}
