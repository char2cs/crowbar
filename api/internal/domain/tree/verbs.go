package tree

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// Every verb validates against the whole tree first, then returns a new tree
// with each level it touched renumbered 0..n-1; the receiver is never changed.

// Insert places a new node n under parentID at index.
func (t Tree) Insert(n Node, parentID string, index int, f Facts) (Tree, error) {
	if n.ID == "" {
		return Tree{}, fmt.Errorf("tree: insert: missing id: %w", problem.ErrInvalidArgument)
	}
	if _, err := KindOf(string(n.Kind)); err != nil {
		return Tree{}, fmt.Errorf("tree: insert %s: %w", n.ID, err)
	}
	if _, ok := t.Nodes[n.ID]; ok {
		return Tree{}, fmt.Errorf("tree: insert %s: %w", n.ID, ErrExists)
	}
	if n.Kind == KindFolder && strings.TrimSpace(n.Name) == "" {
		return Tree{}, fmt.Errorf("tree: insert %s: %w", n.ID, ErrNameRequired)
	}
	if err := t.knownParent(n.ID, parentID); err != nil {
		return Tree{}, err
	}
	out := t.Clone()
	n.ParentID = parentID
	out.Nodes[n.ID] = n
	out.place(n.ID, parentID, index)
	if want, ok := ownContext(n, f); ok && want != contextOf(out, parentID) {
		return Tree{}, fmt.Errorf("tree: insert %s %s: %w", n.Kind, n.ID, ErrCrossContext)
	}
	if err := check(out, n.ID, f); err != nil {
		return Tree{}, err
	}
	return out, nil
}

// Move re-places id, with everything below it, under parentID at index.
func (t Tree) Move(id, parentID string, index int, f Facts) (Tree, error) {
	n, ok := t.Nodes[id]
	if !ok {
		return Tree{}, fmt.Errorf("tree: move %s: %w", id, ErrUnknownNode)
	}
	if err := t.knownParent(id, parentID); err != nil {
		return Tree{}, err
	}
	sub := t.Subtree(id)
	for _, d := range sub {
		if d == parentID {
			return Tree{}, fmt.Errorf("tree: move %s under %s: %w", id, parentID, ErrCycle)
		}
	}
	if err := checkWorking(t, id, f); err != nil {
		return Tree{}, err
	}
	out := t.Clone()
	delete(out.Nodes, id)
	out.renumber(n.ParentID)
	n.ParentID = parentID
	out.Nodes[id] = n
	out.place(id, parentID, index)
	if n.Kind != KindRepo && contextOf(t, id) != contextOf(out, id) {
		return Tree{}, fmt.Errorf("tree: move %s %s: %w", n.Kind, id, ErrCrossContext)
	}
	for _, d := range sub {
		if err := check(out, d, f); err != nil {
			return Tree{}, err
		}
	}
	return out, nil
}

// Rename names folder id.
func (t Tree) Rename(id, name string) (Tree, error) {
	n, ok := t.Nodes[id]
	if !ok {
		return Tree{}, fmt.Errorf("tree: rename %s: %w", id, ErrUnknownNode)
	}
	if n.Kind != KindFolder {
		return Tree{}, fmt.Errorf("tree: rename %s %s: %w", n.Kind, id, ErrNotAContainer)
	}
	if name = strings.TrimSpace(name); name == "" {
		return Tree{}, fmt.Errorf("tree: rename %s: %w", id, ErrNameRequired)
	}
	out := t.Clone()
	n.Name = name
	out.Nodes[id] = n
	return out, nil
}

// Remove takes id out. With promote its children take its place, in their
// order; without, its whole subtree goes. It returns the ids removed.
func (t Tree) Remove(id string, promote bool) (Tree, []string, error) {
	n, ok := t.Nodes[id]
	if !ok {
		return Tree{}, nil, fmt.Errorf("tree: remove %s: %w", id, ErrUnknownNode)
	}
	out := t.Clone()
	if !promote {
		gone := t.Subtree(id)
		for _, d := range gone {
			delete(out.Nodes, d)
		}
		out.renumber(n.ParentID)
		return out, gone, nil
	}
	level := out.Children(n.ParentID)
	kids := out.Children(id)
	delete(out.Nodes, id)
	order := 0
	for _, s := range level {
		if s.ID != id {
			out.setPlace(s.ID, n.ParentID, order)
			order++
			continue
		}
		for _, k := range kids {
			out.setPlace(k.ID, n.ParentID, order)
			order++
		}
	}
	return out, []string{id}, nil
}

// Promote turns chat node chatID into workspace node workspaceID at the same
// place; the chat's children hang under the workspace node.
func (t Tree) Promote(chatID, workspaceID string) (Tree, error) {
	n, ok := t.Nodes[chatID]
	if !ok {
		return Tree{}, fmt.Errorf("tree: promote %s: %w", chatID, ErrUnknownNode)
	}
	if n.Kind != KindChat {
		return Tree{}, fmt.Errorf("tree: promote %s %s: %w", n.Kind, chatID, ErrNotAContainer)
	}
	if _, ok := t.Nodes[workspaceID]; ok {
		return Tree{}, fmt.Errorf("tree: promote %s to %s: %w", chatID, workspaceID, ErrExists)
	}
	out := t.Clone()
	for _, k := range out.Children(chatID) {
		out.setPlace(k.ID, workspaceID, k.Order)
	}
	delete(out.Nodes, chatID)
	out.Nodes[workspaceID] = Node{ID: workspaceID, Kind: KindWorkspace, ParentID: n.ParentID, Order: n.Order}
	return out, nil
}

func (t Tree) knownParent(id, parentID string) error {
	if parentID == "" {
		return nil
	}
	if parentID == id {
		return fmt.Errorf("tree: %s under itself: %w", id, ErrCycle)
	}
	if _, ok := t.Nodes[parentID]; !ok {
		return fmt.Errorf("tree: %s under %s: %w", id, parentID, ErrUnknownNode)
	}
	return nil
}

// place puts id at index among parentID's other children (clamped) and
// renumbers that level. t must be a tree this verb owns.
func (t Tree) place(id, parentID string, index int) {
	var level []string
	for _, s := range t.Children(parentID) {
		if s.ID != id {
			level = append(level, s.ID)
		}
	}
	index = max(0, min(index, len(level)))
	level = append(level[:index], append([]string{id}, level[index:]...)...)
	for i, s := range level {
		t.setPlace(s, parentID, i)
	}
}

func (t Tree) renumber(parentID string) {
	for i, s := range t.Children(parentID) {
		t.setPlace(s.ID, parentID, i)
	}
}

func (t Tree) setPlace(id, parentID string, order int) {
	n := t.Nodes[id]
	n.ParentID, n.Order = parentID, order
	t.Nodes[id] = n
}
