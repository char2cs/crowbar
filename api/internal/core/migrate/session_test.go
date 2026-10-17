package migrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestBegin_NothingPendingWritesNothing(t *testing.T) {
	t.Run("an empty catalog on a fresh home", func(t *testing.T) {
		b := newBench(t)
		s := b.begin()

		require.NoError(t, s.RunPreOpen(context.Background(), nil))
		require.NoError(t, s.RunPostOpen(context.Background(), nil))

		assert.NoFileExists(t, b.home.LayoutFile())
		assert.NoDirExists(t, b.home.MigrateDir())
		assert.Nil(t, s.Halt())
	})

	t.Run("a step that already ran in an earlier session", func(t *testing.T) {
		b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
		var order []int
		first := b.begin()
		require.NoError(t, first.RunPreOpen(context.Background(), tracing(&order, 1)))
		require.NoError(t, first.Close())

		second := b.begin()
		before := testutil.TreeSnapshot(t, b.home)
		require.NoError(t, second.RunPreOpen(context.Background(), tracing(&order, 1)))
		require.NoError(t, second.RunPostOpen(context.Background(), tracing(&order, 1)))

		assert.Equal(t, before, testutil.TreeSnapshot(t, b.home), "the ledger is not appended to")
		assert.Equal(t, []int{1}, order)
	})
}

func TestBegin_RefusesAHomeAnotherSessionHolds(t *testing.T) {
	b := newBench(t)
	first := b.begin()

	_, err := Begin(context.Background(), b.home, b.cat, Options{})
	require.ErrorIs(t, err, ErrHomeLocked)

	require.NoError(t, first.Close())
	b.begin()
}

func TestBegin_ReleasesTheLockWhenItCannotReadTheHome(t *testing.T) {
	tests := []struct {
		name  string
		write func(*bench)
		want  error
	}{
		{
			name: "unreadable LAYOUT",
			write: func(b *bench) {
				testutil.WriteFile(b.tb, b.home, "LAYOUT", "abc\n")
			},
			want: ErrLayoutUnreadable,
		},
		{
			name: "corrupt ledger",
			write: func(b *bench) {
				testutil.WriteFile(b.tb, b.home, "migrate/ledger.jsonl", "not json\nmore\n")
			},
			want: ErrLedgerCorrupt,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newBench(t)
			tc.write(b)

			_, err := Begin(context.Background(), b.home, b.cat, Options{})

			require.ErrorIs(t, err, tc.want)
			require.ErrorContains(t, err, "migrate: begin")
			lock, lockErr := lockHome(b.home)
			require.NoError(t, lockErr, "a failed Begin must not keep the home locked")
			require.NoError(t, lock.Close())
		})
	}
}

func TestBegin_RemovesHalfWrittenBackups(t *testing.T) {
	b := newBench(t)
	testutil.WriteFile(t, b.home, "migrate/backups/20261017T010203.000000000Z.partial/state/x", "x")
	testutil.WriteFile(t, b.home, "migrate/backups/20261016T010203.000000000Z/state/y", "y")

	b.begin()

	assert.NoDirExists(t, filepath.Join(b.home.MigrateDir(), "backups", "20261017T010203.000000000Z.partial"))
	assert.FileExists(t, filepath.Join(b.home.MigrateDir(), "backups", "20261016T010203.000000000Z", "state", "y"))
}

func TestSession_CloseReleasesTheHomeAndWritesNothing(t *testing.T) {
	b := newBench(t, newStep(1, "a", PreOpen, 0, 0))
	s := b.begin()
	require.NoError(t, s.RunPreOpen(context.Background(), healthy(1)))
	before := testutil.TreeSnapshot(t, b.home)

	require.NoError(t, s.Close())
	require.NoError(t, s.Close(), "closing twice is fine")

	assert.Equal(t, before, testutil.TreeSnapshot(t, b.home))
	lock, err := lockHome(b.home)
	require.NoError(t, err)
	require.NoError(t, lock.Close())
}

func TestSession_CloseWithoutALockIsSafe(t *testing.T) {
	require.NoError(t, (&Session{}).Close())
}

func TestOptions_DefaultsAreUsable(t *testing.T) {
	opts := Options{}.withDefaults()

	require.NotNil(t, opts.Clock)
	require.NotNil(t, opts.HaveGit)
	require.NotNil(t, opts.Log)
	free, err := opts.FreeBytes(os.TempDir())
	require.NoError(t, err)
	assert.NotZero(t, free)
	assert.NotPanics(t, func() { opts.HaveGit() })
}
