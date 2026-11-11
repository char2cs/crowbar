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

// TestContainer_AgentToolDepsWireEveryToolGroup is the wiring guard.
//
// agenttools registers no tool whose port is nil, so a group the container forgets
// to hand a dependency to simply does not exist — and the only symptom in a running
// daemon is an agent with fewer tools than it should have, which nothing logs and no
// unit test in agenttools can see (its own fixtures supply their own deps). This
// asserts the PRODUCTION Deps, built by the real newAgentToolDeps over the real
// repositories, advertises the complete surface by name.
//
// Chats, ChatLogs and Lineage are filled in the way production fills them:
// agentusecase.New binds the usecase to itself for all three (see its doc
// comment), so c.AgentChat is the exact value the running daemon's Deps carries
// for each port.
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

// A missing port is a failed start, not a silently narrowed tool list.
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

	// A container with SOME stores wired still refuses a genuinely partial
	// surface: bare's AgentChat==nil check alone can never exercise the
	// Workspace/ReviewThread branches behind it in the same guard. Every store
	// ahead of the one under test is therefore wired for real, and the message is
	// asserted rather than just "an error" — a case that merely trips an earlier
	// guard would pass while proving nothing about the branch it names.
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

// stubBranchRenamer stands in for the workspace usecase in the refusal test,
// where every other port is deliberately nil in turn and this one only has to
// be non-nil so the case under test is the one that fires.
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
