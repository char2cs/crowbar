package ws

import "testing"

// TestPush_SlowConsumer_DisconnectsInsteadOfDroppingForever proves H22: when a
// client's send buffer fills (its writePump stalled), the broadcaster must
// disconnect it (so it reconnects and re-syncs via the snapshot) rather than
// silently dropping coalesced git-status/file frames it would never re-receive.
func TestPush_SlowConsumer_DisconnectsInsteadOfDroppingForever(t *testing.T) {
	def := StreamDef[int]{
		Serialize: func(i int) ([]byte, error) { return []byte{byte(i)}, nil },
	}
	b := NewBroadcaster(def)
	cl := &filteredClient[int]{client: newClient(), predicate: func(int) bool { return true }}
	b.register(cl)

	// Nobody drains cl.send: fill the buffer, then overflow it once.
	for i := 0; i < sendBuffer+1; i++ {
		b.Push(i)
	}

	select {
	case <-cl.done:
		// Disconnected on overflow — the client will reconnect and resync.
	default:
		t.Fatal("a slow consumer that overflowed its buffer must be disconnected, not silently starved")
	}
}

// TestRegression_CoalescedStream_NeverDisconnectsOnOverflow is the fix for a
// live-reported bug: an assistant message streamed by a fast provider
// visibly lost words mid-reply, self-correcting only once the turn ended.
// message_delta rides this exact Broadcaster/client machinery — the SAME
// bounded queue TestPush_SlowConsumer_DisconnectsInsteadOfDroppingForever
// proves above — and a burst of high-frequency deltas filled it while the
// browser's own JS thread was momentarily busy, so the client was
// disconnected: correct for a full-state stream, wrong for one that is
// already "the text so far" by construction, where a dropped frame was
// supposed to cost nothing (see hub.BroadcastAgentChatMessageDelta's own doc
// comment). A disconnected client reconnects with NO snapshot for a
// still-streaming message (it deliberately never touches durable storage),
// so whatever text arrived between the disconnect and the reconnect was
// gone from the live view until the persisted ledger message reconciled it
// at turn end. CoalesceKey is the fix: a stream that opts in never queues
// one entry per delta — only the LATEST payload per key is ever pending, so
// the buffer can never fill and the client is never disconnected.
func TestRegression_CoalescedStream_NeverDisconnectsOnOverflow(t *testing.T) {
	def := StreamDef[int]{
		Serialize:   func(i int) ([]byte, error) { return []byte{byte(i)}, nil },
		CoalesceKey: func(int) (string, bool) { return "one-message", true },
	}
	b := NewBroadcaster(def)
	cl := &filteredClient[int]{client: newClient(), predicate: func(int) bool { return true }}
	b.register(cl)

	// Nobody drains anything: push far more than sendBuffer would ever hold
	// (still under 256 so the fake payload byte below never wraps).
	const pushes = sendBuffer * 3
	for i := 0; i < pushes; i++ {
		b.Push(i)
	}

	select {
	case <-cl.done:
		t.Fatal("THE FIX: a coalescible stream must never disconnect on overflow — " +
			"a later value for the same key supersedes the earlier one instead")
	default:
	}

	pending := cl.drainPending()
	if len(pending) != 1 {
		t.Fatalf("one key was ever used, so exactly one payload should be pending, got %d", len(pending))
	}
	if want := byte(pushes - 1); pending[0][0] != want {
		t.Fatalf("expected the LATEST payload %d for the key, got %d — an earlier one must never win", want, pending[0][0])
	}
}

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
