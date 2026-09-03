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
