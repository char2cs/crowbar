package store

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type flakyStorage struct {
	storage
	failCount int
	calls     int
	err       error
}

func (f *flakyStorage) Save(
	_ context.Context,
	_ domain.ReviewThread,
) error {
	f.calls++
	if f.calls <= f.failCount {
		return f.err
	}
	return nil
}

func TestSaveWithRetry_RetriesTransientThenSucceeds(t *testing.T) {
	st := &flakyStorage{failCount: 2, err: errors.New("disk I/O error")}
	p := &projector{store: st, broadcast: func(_ domain.ReviewThread) {}}
	err := p.saveWithRetry(context.Background(), domain.ReviewThread{ID: "t1"})
	require.NoError(t, err)
	assert.Equal(t, 3, st.calls)
}

func TestSaveWithRetry_PersistentTransientGivesUp(t *testing.T) {
	st := &flakyStorage{failCount: 5, err: errors.New("disk I/O error")}
	p := &projector{store: st, broadcast: func(_ domain.ReviewThread) {}}
	err := p.saveWithRetry(context.Background(), domain.ReviewThread{ID: "t1"})
	require.Error(t, err)
	assert.Equal(t, 3, st.calls)
}

func TestSaveWithRetry_NonTransientNotRetried(t *testing.T) {
	st := &flakyStorage{failCount: 5, err: errors.New("constraint violation")}
	p := &projector{store: st, broadcast: func(_ domain.ReviewThread) {}}
	err := p.saveWithRetry(context.Background(), domain.ReviewThread{ID: "t1"})
	require.Error(t, err)
	assert.Equal(t, 1, st.calls)
}
