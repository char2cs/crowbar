package operation_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

var epoch = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

type fixture struct {
	svc    *operation.Service
	store  store.Store[domain.Operation, string]
	frames chan domain.Operation
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db, err := storesqlite.OpenReadPoolDB(filepath.Join(t.TempDir(), "view.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	opStore, err := storesqlite.NewFromDB[domain.Operation, string](db)
	require.NoError(t, err)
	frames := make(chan domain.Operation, 1024)
	svc := operation.New(opStore, func(op domain.Operation) { frames <- op }, func() time.Time { return epoch })
	return fixture{svc: svc, store: opStore, frames: frames}
}

// until reads frames until one for id is settled, returning every frame for id.
func (f fixture) until(t *testing.T, id string) []domain.Operation {
	t.Helper()
	var got []domain.Operation
	for op := range f.frames {
		if op.ID != id {
			continue
		}
		got = append(got, op)
		if op.State == domain.OpSucceeded || op.State == domain.OpFailed {
			return got
		}
	}
	return got
}

func (f fixture) row(t *testing.T, id string) domain.Operation {
	t.Helper()
	got, err := f.store.FindByKey(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, got)
	return *got
}

func pushSpec(id string) operation.Spec {
	return operation.Spec{
		ID: id, Kind: domain.OpGitPush, ProjectID: "p1", Label: "Push main",
		Subject: domain.EntityRef{Kind: domain.EntityWorkspace, ID: "ws1"},
	}
}

func succeed(context.Context, operation.Progress) error { return nil }

func TestService_StartRecordsAcceptedThenRunsTheWork(t *testing.T) {
	f := newFixture(t)
	id := uuid.NewString()

	op, err := f.svc.Start(context.Background(), pushSpec(id), succeed)
	require.NoError(t, err)
	assert.Equal(t, domain.OpAccepted, op.State)

	frames := f.until(t, id)
	require.Len(t, frames, 3)
	for i, want := range []domain.OperationState{domain.OpAccepted, domain.OpRunning, domain.OpSucceeded} {
		assert.Equal(t, want, frames[i].State)
		assert.Equal(t, i+1, frames[i].Version)
	}
	row := f.row(t, id)
	assert.Equal(t, frames[2].State, row.State)
	assert.Equal(t, frames[2].Version, row.Version)
	assert.Equal(t, "Push main", row.Label)
	assert.Equal(t, domain.EntityRef{Kind: domain.EntityWorkspace, ID: "ws1"}, row.Subject)
}

func TestService_FailedWorkCarriesItsProblem(t *testing.T) {
	f := newFixture(t)
	id := uuid.NewString()

	_, err := f.svc.Start(context.Background(), pushSpec(id), func(context.Context, operation.Progress) error {
		return problem.New(problem.Conflict, "held")
	})
	require.NoError(t, err)

	frames := f.until(t, id)
	last := frames[len(frames)-1]
	assert.Equal(t, domain.OpFailed, last.State)
	require.NotNil(t, last.Error)
	assert.Equal(t, problem.Problem{Code: problem.Conflict, Message: "held"}, *last.Error)
	assert.Equal(t, last.Error, f.row(t, id).Error)
}

func TestService_AFailedItemFailsTheOperationAsPartial(t *testing.T) {
	f := newFixture(t)
	id := uuid.NewString()
	ref := func(id string) domain.EntityRef { return domain.EntityRef{Kind: domain.EntityChat, ID: id} }

	_, err := f.svc.Start(context.Background(), pushSpec(id), func(_ context.Context, p operation.Progress) error {
		p.Item(domain.OperationItem{Ref: ref("c1"), Label: "a", State: domain.OpSucceeded})
		p.Item(domain.OperationItem{Ref: ref("c2"), Label: "b", State: domain.OpRunning})
		p.Item(domain.OperationItem{
			Ref: ref("c2"), Label: "b", State: domain.OpFailed,
			Error: &problem.Problem{Code: problem.Conflict, Message: "taken"},
		})
		return nil
	})
	require.NoError(t, err)

	last := f.until(t, id)
	op := last[len(last)-1]
	assert.Equal(t, domain.OpFailed, op.State)
	require.NotNil(t, op.Error)
	assert.Equal(t, problem.Partial, op.Error.Code)
	assert.Equal(t, "1 of 2 failed", op.Error.Message)
	require.Len(t, op.Items, 2, "an item with the same label replaces the earlier one")
	assert.Equal(t, "taken", op.Items[1].Error.Message)
}

func TestService_StartIsIdempotentOnTheKey(t *testing.T) {
	f := newFixture(t)
	id := uuid.NewString()
	runs := make(chan struct{}, 2)
	release := make(chan struct{})
	work := func(context.Context, operation.Progress) error {
		runs <- struct{}{}
		<-release
		return nil
	}

	first, err := f.svc.Start(context.Background(), pushSpec(id), work)
	require.NoError(t, err)
	<-runs
	again, err := f.svc.Start(context.Background(), pushSpec(id), work)
	require.NoError(t, err)
	close(release)
	f.until(t, id)
	require.NoError(t, f.svc.Shutdown(context.Background()))

	assert.Equal(t, first.ID, again.ID)
	assert.Equal(t, domain.OpRunning, again.State, "the retry answers the first operation as it stands")
	assert.Empty(t, runs, "the work ran once")
}

func TestService_AReusedKeyForOtherWorkIsAConflict(t *testing.T) {
	f := newFixture(t)
	id := uuid.NewString()
	_, err := f.svc.Start(context.Background(), pushSpec(id), succeed)
	require.NoError(t, err)

	other := pushSpec(id)
	other.Subject.ID = "ws2"
	_, err = f.svc.Start(context.Background(), other, succeed)
	assert.Equal(t, problem.OperationIDConflict, problem.CodeOf(err))
}

func TestService_AMalformedKeyIsInvalid(t *testing.T) {
	f := newFixture(t)

	_, err := f.svc.Start(context.Background(), pushSpec("x"), succeed)
	assert.Equal(t, problem.InvalidArgument, problem.CodeOf(err))

	op, err := f.svc.Start(context.Background(), pushSpec(""), succeed)
	require.NoError(t, err)
	_, err = uuid.Parse(op.ID)
	assert.NoError(t, err, "an absent key is minted")
}

func TestService_StartRunsValidateOnlyForANewKey(t *testing.T) {
	f := newFixture(t)
	refused := errors.New("chat id taken")
	spec := pushSpec(uuid.NewString())
	spec.Validate = func(context.Context) error { return refused }

	_, err := f.svc.Start(context.Background(), spec, succeed)
	require.ErrorIs(t, err, refused)
	got, err := f.store.FindByKey(context.Background(), spec.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "a refused start writes no row")

	spec.Validate = nil
	_, err = f.svc.Start(context.Background(), spec, succeed)
	require.NoError(t, err)
	f.until(t, spec.ID)
	spec.Validate = func(context.Context) error { return refused }
	op, err := f.svc.Start(context.Background(), spec, succeed)
	require.NoError(t, err, "a known key skips Validate")
	assert.Equal(t, domain.OpSucceeded, op.State)
}

func TestService_APanickingWorkFailsTheOperation(t *testing.T) {
	f := newFixture(t)
	id := uuid.NewString()

	_, err := f.svc.Start(context.Background(), pushSpec(id), func(context.Context, operation.Progress) error {
		panic("boom")
	})
	require.NoError(t, err)

	frames := f.until(t, id)
	last := frames[len(frames)-1]
	assert.Equal(t, domain.OpFailed, last.State)
	assert.Equal(t, problem.Problem{Code: problem.Internal, Message: "internal error: boom"}, *last.Error)
}

// A graceful stop cancels at once: work in flight at quit must not eat the
// writer drain the terminal and the aggregates still need.
func TestService_ShutdownCancelsAndRecordsTheWorkInterrupted(t *testing.T) {
	f := newFixture(t)
	id := uuid.NewString()
	started := make(chan struct{})
	_, err := f.svc.Start(context.Background(), pushSpec(id), func(ctx context.Context, _ operation.Progress) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	require.NoError(t, err)
	<-started

	require.NoError(t, f.svc.Shutdown(context.Background()))

	row := f.row(t, id)
	assert.Equal(t, domain.OpFailed, row.State)
	assert.Equal(t, problem.Interrupted, row.Error.Code)
}

func TestService_ShutdownReturnsWhenItsCtxEndsThoughWorkIgnoresCancel(t *testing.T) {
	f := newFixture(t)
	release := make(chan struct{})
	started := make(chan struct{})
	_, err := f.svc.Start(context.Background(), pushSpec(uuid.NewString()), func(context.Context, operation.Progress) error {
		close(started)
		<-release
		return nil
	})
	require.NoError(t, err)
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.ErrorIs(t, f.svc.Shutdown(ctx), context.Canceled)
	close(release)
}

// Every admitted operation is cancellable the moment Start returns, so a stop
// racing a start never leaves work it did not cancel.
func TestService_ShutdownRacingStartsCancelsEveryAdmittedOperation(t *testing.T) {
	for range 50 {
		f := newFixture(t)
		admitted := make(chan string, 1)
		go func() {
			op, err := f.svc.Start(context.Background(), pushSpec(""), func(ctx context.Context, _ operation.Progress) error {
				<-ctx.Done()
				return ctx.Err()
			})
			if err != nil {
				op.ID = ""
			}
			admitted <- op.ID
		}()
		require.NoError(t, f.svc.Shutdown(context.Background()))
		if id := <-admitted; id != "" {
			require.NoError(t, f.svc.Shutdown(context.Background()))
			assert.Equal(t, domain.OpFailed, f.row(t, id).State)
		}
	}
}

// The service keeps the cap as operations settle, not only at boot.
func TestService_SettlingPrunesPastTheNewest500(t *testing.T) {
	f := newFixture(t)
	for i := range 500 {
		seed(t, f, fmt.Sprintf("op-%03d", i), domain.OpSucceeded, epoch.Add(-time.Duration(1000-i)*time.Minute))
	}
	id := uuid.NewString()
	_, err := f.svc.Start(context.Background(), pushSpec(id), succeed)
	require.NoError(t, err)
	f.until(t, id)
	require.NoError(t, f.svc.Shutdown(context.Background()))

	all, err := f.store.FindAll(context.Background())
	require.NoError(t, err)
	assert.Len(t, all, 500)
	gone, err := f.store.FindByKey(context.Background(), "op-000")
	require.NoError(t, err)
	assert.Nil(t, gone, "the oldest settled row goes")
	assert.Equal(t, id, f.row(t, id).ID)
}

// A running row is never pruned: deleting it would lose its idempotency key.
func TestService_SettlingNeverPrunesARunningOperation(t *testing.T) {
	f := newFixture(t)
	seed(t, f, "running", domain.OpRunning, epoch.Add(-48*time.Hour))
	for i := range 500 {
		seed(t, f, fmt.Sprintf("op-%03d", i), domain.OpSucceeded, epoch.Add(-time.Duration(1000-i)*time.Minute))
	}
	id := uuid.NewString()
	_, err := f.svc.Start(context.Background(), pushSpec(id), succeed)
	require.NoError(t, err)
	f.until(t, id)
	require.NoError(t, f.svc.Shutdown(context.Background()))

	assert.Equal(t, domain.OpRunning, f.row(t, "running").State)
	gone, err := f.store.FindByKey(context.Background(), "op-000")
	require.NoError(t, err)
	assert.Nil(t, gone)
}

func TestService_StartAfterShutdownIsUnavailable(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.svc.Shutdown(context.Background()))

	_, err := f.svc.Start(context.Background(), pushSpec(uuid.NewString()), succeed)
	assert.Equal(t, problem.Unavailable, problem.CodeOf(err))
}

func TestService_RunRecordsAndReturnsTheSettledOperation(t *testing.T) {
	f := newFixture(t)
	ran := false

	op, err := f.svc.Run(context.Background(), pushSpec(""), func(context.Context, operation.Progress) error {
		ran = true // the caller's goroutine: no synchronisation needed to read it below
		return nil
	})
	require.NoError(t, err)
	assert.True(t, ran)
	assert.Equal(t, domain.OpSucceeded, op.State)
	assert.Equal(t, op, f.row(t, op.ID))
}

func TestService_FailRecordsDaemonStartedWork(t *testing.T) {
	f := newFixture(t)

	op, err := f.svc.Fail(context.Background(), pushSpec(""), problem.New(problem.Unavailable, "disk gone"))
	require.NoError(t, err)

	frame := <-f.frames
	assert.Equal(t, op, frame)
	assert.Equal(t, domain.OpFailed, frame.State)
	assert.Equal(t, 1, frame.Version)
	assert.Equal(t, problem.Problem{Code: problem.Unavailable, Message: "disk gone"}, *frame.Error)
	assert.Empty(t, f.frames, "one frame")
}

func TestService_DismissMarksASettledOperation(t *testing.T) {
	f := newFixture(t)
	id := uuid.NewString()
	_, err := f.svc.Start(context.Background(), pushSpec(id), succeed)
	require.NoError(t, err)
	settled := f.until(t, id)

	op, err := f.svc.Dismiss(context.Background(), id)
	require.NoError(t, err)
	assert.True(t, op.Dismissed)
	assert.Equal(t, settled[len(settled)-1].Version+1, op.Version)
	assert.Equal(t, op, <-f.frames)
	assert.True(t, f.row(t, id).Dismissed)
}

func TestService_DismissingRunningWorkIsAConflict(t *testing.T) {
	f := newFixture(t)
	id := uuid.NewString()
	release := make(chan struct{})
	_, err := f.svc.Start(context.Background(), pushSpec(id), func(context.Context, operation.Progress) error {
		<-release
		return nil
	})
	require.NoError(t, err)

	_, err = f.svc.Dismiss(context.Background(), id)
	assert.Equal(t, problem.OperationRunning, problem.CodeOf(err))
	close(release)
	f.until(t, id)

	_, err = f.svc.Dismiss(context.Background(), uuid.NewString())
	assert.Equal(t, problem.NotFound, problem.CodeOf(err))
}

func moveSpec(id string) operation.Spec {
	return operation.Spec{
		ID: id, Kind: domain.OpWorkspaceReparent, ProjectID: "p1", Label: "Move a under b",
		Subject: domain.EntityRef{Kind: domain.EntityWorkspace, ID: "ws1"},
	}
}

// Two hierarchy verbs on one workspace would race each other's git and rows:
// the second is refused while the first runs, and admitted once it settled.
func TestService_ASecondWorkspaceVerbWhileOneRunsIsRefused(t *testing.T) {
	f := newFixture(t)
	first := uuid.NewString()
	release := make(chan struct{})
	_, err := f.svc.Start(context.Background(), moveSpec(first), func(context.Context, operation.Progress) error {
		<-release
		return nil
	})
	require.NoError(t, err)

	again, err := f.svc.Start(context.Background(), moveSpec(first), succeed)
	require.NoError(t, err, "a retry of the running key is the same request")
	assert.Equal(t, first, again.ID)
	other := moveSpec(uuid.NewString())
	other.Kind = domain.OpWorkspaceSync
	_, err = f.svc.Start(context.Background(), other, succeed)
	assert.Equal(t, problem.OperationRunning, problem.CodeOf(err))
	push, err := f.svc.Start(context.Background(), pushSpec(uuid.NewString()), succeed)
	require.NoError(t, err, "git verbs are not hierarchy verbs")
	f.until(t, push.ID)

	close(release)
	f.until(t, first)
	other.ID = uuid.NewString()
	next, err := f.svc.Start(context.Background(), other, succeed)
	require.NoError(t, err)
	f.until(t, next.ID)
}

// The boot sweep that finishes a purge clears the failure it reported.
func TestService_DismissFailedClearsTheSubjectsFailures(t *testing.T) {
	f := newFixture(t)
	ref := domain.EntityRef{Kind: domain.EntityWorkspace, ID: "ws1"}
	purge, err := f.svc.Fail(context.Background(), operation.Spec{Kind: domain.OpWorkspacePurge, Subject: ref}, errors.New("busy"))
	require.NoError(t, err)
	<-f.frames
	other, err := f.svc.Fail(context.Background(), pushSpec(""), errors.New("gone"))
	require.NoError(t, err)
	<-f.frames

	require.NoError(t, f.svc.DismissFailed(context.Background(), domain.OpWorkspacePurge, ref))

	assert.True(t, (<-f.frames).Dismissed)
	assert.True(t, f.row(t, purge.ID).Dismissed)
	assert.False(t, f.row(t, other.ID).Dismissed, "another kind's failure is not the purge's to clear")
}
