package ingress

import (
	"testing"
)

func TestLock_SerialisesOneChatAndNotOthers(t *testing.T) {
	locks := NewLocks()
	unlock := locks.Lock("chat-1")

	acquired := make(chan struct{})
	go func() {
		defer locks.Lock("chat-1")()
		close(acquired)
	}()
	locks.Lock("chat-2")() // another chat is never held up by this one

	select {
	case <-acquired:
		t.Fatal("a second holder of chat-1 got in while the first held it")
	default:
	}
	unlock()
	unlock() // a second release is a no-op, never a release of the next holder
	<-acquired
	locks.Lock("chat-1")()

	locks.mu.Lock()
	defer locks.mu.Unlock()
	if len(locks.chats) != 0 {
		t.Fatalf("a chat no one holds keeps no entry: %v", locks.chats)
	}
}
