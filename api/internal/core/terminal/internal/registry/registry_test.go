package registry

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
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

	r.Add("s1", "chat1", s)

	got, ok := r.Get("s1")
	assert.True(t, ok)
	assert.Equal(t, s, got)
}

func TestRegistry_AddWithChat(t *testing.T) {
	r := New()
	s := newTestSession(t, "s1")

	r.Add("s1", "chat1", s)

	assert.ElementsMatch(t, []string{"s1"}, r.ListByChat("chat1"))
}

func TestRegistry_ListByChat_ScopesToChat(t *testing.T) {
	r := New()
	r.Add("a", "chat1", newTestSession(t, "a"))
	r.Add("b", "chat1", newTestSession(t, "b"))
	r.Add("c", "chat2", newTestSession(t, "c"))

	assert.ElementsMatch(t, []string{"a", "b"}, r.ListByChat("chat1"))
	assert.ElementsMatch(t, []string{"c"}, r.ListByChat("chat2"))
}

func TestRegistry_ListByChat_EmptyForUnknownChat(t *testing.T) {
	r := New()
	r.Add("a", "chat1", newTestSession(t, "a"))

	assert.Empty(t, r.ListByChat("unknown"))
}

func TestRegistry_Remove_DropsFromChatIndex(t *testing.T) {
	r := New()
	r.Add("a", "chat1", newTestSession(t, "a"))
	r.Add("b", "chat1", newTestSession(t, "b"))

	r.Remove("a")

	assert.ElementsMatch(t, []string{"b"}, r.ListByChat("chat1"))

	r.Remove("b")
	assert.Empty(t, r.ListByChat("chat1"))
}

func TestRegistry_GetMissing(t *testing.T) {
	r := New()
	_, ok := r.Get("nope")
	assert.False(t, ok)
}

func TestRegistry_Remove(t *testing.T) {
	r := New()
	s := newTestSession(t, "s2")
	r.Add("s2", "chat1", s)
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
	r.Add("a", "chat1", s1)
	r.Add("b", "chat1", s2)

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
			r.Add(id, "chat1", s)
			_, _ = r.Get(id)
			r.Remove(id)
		}(i)
	}
	wg.Wait()
}

func TestRegistry_ErrSessionNotFound(t *testing.T) {
	assert.EqualError(t, ErrSessionNotFound, "registry: session not found")
}

// TestRegistry_ChatID covers both ChatID branches: an unknown id returns
// ("", false); a registered id returns its owning chat and true.
func TestRegistry_ChatID(t *testing.T) {
	r := New()

	chatID, ok := r.ChatID("missing")
	assert.False(t, ok, "an unknown session id must report not-found")
	assert.Empty(t, chatID)

	s := newTestSession(t, "sid-chat")
	r.Add("sid-chat", "chat-42", s)

	chatID, ok = r.ChatID("sid-chat")
	assert.True(t, ok, "a registered session id must resolve its chat")
	assert.Equal(t, "chat-42", chatID)
}
