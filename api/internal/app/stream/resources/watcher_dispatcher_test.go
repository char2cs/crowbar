package resources

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
)

// signal is one Signal call a recordingSignals saw.
type signal struct {
	scope stream.Scope
	kind  string
	data  any
}

type recordingSignals struct{ got chan signal }

func newRecordingSignals() *recordingSignals { return &recordingSignals{got: make(chan signal, 16)} }

func (r *recordingSignals) Signal(scope stream.Scope, kind string, data any) {
	r.got <- signal{scope: scope, kind: kind, data: data}
}

type fakeWorkspaceRepo struct {
	workspacerepo.Workspace
	syncs chan workspacerepo.SyncInput
}

func (f *fakeWorkspaceRepo) ObserveWorkingTreeState(
	_ context.Context,
	in workspacerepo.SyncInput,
	_ time.Time,
) (domain.Workspace, error) {
	f.syncs <- in
	return domain.Workspace{ID: in.ID}, nil
}

// recv reads ch, failing the test instead of hanging it when nothing comes.
func recv[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(hostWaitBound):
		require.FailNow(t, "nothing arrived")
		var zero T
		return zero
	}
}

func TestWatcherDispatcher_FansOutAllThree(t *testing.T) {
	signals := newRecordingSignals()
	repo := &fakeWorkspaceRepo{syncs: make(chan workspacerepo.SyncInput, 1)}
	d := newWatcherDispatcher(signals, repo, func() time.Time { return time.Unix(0, 0) })

	change := domain.FileChangeEvent{WsID: "w1", Path: "a.go"}
	d.OnFileChange(context.Background(), change)
	assert.Equal(t, signal{scope: stream.Scope{Kind: stream.KindFiles, Key: "w1"}, kind: "file_change", data: change}, recv(t, signals.got))

	d.OnGitStatus(context.Background(), "w1", gitdomain.GitStatus{Branch: "main"})
	assert.Equal(t, signal{
		scope: stream.Scope{Kind: stream.KindGit, Key: "w1"}, kind: "git_status",
		data: gitdomain.GitStatus{Branch: "main", Files: []gitdomain.GitFile{}},
	}, recv(t, signals.got), "a clean tree says files: [], never null")

	d.OnSyncWorkingTreeState(context.Background(), enginefs.SyncInput{
		WsID:       "w1",
		Added:      2,
		HasCommits: true,
	})
	gotSync := recv(t, repo.syncs)
	assert.Equal(t, "w1", gotSync.ID)
	assert.Equal(t, 2, gotSync.Added)
	require.True(t, gotSync.HasCommits)
}

func TestWatcherDispatcher_UnreadableGitIsAGitStatusNamingTheReason(t *testing.T) {
	signals := newRecordingSignals()
	d := newWatcherDispatcher(signals, &fakeWorkspaceRepo{}, time.Now)

	d.OnGitStatusUnreadable(context.Background(), "w1", errors.New("not a git repository"))

	got := recv(t, signals.got)
	assert.Equal(t, stream.Scope{Kind: stream.KindGit, Key: "w1"}, got.scope)
	assert.Equal(t, "git_status", got.kind)
	body, err := json.Marshal(got.data)
	require.NoError(t, err)
	assert.JSONEq(t, `{"error":"not a git repository"}`, string(body),
		"the client reloads on a changed frame and shows why git cannot read the workspace")
}
