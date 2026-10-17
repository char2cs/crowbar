package realtime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

type stubGitEngine struct {
	enginegit.Engine
	status  gitdomain.GitStatus
	added   int
	deleted int
}

func (s stubGitEngine) Status(
	_ context.Context,
	_ string,
) (gitdomain.GitStatus, error) {
	return s.status, nil
}

func (s stubGitEngine) ResolveDiffBase(
	_ context.Context,
	_ string,
	_ string,
) gitdomain.DiffBase {
	return gitdomain.DiffBase{Commit: "resolved", HasCommits: true}
}

func (s stubGitEngine) DiffStat(
	_ context.Context,
	_ string,
	_ string,
) (int, int, error) {
	return s.added, s.deleted, nil
}

func TestGitStatusProvider_DelegatesToEngine(t *testing.T) {
	p := &gitStatusProvider{engine: stubGitEngine{
		status:  gitdomain.GitStatus{Branch: "main"},
		added:   3,
		deleted: 1,
	}}

	st, err := p.ComputeStatus(context.Background(), "/repo")
	require.NoError(t, err)
	assert.Equal(t, "main", st.Branch)

	base := p.ResolveDiffBase(context.Background(), "/repo", "main")
	assert.Equal(t, gitdomain.DiffBase{Commit: "resolved", HasCommits: true}, base)

	added, deleted, err := p.DiffStat(context.Background(), "/repo", base.Commit)
	require.NoError(t, err)
	assert.Equal(t, 3, added)
	assert.Equal(t, 1, deleted)
}
