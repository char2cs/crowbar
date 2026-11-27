package ledger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlobs_NamingHoldsCollectionOff(t *testing.T) {
	s, err := OpenBlobs(t.TempDir())
	require.NoError(t, err)

	release := s.Naming()
	assert.False(t, s.refs.TryLock(), "a collection waits for the naming")
	release()
	require.True(t, s.refs.TryLock(), "and starts once it is over")
	s.refs.Unlock()
}
