package ingress

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lockIn takes chatID's lock in a goroutine and closes the returned channel
// once it holds it; the lock is released by calling what release receives.
func lockIn(locks *Locks, chatID string) (holding <-chan struct{}, release <-chan func()) {
	got, unlocks := make(chan struct{}), make(chan func(), 1)
	go func() {
		unlock, err := locks.Lock(context.Background(), chatID)
		if err == nil {
			unlocks <- unlock
			close(got)
		}
	}()
	return got, unlocks
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestLock_SerialisesOneChatAndNotOthers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		locks := NewLocks()
		unlock, err := locks.Lock(t.Context(), "chat-1")
		require.NoError(t, err)

		second, releaseSecond := lockIn(locks, "chat-1")
		synctest.Wait()
		require.False(t, isClosed(second), "a second holder of chat-1 got in while the first held it")
		other, err := locks.Lock(t.Context(), "chat-2")
		require.NoError(t, err, "another chat is never held up by this one")
		other()

		unlock()
		unlock() // a second release is a no-op, never a release of the next holder
		synctest.Wait()
		require.True(t, isClosed(second), "a released chat goes to the holder waiting on it")
		third, releaseThird := lockIn(locks, "chat-1")
		synctest.Wait()
		require.False(t, isClosed(third), "the waiter that took chat-1 still holds it")

		(<-releaseSecond)()
		synctest.Wait()
		require.True(t, isClosed(third))
		(<-releaseThird)()
		assert.Empty(t, locks.chats, "a chat no one holds keeps no entry")
	})
}

func TestLock_AWaiterWhoseContextEndsGivesUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		locks := NewLocks()
		unlock, err := locks.Lock(t.Context(), "chat-1")
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		gaveUp := make(chan error, 1)
		go func() {
			_, err := locks.Lock(ctx, "chat-1")
			gaveUp <- err
		}()
		synctest.Wait()

		cancel()

		require.ErrorIs(t, <-gaveUp, context.Canceled)
		unlock()
		_, err = locks.Lock(ctx, "chat-1")
		require.ErrorIs(t, err, context.Canceled, "an ended context never takes the lock, even a free one")
		assert.Empty(t, locks.chats, "a waiter that gave up keeps no entry")
	})
}
