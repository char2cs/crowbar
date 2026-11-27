package tools_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestResolve_DoesNotReadTheWorkspaceTree(t *testing.T) {
	ws := &treeLister{all: tree()}
	r, m := lazyResolverOn(t, "ws-a", ws)

	_, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)

	require.Zero(t, ws.lists, "resolving a caller must not list workspaces")
}

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

func TestCanSee_ZeroCallerDenies(t *testing.T) {
	var c tools.Caller

	require.False(t, c.CanSee("ws-a"))
	_, err := c.Visible()
	require.ErrorIs(t, err, tools.ErrUnauthorized)
}

func TestResolve_GitWorkspaceSeesItselfAndDescendants(t *testing.T) {
	r, m := resolverOn(t, "ws-a")
	c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"ws-a", "ws-a1"}, visibleIDs(c))
}

func TestResolve_NeverSeesUpwards(t *testing.T) {
	r, m := resolverOn(t, "ws-a1")
	c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"ws-a1"}, visibleIDs(c))
	require.False(t, c.CanSee("ws-a"))
	require.False(t, c.CanSee("repo-default"))
}

func TestResolve_RepoDefaultSeesWholeRepoButNotOtherRepos(t *testing.T) {
	r, m := resolverOn(t, "repo-default")
	c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"repo-default", "ws-a", "ws-a1", "ws-b"}, visibleIDs(c))
	require.False(t, c.CanSee("other-repo-ws"))
	require.False(t, c.CanSee("home"))
}

func TestResolve_HomeWorkspaceSeesWholeProject(t *testing.T) {
	r, m := resolverOn(t, "home")
	c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)
	require.ElementsMatch(t,
		[]string{"home", "repo-default", "ws-a", "ws-a1", "ws-b", "other-repo-ws"},
		visibleIDs(c))
}

func TestResolve_RejectsBadToken(t *testing.T) {
	r, m := resolverOn(t, "ws-a")
	_, err := r.Resolve(context.Background(), "RUN", m.Mint("SOMEONE-ELSE"))
	require.ErrorIs(t, err, tools.ErrUnauthorized)
}

func TestResolve_RejectsAnExitedRunner(t *testing.T) {
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	r := tools.NewResolver(m, stubRunners{}, stubWorkspaces{all: tree()})

	_, err = r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.ErrorIs(t, err, tools.ErrUnauthorized)
}

func TestResolve_NoWorkspaceStoreFailsClosed(t *testing.T) {
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	r := tools.NewResolver(m, stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}}, nil)

	_, err = r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.ErrorIs(t, err, tools.ErrUnauthorized)
}

func relayResolver(t *testing.T, runners stubRunners) (*tools.Resolver, *tools.TokenMinter) {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	return tools.NewResolver(m, runners, stubWorkspaces{all: tree()}), m
}

func TestResolveRelay_ResolvesToTheChatsLiveRunner(t *testing.T) {
	live := agents.Runner{ID: "RUN-2", CurrentChatID: "CHAT", ProviderID: "p", WorkspaceID: "ws-a"}
	r, m := relayResolver(t, stubRunners{r: live, relay: "RELAY"})

	c, err := r.ResolveRelay(context.Background(), "RELAY", m.MintRelay("RELAY"))
	require.NoError(t, err)
	require.Equal(t, "RUN-2", c.RunnerID)
	require.Equal(t, "CHAT", c.ChatID)
	require.ElementsMatch(t, []string{"ws-a", "ws-a1"}, visibleIDs(c))
}

