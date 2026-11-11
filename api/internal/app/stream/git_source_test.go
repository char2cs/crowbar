package stream_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type statuses map[string]gitdomain.GitStatus

func (s statuses) Status(_ context.Context, wsID string) (gitdomain.GitStatus, error) {
	status, ok := s[wsID]
	if !ok {
		return gitdomain.GitStatus{}, errors.New("not a git checkout")
	}
	return status, nil
}

func gitSourceHub(src appstream.Source) *appstream.Hub {
	return newHub(map[stream.ScopeKind]appstream.Source{stream.KindGit: appstream.Unordered(src)}, nil)
}

func TestGitSource_SnapshotIsTheCurrentStatus(t *testing.T) {
	src := appstream.NewGitSource(
		knownWorkspaces{"w1": {ID: "w1"}},
		statuses{"w1": {Branch: "main"}},
	)
	conn := newFakeConn()
	served(t, gitSourceHub(src), conn)

	frames := conn.sub(t, "git:w1")

	require.Len(t, frames, 1)
	assert.Equal(t, stream.FrameSignal, frames[0].T)
	assert.Equal(t, "git_status", frames[0].Kind)
	assert.Equal(t, "git:w1", frames[0].Scope)
	assert.JSONEq(t, `{"branch":"main","ahead":0,"behind":0,"files":[]}`, string(frames[0].Data), "a clean tree says files: [], never null")
}

func TestGitSource_AStatusThatCannotBeReadSnapshotsNothing(t *testing.T) {
	src := appstream.NewGitSource(knownWorkspaces{"w1": {ID: "w1", Kind: domain.WorkspaceKindHome}}, statuses{})
	conn := newFakeConn()
	served(t, gitSourceHub(src), conn)

	assert.Empty(t, conn.sub(t, "git:w1"))
}

func TestGitSource_AnUnknownWorkspaceIsRefused(t *testing.T) {
	src := appstream.NewGitSource(knownWorkspaces{}, statuses{"gone": {}})
	conn := newFakeConn()
	served(t, gitSourceHub(src), conn)

	conn.send(t, stream.OpSub, "git:gone")
	closed := conn.next(t)

	assert.Equal(t, stream.ReasonRefused, closed.Reason)
	assert.Equal(t, problem.NotFound, closed.Problem.Code)
}
