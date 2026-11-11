package readonce_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/readonce"
)

func counted(n *int) func() (string, error) {
	return func() (string, error) { *n++; return "v", nil }
}

func TestDo_AReadBegunAnswersEachKeyOnce(t *testing.T) {
	ctx, end := readonce.Begin(context.Background())
	defer end()
	reads := 0

	for range 3 {
		v, err := readonce.Do(ctx, "k", counted(&reads))
		require.NoError(t, err)
		assert.Equal(t, "v", v)
	}
	_, err := readonce.Do(ctx, "other", counted(&reads))
	require.NoError(t, err)

	assert.Equal(t, 2, reads)
}

func TestDo_OutsideARead_EveryCallReads(t *testing.T) {
	reads := 0
	for range 2 {
		_, err := readonce.Do(context.Background(), "k", counted(&reads))
		require.NoError(t, err)
	}
	assert.Equal(t, 2, reads)
}

// Work the read started in the background outlives it, and must read afresh.
func TestDo_AfterTheReadEnds_EveryCallReads(t *testing.T) {
	ctx, end := readonce.Begin(context.Background())
	reads := 0
	_, err := readonce.Do(ctx, "k", counted(&reads))
	require.NoError(t, err)

	end()
	_, err = readonce.Do(ctx, "k", counted(&reads))
	require.NoError(t, err)

	assert.Equal(t, 2, reads)
}

func TestDo_AFailedReadIsTheReadsAnswer(t *testing.T) {
	ctx, end := readonce.Begin(context.Background())
	defer end()
	boom := errors.New("boom")
	reads := 0
	read := func() (int, error) { reads++; return 0, boom }

	_, err := readonce.Do(ctx, "k", read)
	require.ErrorIs(t, err, boom)
	_, err = readonce.Do(ctx, "k", read)
	require.ErrorIs(t, err, boom)

	assert.Equal(t, 1, reads)
}

func TestDo_ConcurrentCallersShareOneRead(t *testing.T) {
	ctx, end := readonce.Begin(context.Background())
	defer end()
	var mu sync.Mutex
	reads := 0
	read := func() (int, error) {
		mu.Lock()
		defer mu.Unlock()
		reads++
		return 7, nil
	}
	var wg sync.WaitGroup
	got, errs := make([]int, 8), make([]error, 8)
	for i := range got {
		wg.Go(func() { got[i], errs[i] = readonce.Do(ctx, "k", read) })
	}
	wg.Wait()
	assert.Equal(t, []int{7, 7, 7, 7, 7, 7, 7, 7}, got)
	assert.Equal(t, make([]error, 8), errs)
	assert.Equal(t, 1, reads)
}
