package chatlineage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chatlineage"
	"github.com/char2cs/crowbar/api/internal/domain"
)

const workspaceID = "ws-1"

// stubChats is the chat read port with its two reads answered independently, so
// a test can make them DISAGREE — which is the state the daemon is really in for
// a moment after every placement write: the log fold (LoadChat) already has the
// new parent while the projection the list serves has not folded it yet.
type stubChats struct {
	keyed  map[string]domain.AgentChat
	listed []domain.AgentChat
	getErr error
	list   error
	lists  int
}

func (s *stubChats) LoadChat(
	_ context.Context,
	id string,
) (domain.AgentChat, error) {
	if s.getErr != nil {
		return domain.AgentChat{}, s.getErr
	}
	chat, ok := s.keyed[id]
	if !ok {
		return domain.AgentChat{}, errors.New("no such chat")
	}
	return chat, nil
}

func (s *stubChats) ListByWorkspace(
	_ context.Context,
	_ string,
) ([]domain.AgentChat, error) {
	s.lists++
	if s.list != nil {
		return nil, s.list
	}
	return s.listed, nil
}

type stubFolders struct {
	rows  []domain.AgentChatFolder
	err   error
	finds int
}

func (s *stubFolders) FindWhere(
	_ context.Context,
	_ domain.AgentChatFolder,
) ([]domain.AgentChatFolder, error) {
	s.finds++
	if s.err != nil {
		return nil, s.err
	}
	return s.rows, nil
}

func chat(
	id string,
	parentID string,
) domain.AgentChat {
	return domain.AgentChat{ID: id, WorkspaceID: workspaceID, ParentID: parentID}
}

func folder(
	id string,
	parentID string,
) domain.AgentChatFolder {
	return domain.AgentChatFolder{ID: id, WorkspaceID: workspaceID, ParentID: parentID}
}

// tree builds a resolver over one workspace's rows, keying the chats for the
// authoritative read and listing the same rows for the walk.
func tree(
	chats []domain.AgentChat,
	folders []domain.AgentChatFolder,
) (*stubFolders, *stubChats, *chatlineage.Resolver) {
	keyed := make(map[string]domain.AgentChat, len(chats))
	for _, c := range chats {
		keyed[c.ID] = c
	}
	cs := &stubChats{keyed: keyed, listed: chats}
	fs := &stubFolders{rows: folders}
	return fs, cs, chatlineage.New(fs, cs)
}

func TestAncestors_AChatUnderAChatReadsIt(t *testing.T) {
	_, _, resolver := tree([]domain.AgentChat{chat("c1", ""), chat("c2", "c1")}, nil)

	got, err := resolver.Ancestors(context.Background(), "c2")
	require.NoError(t, err)
	assert.Equal(t, []string{"c1"}, got)
}

// The folder-transparency rule, asserted against the store rather than against
// the walk: the folder rows have to be READ for the chain to be followed through
// them at all, and a resolver that skipped that read would report that a filed
// thread inherits nothing.
func TestAncestors_StepsThroughTwoFolders(t *testing.T) {
	_, _, resolver := tree(
		[]domain.AgentChat{chat("c1", ""), chat("c2", "f2")},
		[]domain.AgentChatFolder{folder("f1", "c1"), folder("f2", "f1")},
	)

	got, err := resolver.Ancestors(context.Background(), "c2")
	require.NoError(t, err)
	assert.Equal(t, []string{"c1"}, got,
		"a thread two folders deep under a chat resolves the same lineage as one sitting directly under it")
}

func TestAncestors_ReturnsTheWholeChainNearestFirst(t *testing.T) {
	_, _, resolver := tree(
		[]domain.AgentChat{chat("c1", ""), chat("c2", "c1"), chat("c3", "f1")},
		[]domain.AgentChatFolder{folder("f1", "c2")},
	)

	got, err := resolver.Ancestors(context.Background(), "c3")
	require.NoError(t, err)
	assert.Equal(t, []string{"c2", "c1"}, got)
}

// A chat at the panel root is nearly every chat, and it must cost ONE read. The
// table scans are what this feature would otherwise charge every spawn in the
// daemon for a relationship almost none of them have.
func TestAncestors_AChatAtTheRootReadsNoTables(t *testing.T) {
	folders, chats, resolver := tree([]domain.AgentChat{chat("c1", "")}, nil)

	got, err := resolver.Ancestors(context.Background(), "c1")
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Zero(t, folders.finds, "a chat with no parent needs no folder table")
	assert.Zero(t, chats.lists, "and no chat list either")
}

// A chat filed in a root folder has a parent and so pays for the reads, but
// still inherits nothing — the walk finds a folder above it and then the root.
func TestAncestors_AChatFiledInARootFolderInheritsNothing(t *testing.T) {
	_, _, resolver := tree(
		[]domain.AgentChat{chat("c1", "f1")},
		[]domain.AgentChatFolder{folder("f1", "")},
	)

	got, err := resolver.Ancestors(context.Background(), "c1")
	require.NoError(t, err)
	assert.Empty(t, got)
}

// The list is a projection and the log fold is not, and the subject is the row
// that must never be taken from the projection: it is the one the operation
// asking the question just WROTE. A chat placed a moment ago is still LISTED at
// its old placement, so a create — which asks microseconds after placing — reads
// "no parent" and inherits nothing.
//
// The early exit is where that bites, because it is the branch that decides on
// this field ALONE and returns before the walk that would have compensated ever
// runs. So the list here is maximally wrong about the subject and the answer must
// still be right.
func TestAncestors_ThePlacementComesFromTheLogFoldNotTheStaleList(t *testing.T) {
	placed := chat("c2", "c1")
	unplaced := chat("c2", "")
	cs := &stubChats{
		keyed:  map[string]domain.AgentChat{"c1": chat("c1", ""), "c2": placed},
		listed: []domain.AgentChat{chat("c1", ""), unplaced},
	}
	resolver := chatlineage.New(&stubFolders{}, cs)

	got, err := resolver.Ancestors(context.Background(), "c2")
	require.NoError(t, err)
	assert.Equal(t, []string{"c1"}, got,
		"a placement decision taken on the projection is wrong exactly when it is asked: right after the write")
}

func TestAncestors_SurfacesAChatReadFailure(t *testing.T) {
	cs := &stubChats{getErr: errors.New("boom")}
	resolver := chatlineage.New(&stubFolders{}, cs)

	_, err := resolver.Ancestors(context.Background(), "c2")
	require.ErrorContains(t, err, "boom")
}

func TestAncestors_SurfacesAFolderReadFailure(t *testing.T) {
	folders, _, _ := tree([]domain.AgentChat{chat("c1", ""), chat("c2", "c1")}, nil)
	folders.err = errors.New("folders down")
	resolver := chatlineage.New(folders, &stubChats{
		keyed: map[string]domain.AgentChat{"c2": chat("c2", "c1")},
	})

	_, err := resolver.Ancestors(context.Background(), "c2")
	require.ErrorContains(t, err, "folders down")
}

func TestAncestors_SurfacesAChatListFailure(t *testing.T) {
	cs := &stubChats{
		keyed: map[string]domain.AgentChat{"c2": chat("c2", "c1")},
		list:  errors.New("chats down"),
	}
	resolver := chatlineage.New(&stubFolders{}, cs)

	_, err := resolver.Ancestors(context.Background(), "c2")
	require.ErrorContains(t, err, "chats down")
}
