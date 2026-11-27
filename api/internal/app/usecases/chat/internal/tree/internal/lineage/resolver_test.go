package lineage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree/internal/lineage"
	"github.com/char2cs/crowbar/api/internal/domain"
)

const workspaceID = "ws-1"

type stubChats struct {
	keyed   map[string]domain.Chat
	listed  []domain.Chat
	getErr  error
	list    error
	lists   int
	missing error
}

func (s *stubChats) LoadChat(
	_ context.Context,
	id string,
) (domain.Chat, error) {
	if s.getErr != nil {
		return domain.Chat{}, s.getErr
	}
	chat, ok := s.keyed[id]
	if !ok && s.missing != nil {
		return domain.Chat{}, s.missing
	}
	if !ok {
		return domain.Chat{}, errors.New("no such chat")
	}
	return chat, nil
}

func (s *stubChats) ListByWorkspace(
	_ context.Context,
	wsID string,
) ([]domain.Chat, error) {
	s.lists++
	if s.list != nil {
		return nil, s.list
	}
	var out []domain.Chat
	for _, row := range s.listed {
		if row.WorkspaceID == wsID {
			out = append(out, row)
		}
	}
	return out, nil
}

func chat(
	id string,
	parentID string,
) domain.Chat {
	return domain.Chat{ID: id, Type: domain.ChatTypeChat, WorkspaceID: workspaceID, ParentID: parentID}
}

func folder(
	id string,
	parentID string,
) domain.Chat {
	return domain.Chat{ID: id, Type: domain.ChatTypeFolder, ParentID: parentID}
}

func tree(
	rows []domain.Chat,
) (*stubChats, *lineage.Resolver) {
	keyed := make(map[string]domain.Chat, len(rows))
	for _, r := range rows {
		keyed[r.ID] = r
	}
	cs := &stubChats{keyed: keyed, listed: rows}
	return cs, lineage.New(cs)
}

func TestAncestors_AChatUnderAChatReadsIt(t *testing.T) {
	_, resolver := tree([]domain.Chat{chat("c1", ""), chat("c2", "c1")})

	got, err := resolver.Ancestors(context.Background(), "c2")
	require.NoError(t, err)
	assert.Equal(t, []string{"c1"}, got)
}

func TestAncestors_StepsThroughTwoFolders(t *testing.T) {
	_, resolver := tree([]domain.Chat{
		chat("c1", ""), chat("c2", "f2"),
		folder("f1", "c1"), folder("f2", "f1"),
	})

	got, err := resolver.Ancestors(context.Background(), "c2")
	require.NoError(t, err)
	assert.Equal(t, []string{"c1"}, got,
		"a thread two folders deep under a chat resolves the same lineage as one sitting directly under it")
}

func TestAncestors_ReturnsTheWholeChainNearestFirst(t *testing.T) {
	_, resolver := tree([]domain.Chat{
		chat("c1", ""), chat("c2", "c1"), chat("c3", "f1"),
		folder("f1", "c2"),
	})

	got, err := resolver.Ancestors(context.Background(), "c3")
	require.NoError(t, err)
	assert.Equal(t, []string{"c2", "c1"}, got)
}

func TestAncestors_AChatAtTheRootReadsNoTables(t *testing.T) {
	chats, resolver := tree([]domain.Chat{chat("c1", "")})

	got, err := resolver.Ancestors(context.Background(), "c1")
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Zero(t, chats.lists, "a chat with no parent needs no list read")
}

func TestAncestors_AChatFiledInARootFolderInheritsNothing(t *testing.T) {
	_, resolver := tree([]domain.Chat{chat("c1", "f1"), folder("f1", "")})

	got, err := resolver.Ancestors(context.Background(), "c1")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestAncestors_ThePlacementComesFromTheLogFoldNotTheStaleList(t *testing.T) {
	placed := chat("c2", "c1")
	unplaced := chat("c2", "")
	cs := &stubChats{
		keyed:  map[string]domain.Chat{"c1": chat("c1", ""), "c2": placed},
		listed: []domain.Chat{chat("c1", ""), unplaced},
	}
	resolver := lineage.New(cs)

	got, err := resolver.Ancestors(context.Background(), "c2")
	require.NoError(t, err)
	assert.Equal(t, []string{"c1"}, got,
		"a placement decision taken on the projection is wrong exactly when it is asked: right after the write")
}

func TestAncestors_SurfacesAChatReadFailure(t *testing.T) {
	cs := &stubChats{getErr: errors.New("boom")}
	resolver := lineage.New(cs)

	_, err := resolver.Ancestors(context.Background(), "c2")
	require.ErrorContains(t, err, "boom")
}

func TestAncestors_StopsAtAnAncestorThatIsNoChatOrFolder(t *testing.T) {
	cs := &stubChats{
		keyed:  map[string]domain.Chat{"c1": chat("c1", "ws-node")},
		listed: []domain.Chat{chat("c1", "ws-node")},
	}
	cs.missing = agentchat.ErrNotFound

	got, err := lineage.New(cs).Ancestors(context.Background(), "c1")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestAncestors_SurfacesAnUnlistedAncestorReadFailure(t *testing.T) {
	cs := &stubChats{
		keyed:  map[string]domain.Chat{"c2": chat("c2", "f1")},
		listed: []domain.Chat{chat("c2", "f1")},
	}
	resolver := lineage.New(cs)

	_, err := resolver.Ancestors(context.Background(), "c2")
	require.ErrorContains(t, err, "ancestor f1")
}

func TestAncestors_SurfacesAChatListFailure(t *testing.T) {
	cs := &stubChats{
		keyed: map[string]domain.Chat{"c2": chat("c2", "c1")},
		list:  errors.New("chats down"),
	}
	resolver := lineage.New(cs)

	_, err := resolver.Ancestors(context.Background(), "c2")
	require.ErrorContains(t, err, "chats down")
}
