package tree

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// The refusals a placement verb answers with; each message is what the user reads.
var (
	ErrCycle          = problem.New(problem.TreeCycle, "A row can't be moved inside itself")
	ErrNotAContainer  = problem.New(problem.TreeNotAContainer, "That row can't hold this one")
	ErrCrossContext   = problem.New(problem.TreeCrossContext, "A row stays in the project home or the repo it belongs to")
	ErrCrossWorkspace = problem.New(problem.TreeCrossWorkspace, "A thread stays in its chat's workspace")
	ErrForkChain      = problem.New(problem.TreeForkChain, "A branch stays under the branch it was forked from")
	ErrSubtreeWorking = problem.New(problem.TreeSubtreeWorking, "A chat in here is still working; stop it first")
	ErrNameRequired   = problem.New(problem.InvalidArgument, "A folder needs a name")
	ErrExists         = problem.New(problem.IDConflict, "tree: the row is already placed")
)

// Facts are what the rules need from other aggregates, for the nodes a verb
// touches and the containers above them. They are passed in, never stored.
type Facts struct {
	ProjectHome string            // the project home workspace id
	RepoHome    map[string]string // repo id → its home checkout's workspace id
	Repo        map[string]string // workspace id → its repo id ("" for the project home)
	BoundTo     map[string]string // chat id → the workspace it runs in
	ForkParent  map[string]string // workspace id → the workspace it was forked from ("" = none)
	Working     map[string]bool   // node id → a chat it stands for is working
}

// check is every structural rule for id at the place t now holds it.
func check(t Tree, id string, f Facts) error {
	n := t.Nodes[id]
	if n.ParentID == "" {
		return nil
	}
	parent, ok := t.Nodes[n.ParentID]
	if !ok {
		return fmt.Errorf("tree: %s under %s: %w", id, n.ParentID, ErrUnknownNode)
	}
	if err := checkContainer(t, n, parent); err != nil {
		return err
	}
	switch n.Kind {
	case KindChat:
		return checkChat(t, n, parent, f)
	case KindWorkspace:
		return checkFork(t, n, f)
	case KindFolder, KindRepo:
	}
	return nil
}

// checkContainer: a chat holds only chats and workspaces, and a repo hangs only
// at project level (the root, checked by the caller, or a home folder).
func checkContainer(t Tree, n, parent Node) error {
	if parent.Kind == KindChat && n.Kind != KindChat && n.Kind != KindWorkspace {
		return fmt.Errorf("tree: %s %s under chat %s: %w", n.Kind, n.ID, parent.ID, ErrNotAContainer)
	}
	if n.Kind == KindRepo && (parent.Kind != KindFolder || contextOf(t, parent.ID) != "") {
		return fmt.Errorf("tree: repo %s under %s %s: %w", n.ID, parent.Kind, parent.ID, ErrNotAContainer)
	}
	return nil
}

// checkChat: a chat directly under a chat or a workspace runs in that row's
// workspace; one filed in a folder keeps its own wherever the folder hangs.
func checkChat(t Tree, n, parent Node, f Facts) error {
	if parent.Kind != KindChat && parent.Kind != KindWorkspace {
		return nil
	}
	bound := f.BoundTo[n.ID]
	if bound == "" || bound == ResolveWorkspace(t, parent.ID, f) {
		return nil
	}
	return fmt.Errorf("tree: chat %s of %s under %s: %w", n.ID, bound, parent.ID, ErrCrossWorkspace)
}

// checkFork: a workspace hangs where its fork parent resolves (the repo's
// home checkout when it was forked from none).
func checkFork(t Tree, n Node, f Facts) error {
	want := f.ForkParent[n.ID]
	if want == "" {
		want = f.RepoHome[f.Repo[n.ID]]
	}
	if want == "" || want == ResolveWorkspace(t, n.ParentID, f) {
		return nil
	}
	return fmt.Errorf("tree: workspace %s away from %s: %w", n.ID, want, ErrForkChain)
}

// contextOf is the repo whose subtree holds id, "" for the project home.
func contextOf(t Tree, id string) string {
	for seen := map[string]bool{}; id != "" && !seen[id]; {
		seen[id] = true
		n, ok := t.Nodes[id]
		if !ok {
			return ""
		}
		if n.Kind == KindRepo {
			return n.ID
		}
		id = n.ParentID
	}
	return ""
}

// ownContext is the context a new chat or workspace belongs in, when the
// facts name it.
func ownContext(n Node, f Facts) (string, bool) {
	ws := n.ID
	if n.Kind == KindChat {
		ws = f.BoundTo[n.ID]
	}
	if n.Kind != KindChat && n.Kind != KindWorkspace || ws == "" {
		return "", false
	}
	repo, ok := f.Repo[ws]
	return repo, ok
}

func checkWorking(t Tree, id string, f Facts) error {
	for _, d := range t.Subtree(id) {
		if f.Working[d] {
			return fmt.Errorf("tree: move %s: %s is working: %w", id, d, ErrSubtreeWorking)
		}
	}
	return nil
}
