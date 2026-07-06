package agent_test

import (
	"sync"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestReducer_SpawnBindsFirstSession(t *testing.T) {
	r := agent.NewRegistry()
	r.BindSegment("seg1", "chatA")
	out := r.OnSessionStart("seg1", "sid-0", newID("chatX"))
	require.Equal(t, "bound", out.Kind)
	require.Equal(t, "chatA", out.ChatID)
	require.Equal(t, "sid-0", out.SessionID)
}

func TestReducer_SameSessionIsNoop(t *testing.T) {
	r := agent.NewRegistry()
	r.BindSegment("seg1", "chatA")
	r.OnSessionStart("seg1", "sid-0", newID("x"))
	out := r.OnSessionStart("seg1", "sid-0", newID("x"))
	require.Equal(t, "noop", out.Kind)
}

func TestReducer_UnknownNewIdRegistersNewChat_Case2(t *testing.T) {
	r := agent.NewRegistry()
	r.BindSegment("seg1", "chatA")
	r.OnSessionStart("seg1", "sid-0", newID("x"))            // bound to chatA
	out := r.OnSessionStart("seg1", "sid-1", newID("chatB")) // /clear -> new unknown id
	require.Equal(t, "registered", out.Kind)
	require.Equal(t, "chatB", out.ChatID)
}

func TestReducer_KnownIdMovesFocus_Case1(t *testing.T) {
	r := agent.NewRegistry()
	r.Seed("sid-known", "chatK") // some other chat's session, already known
	r.BindSegment("seg1", "chatA")
	r.OnSessionStart("seg1", "sid-0", newID("x"))               // bound to chatA
	out := r.OnSessionStart("seg1", "sid-known", newID("nope")) // /resume of a known chat
	require.Equal(t, "focus", out.Kind)
	require.Equal(t, "chatK", out.ChatID)
}

func TestReducer_ClearThenResumeSequence(t *testing.T) {
	r := agent.NewRegistry()
	r.BindSegment("seg1", "chatA")
	require.Equal(t, "bound", r.OnSessionStart("seg1", "s0", newID("x")).Kind)
	reg := r.OnSessionStart("seg1", "s1", newID("chatB")) // clear
	require.Equal(t, "registered", reg.Kind)
	// resuming s0 (now known as chatA) => focus chatA
	back := r.OnSessionStart("seg1", "s0", newID("nope"))
	require.Equal(t, "focus", back.Kind)
	require.Equal(t, "chatA", back.ChatID)
}

func TestReducer_ConcurrentNoRace(t *testing.T) {
	r := agent.NewRegistry()
	r.BindSegment("seg", "chatA")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r.OnSessionStart("seg", "s0", newID("x")) }()
	}
	wg.Wait()
}

func newID(id string) func() string { return func() string { return id } }
