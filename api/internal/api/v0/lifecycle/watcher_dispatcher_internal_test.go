package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
)

type capturingSubscriber struct {
	files chan domain.FileChangeEvent
	git   chan gitGot
}

type gitGot struct {
	wsID   string
	status gitdomain.GitStatus
}

func newCapturingSubscriber() *capturingSubscriber {
	return &capturingSubscriber{
		files: make(chan domain.FileChangeEvent, 1),
		git:   make(chan gitGot, 1),
	}
}

func (s *capturingSubscriber) PushWorkspace(_ domain.Workspace)  {}
func (s *capturingSubscriber) PushChat(_ hub.ChatStatusEvent)    {}
func (s *capturingSubscriber) PushFile(e domain.FileChangeEvent) { s.files <- e }

func (s *capturingSubscriber) PushGit(
	wsID string,
	status gitdomain.GitStatus,
) {
	s.git <- gitGot{wsID: wsID, status: status}
}

type fakeWorkspaceRepo struct {
	workspacerepo.Workspace
	syncs chan workspacerepo.SyncInput
}

func (f *fakeWorkspaceRepo) SyncWorkingTreeState(
	_ context.Context,
	in workspacerepo.SyncInput,
	_ time.Time,
) (domain.Workspace, error) {
	f.syncs <- in
	return domain.Workspace{ID: in.ID}, nil
}

func TestWatcherDispatcher_FansOutAllThree(t *testing.T) {
	h := hub.NewHub()
	sub := newCapturingSubscriber()
	h.Register(sub)
	repo := &fakeWorkspaceRepo{syncs: make(chan workspacerepo.SyncInput, 1)}
	d := newWatcherDispatcher(h, repo, func() time.Time { return time.Unix(0, 0) })

	d.OnFileChange(context.Background(), domain.FileChangeEvent{WsID: "w1", Path: "a.go"})
	gotFile := <-sub.files
	assert.Equal(t, "a.go", gotFile.Path)

	d.OnGitStatus(context.Background(), "w1", gitdomain.GitStatus{Branch: "main"})
	gotGit := <-sub.git
	assert.Equal(t, "w1", gotGit.wsID)
	assert.Equal(t, "main", gotGit.status.Branch)

	d.OnSyncWorkingTreeState(context.Background(), enginefs.SyncInput{
		WsID:       "w1",
		Added:      2,
		HasCommits: true,
	})
	gotSync := <-repo.syncs
	assert.Equal(t, "w1", gotSync.ID)
	assert.Equal(t, 2, gotSync.Added)
	require.True(t, gotSync.HasCommits)
}
