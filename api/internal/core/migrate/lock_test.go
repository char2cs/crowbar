package migrate

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestLockHome_SecondLockerIsRefused(t *testing.T) {
	h := testutil.FixtureHome(t)

	first, err := lockHome(h)
	require.NoError(t, err)

	_, err = lockHome(h)
	require.ErrorIs(t, err, ErrHomeLocked)

	require.NoError(t, first.Close())
	third, err := lockHome(h)
	require.NoError(t, err)
	require.NoError(t, third.Close())

	info, err := os.Stat(h.State())
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	if runtime.GOOS != "windows" {
		assert.Zero(t, info.Mode().Perm()&^0o750, "umask may narrow the mode, never widen it")
	}
}

func TestLockHome_CloseIsIdempotent(t *testing.T) {
	lock, err := lockHome(testutil.FixtureHome(t))
	require.NoError(t, err)

	require.NoError(t, lock.Close())
	require.NoError(t, lock.Close())
}

func TestLockHome_FailsWhenStateIsAFile(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state", "not a directory")

	_, err := lockHome(h)

	require.ErrorContains(t, err, "migrate:")
	require.NotErrorIs(t, err, ErrHomeLocked)
}

func TestProbeLock(t *testing.T) {
	h := testutil.FixtureHome(t)

	assert.False(t, probeLock(h), "a home that was never opened has no holder")
	assert.NoDirExists(t, h.State())

	require.NoError(t, os.MkdirAll(h.State(), 0o750))
	before := testutil.TreeSnapshot(t, h)
	assert.False(t, probeLock(h))
	assert.NoFileExists(t, h.LockFile())
	assert.Equal(t, before, testutil.TreeSnapshot(t, h))

	lock, err := lockHome(h)
	require.NoError(t, err)
	assert.True(t, probeLock(h))

	require.NoError(t, lock.Close())
	assert.False(t, probeLock(h))

	again, err := lockHome(h)
	require.NoError(t, err, "probing must not leave the lock held")
	require.NoError(t, again.Close())
}
