package lifecycle

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

type recordingLifecycle struct {
	mu       sync.Mutex
	ensures  []string
	shutdown []string
}

func (r *recordingLifecycle) Ensure(
	_ context.Context,
	wsID string,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensures = append(r.ensures, wsID)
}

func (r *recordingLifecycle) Shutdown(
	_ context.Context,
	wsID string,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shutdown = append(r.shutdown, wsID)
}

func TestLSPManager_FirstEnsuresLastShuts(t *testing.T) {
	rec := &recordingLifecycle{}
	m := NewLSPManager(context.Background(), rec)

	m.Acquire("w1")
	m.Acquire("w1") // second subscriber: no extra Ensure
	m.Release("w1")
	m.Release("w1") // last: Shutdown

	assert.Equal(t, []string{"w1"}, rec.ensures)
	assert.Equal(t, []string{"w1"}, rec.shutdown)
}

func TestLSPManager_BlankAndUnknownAreNoops(t *testing.T) {
	rec := &recordingLifecycle{}
	m := NewLSPManager(context.Background(), rec)

	m.Acquire("")
	m.Release("")
	m.Release("ghost")

	assert.Empty(t, rec.ensures)
	assert.Empty(t, rec.shutdown)
}

func TestLSPManager_PerWorkspaceIndependent(t *testing.T) {
	rec := &recordingLifecycle{}
	m := NewLSPManager(context.Background(), rec)

	m.Acquire("w1")
	m.Acquire("w2")
	m.Release("w1")

	assert.Equal(t, []string{"w1", "w2"}, rec.ensures)
	assert.Equal(t, []string{"w1"}, rec.shutdown)
}

func (r *recordingLifecycle) shutdownCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.shutdown)
}

func TestLSPManager_StopAllShutsEveryHeldWorkspace(t *testing.T) {
	rec := &recordingLifecycle{}
	m := NewLSPManager(context.Background(), rec)

	m.Acquire("w1")
	m.Acquire("w2")

	m.StopAll()

	assert.Equal(t, 2, rec.shutdownCount())

	m.mu.Lock()
	remaining := len(m.refs)
	m.mu.Unlock()
	assert.Equal(t, 0, remaining)
}

func TestLSPManager_StopAllIdempotentAndEmptyNoop(t *testing.T) {
	rec := &recordingLifecycle{}
	m := NewLSPManager(context.Background(), rec)

	m.Acquire("w1")
	m.StopAll()
	assert.Equal(t, 1, rec.shutdownCount())

	m.StopAll() // second call shuts nothing further
	assert.Equal(t, 1, rec.shutdownCount())
}

func TestLSPManager_StopAllOnEmptyIsNoop(t *testing.T) {
	rec := &recordingLifecycle{}
	m := NewLSPManager(context.Background(), rec)

	m.StopAll() // nothing held: no-op
	assert.Equal(t, 0, rec.shutdownCount())
}

func TestLSPManager_AcquireAfterStopAllIsNoop(t *testing.T) {
	rec := &recordingLifecycle{}
	m := NewLSPManager(context.Background(), rec)

	m.StopAll()
	m.Acquire("w1") // late subscribe after shutdown: no Ensure, no ref

	assert.Empty(t, rec.ensures)
	m.mu.Lock()
	remaining := len(m.refs)
	m.mu.Unlock()
	assert.Equal(t, 0, remaining)
}

func TestNoopLSPLifecycle_DoesNothing(t *testing.T) {
	lc := NoopLSPLifecycle()
	lc.Ensure(context.Background(), "w1")
	lc.Shutdown(context.Background(), "w1")
}
