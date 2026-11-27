package tree

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

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

type Facts struct {
	ProjectHome string
	RepoHome    map[string]string
	Repo        map[string]string
	BoundTo     map[string]string
	ForkParent  map[string]string
	Working     map[string]bool
}

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

func checkContainer(t Tree, n, parent Node) error {
	if parent.Kind == KindChat && n.Kind != KindChat && n.Kind != KindWorkspace {
		return fmt.Errorf("tree: %s %s under chat %s: %w", n.Kind, n.ID, parent.ID, ErrNotAContainer)
	}
	if n.Kind == KindRepo && (parent.Kind != KindFolder || contextOf(t, parent.ID) != "") {
		return fmt.Errorf("tree: repo %s under %s %s: %w", n.ID, parent.Kind, parent.ID, ErrNotAContainer)
	}
	return nil
}

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
