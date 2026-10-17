package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	"github.com/char2cs/crowbar/api/internal/core/bootgate"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// A session no step has started on yet has no progress to name, so the gate
// still says Crowbar is starting.
func TestBootReport_IsStartingUntilAStepStarts(t *testing.T) {
	sess, err := migrations.Begin(t.Context(), testutil.FixtureHome(t), migrate.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sess.Close()) })
	b := &bootReport{}
	b.session.Store(sess)

	assert.Equal(t, bootgate.Report{State: bootgate.StateMigrating, Message: startingMessage}, b.report())
}
