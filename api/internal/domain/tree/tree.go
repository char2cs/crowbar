// Package tree is the sidebar's placement model: one Tree per project holds where
// every row hangs and in what order. It keeps three records apart (position rows,
// folder identities, chat placements) because one id can carry a position row and a
// chat placement that disagree; a "" parent is the root of the row's own scope.
package tree

import (
	"cmp"
	"fmt"
	"maps"
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// Kind is which entity a position row stands in for.
type Kind string

const (
	KindFolder    Kind = "folder"
	KindRepo      Kind = "repo"
	KindWorkspace Kind = "workspace"
	KindChat      Kind = "chat"
)

// KindOf parses a stored kind; an unknown one is refused, never guessed.
func KindOf(legacy string) (Kind, error) {
	switch k := Kind(legacy); k {
	case KindFolder, KindRepo, KindWorkspace, KindChat:
		return k, nil
	default:
		return "", fmt.Errorf("tree: kind %q: %w", legacy, problem.ErrInvalidArgument)
	}
}

// Node is one row's position: the node it hangs off ("" for the project root)
// and its dense index among that parent's children. ID is the entity's own id;
// Name is a folder's.
type Node struct {
	ID       string `json:"id"`
	Kind     Kind   `json:"kind"`
	ParentID string `json:"parentId,omitempty"`
	Order    int    `json:"order"`
	Name     string `json:"name,omitempty"`
}

// Folder is a folder's identity. RepoID == "" is a project-home folder, and
// HomeID is then the home workspace it belongs to.
type Folder struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	RepoID string `json:"repoId,omitempty"`
	HomeID string `json:"homeId,omitempty"`
}

// InHome reports whether f is homeID's own home folder.
func (f Folder) InHome(homeID string) bool {
	return f.RepoID == "" && f.HomeID == homeID
}

// Placement is a chat's own record of where it hangs.
type Placement struct {
	ParentID string `json:"parentId,omitempty"`
	Order    int    `json:"order"`
}

// Tree is one project's placement aggregate.
type Tree struct {
	ProjectID string               `json:"projectId"`
	Nodes     map[string]Node      `json:"nodes"`
	Folders   map[string]Folder    `json:"folders"`
	Chats     map[string]Placement `json:"chats"`
}

// Unplaced holds the placements of rows no project can be named for yet: a chat
// placed before it has a workspace, under no parent. It is not a project id.
const Unplaced = "unplaced"

// ErrUnknownNode is a write against a row the tree does not hold.
var ErrUnknownNode = problem.New(problem.NotFound, "tree: no such node")

// OrUnplaced is a position read's answer with a row not written yet read as
// the root's first slot, where every reader shows it; any other failure stays.
func OrUnplaced(n Node, err error) (Node, error) {
	if err != nil && problem.CodeOf(err) == problem.NotFound {
		return Node{}, nil
	}
	return n, err
}

// Empty is projectID's tree before anything was placed in it.
func Empty(projectID string) Tree {
	return Tree{
		ProjectID: projectID,
		Nodes:     map[string]Node{},
		Folders:   map[string]Folder{},
		Chats:     map[string]Placement{},
	}
}

// Clone is a deep copy: a command builds its next state without touching the
// state it was handed.
func (t Tree) Clone() Tree {
	out := Empty(t.ProjectID)
	maps.Copy(out.Nodes, t.Nodes)
	maps.Copy(out.Folders, t.Folders)
	maps.Copy(out.Chats, t.Chats)
	return out
}

// Holds reports whether any of t's records names id.
func (t Tree) Holds(id string) bool {
	_, n := t.Nodes[id]
	_, f := t.Folders[id]
	_, c := t.Chats[id]
	return n || f || c
}

// Merge adds every record of o that t lacks and keeps every record t has.
func (t Tree) Merge(o Tree) Tree {
	out := t.Clone()
	for id, n := range o.Nodes {
		if _, ok := out.Nodes[id]; !ok {
			out.Nodes[id] = n
		}
	}
	for id, f := range o.Folders {
		if _, ok := out.Folders[id]; !ok {
			out.Folders[id] = f
		}
	}
	for id, p := range o.Chats {
		if _, ok := out.Chats[id]; !ok {
			out.Chats[id] = p
		}
	}
	return out
}

// Without is t less every record of id, and the records it removed.
func (t Tree) Without(id string) (rest, taken Tree) {
	rest, taken = t.Clone(), Empty(t.ProjectID)
	if n, ok := rest.Nodes[id]; ok {
		taken.Nodes[id] = n
		delete(rest.Nodes, id)
	}
	if f, ok := rest.Folders[id]; ok {
		taken.Folders[id] = f
		delete(rest.Folders, id)
	}
	if p, ok := rest.Chats[id]; ok {
		taken.Chats[id] = p
		delete(rest.Chats, id)
	}
	return rest, taken
}

// Children is parentID's children by Order, then ID.
func (t Tree) Children(parentID string) []Node {
	return t.byParent()[parentID]
}

// Subtree is id, then its descendants depth-first in order.
func (t Tree) Subtree(id string) []string {
	kids := t.byParent()
	out := []string{}
	var walk func(id string)
	walk = func(id string) {
		out = append(out, id)
		for _, c := range kids[id] {
			walk(c.ID)
		}
	}
	walk(id)
	return out
}

// byParent is every node under its parent, each level by Order, then ID.
func (t Tree) byParent() map[string][]Node {
	out := map[string][]Node{}
	for _, n := range t.Nodes {
		out[n.ParentID] = append(out[n.ParentID], n)
	}
	for _, level := range out {
		slices.SortFunc(level, func(a, b Node) int {
			return cmp.Or(cmp.Compare(a.Order, b.Order), cmp.Compare(a.ID, b.ID))
		})
	}
	return out
}
