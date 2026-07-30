package agenttools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// treeLister answers like stubWorkspaces but records every List, and can be
// made to fail it. The count is the assertion for the laziness itself — the
// tree read is not observable in a tool's OUTPUT, only in whether it happened —
// and the failure is what proves the direction CanSee fails in.
type treeLister struct {
	all     []domain.Workspace
	lists   int
	listErr error
}

func (s *treeLister) Get(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	for _, w := range s.all {
		if w.ID == id {
			return w, nil
		}
	}
	return domain.Workspace{}, errNotFoundForTest
}

func (s *treeLister) List(
	context.Context,
) ([]domain.Workspace, error) {
	s.lists++
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.all, nil
}

func lazyResolverOn(
	t *testing.T,
	callerWs string,
	workspaces *treeLister,
) (*agenttools.Resolver, *agenttools.TokenMinter) {
	t.Helper()
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	return agenttools.NewResolver(m,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: callerWs}},
		stubChats{c: domain.AgentChat{ID: "CHAT", WorkspaceID: callerWs}},
		workspaces,
	), m
}

// Resolve runs on every MCP call and used to read the whole workspace table
// there — a full scan plus a JSON unmarshal per row — for tools that never look
// past their own workspace. Authenticating must not touch the tree.
func TestResolve_DoesNotReadTheWorkspaceTree(t *testing.T) {
	ws := &treeLister{all: tree()}
	r, m := lazyResolverOn(t, "ws-a", ws)

	_, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)

	require.Zero(t, ws.lists, "resolving a caller must not list workspaces")
}

// The tree is read at most once per resolved caller however many times the
// visible set is consulted, and a Caller passed BY VALUE into a tool handler
// shares that one load with the copy it was made from.
func TestCaller_LoadsTheVisibleSetOnceAcrossCopies(t *testing.T) {
	ws := &treeLister{all: tree()}
	r, m := lazyResolverOn(t, "ws-a", ws)

	c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)

	copied := c
	require.True(t, c.CanSee("ws-a1"))
	require.True(t, copied.CanSee("ws-a1"))
	_, err = copied.Visible()
	require.NoError(t, err)

	require.Equal(t, 1, ws.lists)
}

// A workspace tree that cannot be read must DENY, never allow. This is the
// failure direction the whole authority model rests on: an unreadable tree is
// exactly the moment the daemon cannot tell one workspace from another, and
// reading that as "no restriction" would hand an agent every workspace on the
// machine.
func TestCanSee_DeniesWhenTheVisibleSetCannotBeLoaded(t *testing.T) {
	ws := &treeLister{all: tree(), listErr: errors.New("read model unavailable")}
	r, m := lazyResolverOn(t, "ws-a", ws)

	c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)

	require.False(t, c.CanSee("ws-a1"), "a descendant must not be reachable through a failed load")
	require.False(t, c.CanSee("ws-a"), "not even the caller's own workspace may be allowed on a failed load")
	require.False(t, c.CanSee("other-repo-ws"))

	_, err = c.Visible()
	require.Error(t, err, "the failure must be reportable, not rendered as an empty tree")
}

// A Caller that never came out of Resolve — a zero value, which is what every
// error path here returns — carries no visibility set at all, and must deny
// rather than treat "no set" as "no restriction".
func TestCanSee_ZeroCallerDenies(t *testing.T) {
	var c agenttools.Caller

	require.False(t, c.CanSee("ws-a"))
	_, err := c.Visible()
	require.ErrorIs(t, err, agenttools.ErrUnauthorized)
}

// set_chat_title acts on the caller's own runner and never looks at another
// workspace, so it must not pay for the tree. get_chat_log resolves a chat id
// belonging to SOME workspace and has to check it, so it must.
func TestToolSet_OnlyTheToolsThatNeedTheTreeReadIt(t *testing.T) {
	for _, tc := range []struct {
		name string
		tool string
		args string
		want int
	}{
		{"set_chat_title needs no tree", "set_chat_title", `{"title":"Some Task"}`, 0},
		{"get_chat_log checks the chat's workspace", "get_chat_log", `{"chatId":"other"}`, 1},
		{"list_workspaces renders the tree", "list_workspaces", `{}`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := &treeLister{all: tree()}
			m, err := agenttools.NewTokenMinter()
			require.NoError(t, err)
			chats := stubChats{c: domain.AgentChat{ID: "other", WorkspaceID: "ws-a1"}}
			res := agenttools.NewResolver(m,
				stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
				chats, ws)
			ts := agenttools.NewToolSet(agenttools.Deps{
				Resolver:  res,
				Chats:     &spyRenamer{},
				ChatReads: chats,
				ChatLogs:  &stubChatLogs{turns: chatTurns(1)},
			}, "RUN", m.Mint("RUN"))

			_, err = ts.Call(context.Background(), tc.tool, json.RawMessage(tc.args))
			require.NoError(t, err)
			require.Equal(t, tc.want, ws.lists)
		})
	}
}
