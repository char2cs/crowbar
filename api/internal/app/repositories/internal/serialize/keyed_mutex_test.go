package serialize_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
)

func TestKeyedMutex_SerializesSameKey(t *testing.T) {
	var km serialize.KeyedMutex
	const n = 64
	var counter int
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			km.Lock("same")
			defer km.Unlock("same")
			counter++
		}()
	}
	wg.Wait()
	assert.Equal(t, n, counter, "every same-key holder must run under mutual exclusion")
}

func TestKeyedMutex_DistinctKeysDoNotDeadlock(t *testing.T) {
	var km serialize.KeyedMutex
	var wg sync.WaitGroup
	for i := 0; i < 128; i++ {
		key := string(rune('a' + i%26))
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			km.Lock(k)
			km.Unlock(k)
		}(key)
	}
	wg.Wait()
}
