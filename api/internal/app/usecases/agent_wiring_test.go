package usecases_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestContainer_AgentToolDepsWireEveryToolGroup(t *testing.T) {
	repos, gormStores, eng := newContainerDeps(t)
	c, err := usecases.New(repos, gormStores, eng, func() (string, error) { return t.TempDir(), nil }, noopThreadBroadcast)
	require.NoError(t, err)

	minter, err := agentusecase.NewTokenMinter()
	require.NoError(t, err)
	deps, err := usecases.NewAgentToolDepsForTest(minter, repos, c.BranchReview, noopThreadBroadcast, c.Workspace)
	require.NoError(t, err)
	deps.Chats = c.AgentChat
	deps.ChatLogs = c.AgentChat
	deps.Lineage = c.AgentChat

	names := []string{}
	for _, tool := range agentusecase.NewToolSet(deps, "RUN", minter.Mint("RUN")).Tools() {
		names = append(names, tool.Name)
	}
	require.ElementsMatch(t, []string{
		"set_chat_title",
		"set_branch_name",
		"list_review_threads",
		"wait_for_review_comments",
		"get_review_scope",
		"post_review_comment",
		"reply_to_review_thread",
		"resolve_review_thread",
		"list_workspaces",
		"get_chat_log",
	}, names, "the production agent tool surface is incomplete — a port is unwired in newAgentToolDeps")
}

func TestContainer_AgentToolDeps_RefusesAPartialSurface(t *testing.T) {
	repos, _, _ := newContainerDeps(t)
	minter, err := agentusecase.NewTokenMinter()
	require.NoError(t, err)
	review := stubReviewReaderForContainer{}

	_, err = usecases.NewAgentToolDepsForTest(minter, repos, nil, noopThreadBroadcast, stubBranchRenamer{})
	require.Error(t, err, "no review reader")

	_, err = usecases.NewAgentToolDepsForTest(minter, repos, review, nil, stubBranchRenamer{})
	require.Error(t, err, "no thread broadcaster")

	_, err = usecases.NewAgentToolDepsForTest(nil, repos, review, noopThreadBroadcast, stubBranchRenamer{})
	require.Error(t, err, "no token minter")

	_, err = usecases.NewAgentToolDepsForTest(minter, repos, review, noopThreadBroadcast, nil)
	require.Error(t, err, "no workspace usecase")

	bare := &repositories.Container{}
	_, err = usecases.NewAgentToolDepsForTest(minter, bare, review, noopThreadBroadcast, stubBranchRenamer{})
	require.Error(t, err, "no repository stores")

	chatOnly := &repositories.Container{AgentChat: repos.AgentChat}
	_, err = usecases.NewAgentToolDepsForTest(minter, chatOnly, review, noopThreadBroadcast, stubBranchRenamer{})
	require.ErrorContains(t, err, "no workspace store")

	noReviewThread := &repositories.Container{
		AgentChat: repos.AgentChat,
		Workspace: repos.Workspace,
	}
	_, err = usecases.NewAgentToolDepsForTest(minter, noReviewThread, review, noopThreadBroadcast, stubBranchRenamer{})
	require.ErrorContains(t, err, "no review thread store")
}

type stubBranchRenamer struct{}

func (stubBranchRenamer) RenameBranch(
	_ context.Context,
	_ string,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

type stubReviewReaderForContainer struct{}

func (stubReviewReaderForContainer) GetScope(
	_ context.Context,
	_ domain.Workspace,
) (gitdomain.ReviewScope, error) {
	return gitdomain.ReviewScope{}, nil
}

func (stubReviewReaderForContainer) GetOutline(
	_ context.Context,
	_ string,
	_ string,
) ([]gitdomain.FileOutline, error) {
	return nil, nil
}
