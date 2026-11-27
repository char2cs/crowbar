package tree

import (
	"cmp"
	"fmt"
	"maps"
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type Kind string

const (
	KindFolder    Kind = "folder"
	KindRepo      Kind = "repo"
	KindWorkspace Kind = "workspace"
	KindChat      Kind = "chat"
)

func KindOf(legacy string) (Kind, error) {
	switch k := Kind(legacy); k {
	case KindFolder, KindRepo, KindWorkspace, KindChat:
		return k, nil
	default:
		return "", fmt.Errorf("tree: kind %q: %w", legacy, problem.ErrInvalidArgument)
	}
}

type Node struct {
	ID       string `json:"id"`
	Kind     Kind   `json:"kind"`
	ParentID string `json:"parentId,omitempty"`
	Order    int    `json:"order"`
	Name     string `json:"name,omitempty"`
}

type Folder struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	RepoID string `json:"repoId,omitempty"`
	HomeID string `json:"homeId,omitempty"`
}

func (f Folder) InHome(homeID string) bool {
	return f.RepoID == "" && f.HomeID == homeID
}

type Placement struct {
	ParentID string `json:"parentId,omitempty"`
	Order    int    `json:"order"`
}

type Tree struct {
	ProjectID string               `json:"projectId"`
	Nodes     map[string]Node      `json:"nodes"`
	Folders   map[string]Folder    `json:"folders"`
	Chats     map[string]Placement `json:"chats"`
}

const Unplaced = "unplaced"

var ErrUnknownNode = problem.New(problem.NotFound, "tree: no such node")

func OrUnplaced(n Node, err error) (Node, error) {
	if err != nil && problem.CodeOf(err) == problem.NotFound {
		return Node{}, nil
	}
	return n, err
}

func Empty(projectID string) Tree {
	return Tree{
		ProjectID: projectID,
		Nodes:     map[string]Node{},
		Folders:   map[string]Folder{},
		Chats:     map[string]Placement{},
	}
}

func (t Tree) Clone() Tree {
	out := Empty(t.ProjectID)
	maps.Copy(out.Nodes, t.Nodes)
	maps.Copy(out.Folders, t.Folders)
	maps.Copy(out.Chats, t.Chats)
	return out
}

func (t Tree) Holds(id string) bool {
	_, n := t.Nodes[id]
	_, f := t.Folders[id]
	_, c := t.Chats[id]
	return n || f || c
}

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

func (t Tree) Children(parentID string) []Node {
	return t.byParent()[parentID]
}

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
