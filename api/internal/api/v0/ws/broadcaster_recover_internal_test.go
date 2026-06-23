package ws

import "testing"

// TestPush_PanicInPredicate_ContainedAndFanOutContinues proves H1's per-client
// recovery: a panic in one client's predicate must not propagate out of Push
// (which runs on a watcher/projection goroutine, so it would crash the daemon)
// and must not abort the fan-out to the other, healthy clients.
func TestPush_PanicInPredicate_ContainedAndFanOutContinues(t *testing.T) {
	def := StreamDef[int]{
		Serialize: func(i int) ([]byte, error) { return []byte{byte(i)}, nil },
	}
	b := NewBroadcaster(def)

	bad := &filteredClient[int]{client: newClient(), predicate: func(int) bool { panic("predicate boom") }}
	good := &filteredClient[int]{client: newClient(), predicate: func(int) bool { return true }}
	b.register(bad)
	b.register(good)

	// Must not panic out of Push despite the bad client's predicate panicking.
	b.Push(7)

	select {
	case data := <-good.send:
		if len(data) != 1 || data[0] != 7 {
			t.Fatalf("good client got wrong frame: %v", data)
		}
	default:
		t.Fatal("fan-out aborted: the healthy client never received the frame after a peer's predicate panicked")
	}
}
