package tree_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

func TestRegression_PlaceChat_TiedAnchorsSortByWorkspaceCreatedAtNotID(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	gitStatus.SetRepo(workspaceID, repoID)
	gitStatus.SetDefault(repoID, workspaceID)
	for id, at := range map[string]time.Time{"zz-older": time.Unix(100, 0), "aa-newer": time.Unix(200, 0)} {
		gitStatus.SetRepo(id, repoID)
		gitStatus.SetBranch(id, true)
		gitStatus.SetCreatedAt(id, at)
		nodes.Rows = append(nodes.Rows, dtree.Node{ID: id, Kind: dtree.KindWorkspace, ParentID: "", Order: 0})
	}
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, CreatedAt: time.Unix(300, 0)},
	)
	nodes.Rows = append(nodes.Rows, dtree.Node{ID: "c1", Kind: dtree.KindChat, ParentID: "", Order: 2})

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1", tree.PlaceInput{ParentID: name(""), Order: index(1)})
	require.NoError(t, err)
	older, newer, c1 := nodeRowFor(t, nodes, "zz-older"), nodeRowFor(t, nodes, "aa-newer"), nodeRowFor(t, nodes, "c1")
	assert.Equal(t, 0, older.Order, "the branch created first is drawn first")
	assert.Equal(t, 1, c1.Order)
	assert.Equal(t, 2, newer.Order)
}

func TestRegression_PlaceChat_CountsALegacyNodelessLockedBranch(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	gitStatus.SetRepo(workspaceID, repoID)
	gitStatus.SetDefault(repoID, workspaceID)
	gitStatus.SetRepo("legacy-locked", repoID)
	gitStatus.SetBranch("legacy-locked", true)
	gitStatus.SetCreatedAt("legacy-locked", time.Unix(100, 0))
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, CreatedAt: time.Unix(300, 0)},
		domain.Chat{ID: "c2", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, CreatedAt: time.Unix(400, 0)},
	)
	nodes.Rows = []dtree.Node{
		{ID: "c1", Kind: dtree.KindChat, ParentID: "", Order: 1},
		{ID: "c2", Kind: dtree.KindChat, ParentID: "", Order: 2},
	}

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2", tree.PlaceInput{ParentID: name(""), Order: index(1)})
	require.NoError(t, err)
	anchor := nodeRowFor(t, nodes, "legacy-locked")
	assert.Equal(t, dtree.KindWorkspace, anchor.Kind, "the first write under the level mints the legacy anchor's Node")
	assert.Equal(t, 0, anchor.Order)
	assert.Equal(t, 1, nodeRowFor(t, nodes, "c2").Order)
	assert.Equal(t, 2, nodeRowFor(t, nodes, "c1").Order)
}

func TestPlaceChat_AFailedAnchorReadIsReturned(t *testing.T) {
	for label, fail := range map[string]func(*mocks.AgentWorkspaceGitStatus, error){
		"created at":  func(g *mocks.AgentWorkspaceGitStatus, err error) { g.CreatedAtErr = err },
		"fork parent": func(g *mocks.AgentWorkspaceGitStatus, err error) { g.ForkParentErr = err },
	} {
		t.Run(label, func(t *testing.T) {
			chats, _, nodes, gitStatus, uc := newHomeUsecaseFull(t)
			gitStatus.SetRepo(workspaceID, repoID)
			gitStatus.SetDefault(repoID, workspaceID)
			gitStatus.SetRepo("legacy-locked", repoID)
			gitStatus.SetBranch("legacy-locked", true)
			chats.Rows = append(chats.Rows,
				domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, CreatedAt: time.Unix(300, 0)},
			)
			nodes.Rows = []dtree.Node{{ID: "c1", Kind: dtree.KindChat, ParentID: "", Order: 1}}
			cause := errors.New("workspace read down")
			fail(gitStatus, cause)

			_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1", tree.PlaceInput{ParentID: name(""), Order: index(0)})

			require.ErrorIs(t, err, cause)
		})
	}
}

func TestPlaceChat_ANodeWhoseWorkspaceIsGoneIsSkipped(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	gitStatus.SetRepo(workspaceID, repoID)
	gitStatus.SetDefault(repoID, workspaceID)
	for i, id := range []string{"p", "c1", "c2"} {
		chats.Rows = append(chats.Rows, domain.Chat{
			ID: id, Type: domain.ChatTypeChat, WorkspaceID: workspaceID, CreatedAt: time.Unix(int64(300+i), 0),
		})
	}
	nodes.Rows = []dtree.Node{
		{ID: "p", Kind: dtree.KindChat, ParentID: "", Order: 0},
		{ID: "gone-ws", Kind: dtree.KindWorkspace, ParentID: "p", Order: 0},
		{ID: "c1", Kind: dtree.KindChat, ParentID: "p", Order: 1},
		{ID: "c2", Kind: dtree.KindChat, ParentID: "p", Order: 2},
	}

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2", tree.PlaceInput{ParentID: name("p"), Order: index(0)})

	require.NoError(t, err)
	assert.Equal(t, 0, nodeRowFor(t, nodes, "c2").Order)
	assert.Equal(t, 1, nodeRowFor(t, nodes, "c1").Order)
}
