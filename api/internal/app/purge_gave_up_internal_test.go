package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func recordingOps(t *testing.T) (*operation.Service, chan domain.Operation) {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	opStore, err := storesqlite.NewFromDB[domain.Operation, string](db)
	require.NoError(t, err)
	frames := make(chan domain.Operation, 4)
	return operation.New(opStore, func(op domain.Operation) { frames <- op }, time.Now), frames
}

func TestPurgeGaveUp_RecordsAFailedOperationTheUserSees(t *testing.T) {
	ops, frames := recordingOps(t)
	tomb := &domain.Workspace{ID: "w1", Branch: "feature/a", ProjectID: "p1"}

	purgeGaveUp(ops)(context.Background(), "w1", tomb, errors.New("rm: busy"))

	op := <-frames
	assert.Equal(t, domain.OpWorkspacePurge, op.Kind)
	assert.Equal(t, domain.EntityRef{Kind: domain.EntityWorkspace, ID: "w1"}, op.Subject)
	assert.Equal(t, "p1", op.ProjectID)
	assert.Equal(t, "Remove feature/a", op.Label)
	assert.Equal(t, domain.OpFailed, op.State)
	require.NotNil(t, op.Error)
	assert.Contains(t, op.Error.Message, "rm: busy")
	assert.True(t, strings.HasSuffix(op.Error.Message, "Crowbar retries when it next starts"), op.Error.Message)
}

func TestPurgeGaveUp_ATombNeverObservedIsNamedByTheWorkspace(t *testing.T) {
	ops, frames := recordingOps(t)

	purgeGaveUp(ops)(context.Background(), "w1", nil, errors.New("await tombstone: deadline"))

	assert.Equal(t, "Remove workspace", (<-frames).Label)
}
