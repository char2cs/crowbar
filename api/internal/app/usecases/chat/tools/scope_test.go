package tools_test

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type stubRunners struct {
	r   agents.Runner
	err error
}

func (s stubRunners) Get(context.Context, string) (agents.Runner, error) { return s.r, s.err }

type stubChats struct {
	c    domain.Chat
	list []domain.Chat
}

func (s stubChats) Get(context.Context, string) (domain.Chat, error) { return s.c, nil }
func (s stubChats) ListChats(context.Context) ([]domain.Chat, error) {
	return s.list, nil
}

type stubWorkspaces struct{ all []domain.Workspace }

func (s stubWorkspaces) Get(_ context.Context, id string) (domain.Workspace, error) {
	for _, w := range s.all {
		if w.ID == id {
			return w, nil
		}
	}
	return domain.Workspace{}, apperrNotFound()
}
func (s stubWorkspaces) List(context.Context) ([]domain.Workspace, error) { return s.all, nil }

// The tree used by every case below.
//
//	proj home (home)
//	  repo-default (git, IsDefault)
//	    ws-a
//	      ws-a1
//	    ws-b
//	other-repo-ws (a different repo, same project)
func tree() []domain.Workspace {
	return []domain.Workspace{
		{ID: "home", ProjectID: "P", Kind: domain.WorkspaceKindHome},
		{ID: "repo-default", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, IsDefault: true},
		{ID: "ws-a", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, ParentID: "repo-default"},
		{ID: "ws-a1", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, ParentID: "ws-a"},
		{ID: "ws-b", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, ParentID: "repo-default"},
		{ID: "other-repo-ws", ProjectID: "P", RepoID: "R2", Kind: domain.WorkspaceKindGit},
	}
}

func resolverOn(t *testing.T, callerWs string) (*tools.Resolver, *tools.TokenMinter) {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	return tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: callerWs}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: callerWs}},
		stubWorkspaces{all: tree()},
	), m
}

func visibleIDs(c tools.Caller) []string {
	all, _ := c.Visible()
	out := make([]string, 0, len(all))
	for _, w := range all {
		out = append(out, w.ID)
	}
	return out
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

// A displaced runner has no current chat: it must not resolve to the chat it
// used to be on.
func TestResolve_RejectsDisplacedRunner(t *testing.T) {
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	r := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "", WorkspaceID: "ws-a"}},
		stubChats{}, stubWorkspaces{all: tree()})

	_, err = r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.ErrorIs(t, err, tools.ErrUnauthorized)
}

// TestResolve_NeverSeesDeletedWorkspaces covers all three visibility branches,
// because the bug was in none of them individually and in all of them at once:
// WorkspaceLister.List hands back the residual rows of workspaces the user has
// already deleted, five other consumers of that list filter them, and the tool
// surface did not. The symptom is list_workspaces advertising workspaces the
// user's own sidebar no longer shows — and every other tool then willing to act
// on them, because CanSee is computed from the same set.
func TestResolve_NeverSeesDeletedWorkspaces(t *testing.T) {
	deletedTree := func() []domain.Workspace {
		all := tree()
		for i := range all {
			// One in each branch's reach: a descendant of ws-a, a sibling under
			// the repo default, and a workspace in another repo of the project.
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
				stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: tc.caller}},
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

// The caller's OWN workspace survives the filter even mid-delete: Caller.Visible
// always containing it is an invariant every tool reads through CanSee, so a
// runner whose workspace is being reaped must still resolve to itself rather
// than to an empty set that makes its own chat unreachable.
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
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: all})

	c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)
	require.True(t, c.CanSee("ws-a"))
}

// TestResolve_AParentCycleCannotHangTheWalk is about TERMINATION, not policy —
// read the assertion with that in mind before changing either.
//
// With x.ParentID == y and y.ParentID == x, x's downward walk reaches y, which
// is also x's own parent. That is NOT sanctioned upward visibility and must
// never be read as such. It is unreachable state: Create only ever sets a
// ParentID on a brand-new id, which nothing can be an ancestor of, and Reparent
// refuses both a self-parent and moving a workspace that HAS children — which is
// what closing any cycle would require. A tree like the one below therefore
// comes from direct DB corruption and nothing else.
//
// What is pinned here is only that the seen set makes the walk RETURN on such a
// tree instead of spinning forever. TestResolve_NeverSeesUpwards above is where
// the actual upward-visibility policy is asserted.
func TestResolve_AParentCycleCannotHangTheWalk(t *testing.T) {
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	cyc := []domain.Workspace{
		{ID: "x", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, ParentID: "y"},
		{ID: "y", ProjectID: "P", RepoID: "R", Kind: domain.WorkspaceKindGit, ParentID: "x"},
	}
	r := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "x"}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "x"}},
		stubWorkspaces{all: cyc})

	c, err := r.Resolve(context.Background(), "RUN", m.Mint("RUN"))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"x", "y"}, visibleIDs(c))
}

// TestResolve_NilMinterFailsClosed proves a resolver built without a minter
// refuses every caller instead of panicking inside Verify. A misconfiguration
// must deny, not crash the daemon on the first tool call an agent makes — and it
// must certainly not be mistaken for "no authentication required".
func TestResolve_NilMinterFailsClosed(t *testing.T) {
	r := tools.NewResolver(nil,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})

	_, err := r.Resolve(context.Background(), "RUN", "any-token")
	require.ErrorIs(t, err, tools.ErrUnauthorized)
}

func apperrNotFound() error { return errNotFoundForTest }

var errNotFoundForTest = errors.New("not found")
