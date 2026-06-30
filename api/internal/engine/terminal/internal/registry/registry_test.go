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
	s, err := session.New(id, "/bin/sh", dir, "", os.Environ(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	return s
}

func TestRegistry_AddGet(t *testing.T) {
	r := New()
	s := newTestSession(t, "s1")

	r.Add("s1", "ws1", s)

	got, ok := r.Get("s1")
	assert.True(t, ok)
	assert.Equal(t, s, got)
}

func TestRegistry_AddWithWorkspace(t *testing.T) {
	r := New()
	s := newTestSession(t, "s1")

	r.Add("s1", "ws1", s)

	assert.ElementsMatch(t, []string{"s1"}, r.ListByWorkspace("ws1"))
}

func TestRegistry_ListByWorkspace_ScopesToWorkspace(t *testing.T) {
	r := New()
	r.Add("a", "ws1", newTestSession(t, "a"))
	r.Add("b", "ws1", newTestSession(t, "b"))
	r.Add("c", "ws2", newTestSession(t, "c"))

	assert.ElementsMatch(t, []string{"a", "b"}, r.ListByWorkspace("ws1"))
	assert.ElementsMatch(t, []string{"c"}, r.ListByWorkspace("ws2"))
}

func TestRegistry_ListByWorkspace_EmptyForUnknownWs(t *testing.T) {
	r := New()
	r.Add("a", "ws1", newTestSession(t, "a"))

	assert.Empty(t, r.ListByWorkspace("unknown"))
}

func TestRegistry_Remove_DropsFromWorkspaceIndex(t *testing.T) {
	r := New()
	r.Add("a", "ws1", newTestSession(t, "a"))
	r.Add("b", "ws1", newTestSession(t, "b"))

	r.Remove("a")

	assert.ElementsMatch(t, []string{"b"}, r.ListByWorkspace("ws1"))

	r.Remove("b")
	assert.Empty(t, r.ListByWorkspace("ws1"))
}

func TestRegistry_GetMissing(t *testing.T) {
	r := New()
	_, ok := r.Get("nope")
	assert.False(t, ok)
}

func TestRegistry_Remove(t *testing.T) {
	r := New()
	s := newTestSession(t, "s2")
	r.Add("s2", "ws1", s)
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
	r.Add("a", "ws1", s1)
	r.Add("b", "ws1", s2)

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
			r.Add(id, "ws1", s)
			_, _ = r.Get(id)
			r.Remove(id)
		}(i)
	}
	wg.Wait()
}

func TestRegistry_ErrSessionNotFound(t *testing.T) {
	assert.EqualError(t, ErrSessionNotFound, "registry: session not found")
}