func TestResolveRelay_RefusesWhatNoLiveRunnerOrBindingBacks(t *testing.T) {
	live := agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}
	for name, tc := range map[string]struct {
		runners stubRunners
		relay   string
		token   func(*tools.TokenMinter) string
	}{
		"no live runner on the chat":       {stubRunners{relay: "RELAY"}, "RELAY", func(m *tools.TokenMinter) string { return m.MintRelay("RELAY") }},
		"another binding's relay id":       {stubRunners{r: live, relay: "RELAY"}, "OTHER", func(m *tools.TokenMinter) string { return m.MintRelay("OTHER") }},
		"a token minted for another id":    {stubRunners{r: live, relay: "RELAY"}, "RELAY", func(m *tools.TokenMinter) string { return m.MintRelay("OTHER") }},
		"a runner's token in its place":    {stubRunners{r: live, relay: "RELAY"}, "RELAY", func(m *tools.TokenMinter) string { return m.Mint("RELAY") }},
		"no token":                         {stubRunners{r: live, relay: "RELAY"}, "RELAY", func(*tools.TokenMinter) string { return "" }},
		"an unreadable binding":            {stubRunners{r: live, relay: "RELAY", relayErr: errors.New("ledger down")}, "RELAY", func(m *tools.TokenMinter) string { return m.MintRelay("RELAY") }},
		"an empty relay id with its token": {stubRunners{r: live, relay: ""}, "", func(m *tools.TokenMinter) string { return m.MintRelay("") }},
	} {
		t.Run(name, func(t *testing.T) {
			r, m := relayResolver(t, tc.runners)
			_, err := r.ResolveRelay(context.Background(), tc.relay, tc.token(m))
			require.ErrorIs(t, err, tools.ErrUnauthorized)
		})
	}
}

func TestResolve_ARelayTokenIsNotARunnerToken(t *testing.T) {
	live := agents.Runner{ID: "ID", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}
	r, m := relayResolver(t, stubRunners{r: live, relay: "ID"})

	_, err := r.Resolve(context.Background(), "ID", m.MintRelay("ID"))
	require.ErrorIs(t, err, tools.ErrUnauthorized)
}

func TestResolve_NeverSeesDeletedWorkspaces(t *testing.T) {
	deletedTree := func() []domain.Workspace {
		all := tree()
		for i := range all {
			switch all[i].ID {
			case "ws-a1", "ws-b", "other-repo-ws":
				all[i].Status = domain.WorkspaceStatusDeleted
			}
		}
		return all
	}

	for _, tc := range []struct {
		name   string
		caller string
		want   []string
	}{
		{"git workspace", "ws-a", []string{"ws-a"}},
		{"repo default", "repo-default", []string{"repo-default", "ws-a"}},
		{"project home", "home", []string{"home", "repo-default", "ws-a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := tools.NewTokenMinter()
			require.NoError(t, err)
			r := tools.NewResolver(m,
				stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: tc.caller}},
				stubWorkspaces{all: deletedTree()})

			c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
			require.NoError(t, err)
			require.ElementsMatch(t, tc.want, visibleIDs(c))
			require.False(t, c.CanSee("ws-a1"))
			require.False(t, c.CanSee("ws-b"))
			require.False(t, c.CanSee("other-repo-ws"))
		})
	}
}

func TestResolve_KeepsTheCallersOwnWorkspaceEvenWhenDeleted(t *testing.T) {
	all := tree()
	for i := range all {
		if all[i].ID == "ws-a" {
			all[i].Status = domain.WorkspaceStatusDeleted
		}
	}
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	r := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: all})

	c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)
	require.True(t, c.CanSee("ws-a"))
}

func TestResolve_AParentCycleCannotHangTheWalk(t *testing.T) {
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	cyc := []domain.Workspace{
		{ID: "x", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, ParentID: "y"},
		{ID: "y", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, ParentID: "x"},
	}
	r := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "x"}},
		stubWorkspaces{all: cyc})

	c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"x", "y"}, visibleIDs(c))
}

func TestResolve_NilMinterFailsClosed(t *testing.T) {
	r := tools.NewResolver(nil,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})

	_, err := r.Resolve(context.Background(), "RUN", "any-token")
	require.ErrorIs(t, err, tools.ErrUnauthorized)
}
