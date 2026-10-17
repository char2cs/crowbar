package migratetest_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

func TestNoPostOpen_RunsNoStep(t *testing.T) {
	ran := false
	impls := migrate.Impls{1: {Apply: func(context.Context, migrate.Env, *migrate.Checkpoints) (migrate.Counts, error) {
		ran = true
		return nil, nil
	}}}

	require.NoError(t, migratetest.NoPostOpen().RunPostOpen(context.Background(), impls))

	assert.False(t, ran)
}

func TestApplied_ReadsWhatTheEngineRecorded(t *testing.T) {
	h := testutil.FixtureHome(t)
	assert.Empty(t, migratetest.Applied(t, h), "a home no run touched has no ledger")

	cat, err := migrate.NewCatalog(migrate.Step{
		Number: 1, Name: "one", Phase: migrate.PreOpen,
		Plan: func(context.Context, migrate.Env) (migrate.Plan, error) { return migrate.Plan{}, nil },
	})
	require.NoError(t, err)
	sess, err := migrate.Begin(context.Background(), h, cat, migrate.Options{Version: "v-test"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sess.Close()) })
	require.NoError(t, sess.RunPreOpen(context.Background(), migrate.Impls{1: {
		Apply: func(context.Context, migrate.Env, *migrate.Checkpoints) (migrate.Counts, error) {
			return migrate.Counts{"rows": 2}, nil
		},
	}}))

	assert.Equal(t, map[int][]migrate.Counts{1: {{"rows": 2}}}, migratetest.Applied(t, h))
}
