package tree

import (
	"fmt"
	"maps"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// edit is every Tree command: a pure change to one project's whole tree, so a
// write decides against everything that project holds. Validate and EmitEvent
// run the same change; it touches no IO, which is what makes running it twice safe.
type edit struct {
	projectID string
	op        string
	apply     func(t tree.Tree) (tree.Tree, error)
}

func (c edit) AggregateID() string  { return c.projectID }
func (c edit) EventName() string    { return "tree." + c.op + "." + c.projectID }
func (c edit) ShouldSnapshot() bool { return true }

func (c edit) Validate(current *tree.Tree) error {
	if c.projectID == "" {
		return fmt.Errorf("tree: %s: missing project: %w", c.op, asynxModels.ErrValidation)
	}
	_, err := c.apply(c.base(current))
	return err
}

// EmitEvent runs only after Validate passed; a change that fails here keeps the tree.
func (c edit) EmitEvent(current *tree.Tree) tree.Tree {
	next, err := c.apply(c.base(current))
	if err != nil {
		return c.base(current)
	}
	return next
}

// base is the tree the change applies to: a copy, so the fold's state is never
// mutated, with every map present even on a tree an older event left without one.
func (c edit) base(current *tree.Tree) tree.Tree {
	if current == nil {
		return tree.Empty(c.projectID)
	}
	return tree.Empty(c.projectID).Merge(*current)
}

func seed(t tree.Tree) edit {
	return edit{projectID: t.ProjectID, op: "seeded", apply: func(cur tree.Tree) (tree.Tree, error) {
		return cur.Merge(t), nil
	}}
}

func putNode(projectID string, n tree.Node) edit {
	return edit{projectID: projectID, op: "put", apply: func(cur tree.Tree) (tree.Tree, error) {
		if err := validNode(n); err != nil {
			return tree.Tree{}, err
		}
		cur.Nodes[n.ID] = n
		return cur, nil
	}}
}

// createNode mints n; a row the tree already holds is refused.
func createNode(projectID string, n tree.Node) edit {
	return edit{projectID: projectID, op: "put", apply: func(cur tree.Tree) (tree.Tree, error) {
		if _, ok := cur.Nodes[n.ID]; ok {
			return tree.Tree{}, fmt.Errorf("tree: create %s: exists: %w", n.ID, asynxModels.ErrValidation)
		}
		return putNode(projectID, n).apply(cur)
	}}
}

// moveNode re-places the row the tree holds: kind and identity are its own, and
// keepParent changes only its order, never restating a parent read earlier.
func moveNode(projectID, id, parentID string, order int, keepParent bool) edit {
	return edit{projectID: projectID, op: "put", apply: func(cur tree.Tree) (tree.Tree, error) {
		n, ok := cur.Nodes[id]
		if !ok {
			return tree.Tree{}, fmt.Errorf("tree: place %s: no such node: %w", id, asynxModels.ErrValidation)
		}
		if !keepParent {
			n.ParentID = parentID
		}
		n.Order = order
		return putNode(projectID, n).apply(cur)
	}}
}

func validNode(n tree.Node) error {
	if n.ID == "" {
		return fmt.Errorf("tree: put node: missing id: %w", asynxModels.ErrValidation)
	}
	if _, err := tree.KindOf(string(n.Kind)); err != nil {
		return fmt.Errorf("tree: put node %s: %w: %w", n.ID, err, asynxModels.ErrValidation)
	}
	return validPlacement(n.ID, n.ParentID, n.Order)
}

func validPlacement(id, parentID string, order int) error {
	if parentID == id {
		return fmt.Errorf("tree: %s under itself: %w", id, asynxModels.ErrValidation)
	}
	if order < 0 {
		return fmt.Errorf("tree: %s: negative order: %w", id, asynxModels.ErrValidation)
	}
	return nil
}

func deleteNode(projectID, id string) edit {
	return edit{projectID: projectID, op: "deleted", apply: func(cur tree.Tree) (tree.Tree, error) {
		if _, ok := cur.Nodes[id]; !ok {
			return tree.Tree{}, fmt.Errorf("tree: delete %s: %w", id, tree.ErrUnknownNode)
		}
		delete(cur.Nodes, id)
		return cur, nil
	}}
}

func putFolder(projectID string, f tree.Folder) edit {
	return edit{projectID: projectID, op: "folder_put", apply: func(cur tree.Tree) (tree.Tree, error) {
		if f.ID == "" {
			return tree.Tree{}, fmt.Errorf("tree: put folder: missing id: %w", asynxModels.ErrValidation)
		}
		cur.Folders[f.ID] = f
		return cur, nil
	}}
}

func deleteFolder(projectID, id string) edit {
	return edit{projectID: projectID, op: "folder_deleted", apply: func(cur tree.Tree) (tree.Tree, error) {
		if _, ok := cur.Folders[id]; !ok {
			return tree.Tree{}, fmt.Errorf("tree: delete folder %s: %w", id, tree.ErrUnknownNode)
		}
		delete(cur.Folders, id)
		return cur, nil
	}}
}

func putChat(projectID, chatID string, p tree.Placement) edit {
	return edit{projectID: projectID, op: "chat_put", apply: func(cur tree.Tree) (tree.Tree, error) {
		if err := validPlacement(chatID, p.ParentID, p.Order); err != nil {
			return tree.Tree{}, err
		}
		cur.Chats[chatID] = p
		return cur, nil
	}}
}

// orderChat changes only the order of the chat's current placement; a chat
// never placed is at its root.
func orderChat(projectID, chatID string, order int) edit {
	return edit{projectID: projectID, op: "chat_put", apply: func(cur tree.Tree) (tree.Tree, error) {
		p := cur.Chats[chatID]
		p.Order = order
		return putChat(projectID, chatID, p).apply(cur)
	}}
}

// dropChat removes a chat's own placement once the chat is gone.
func dropChat(projectID, chatID string) edit {
	return edit{projectID: projectID, op: "chat_dropped", apply: func(cur tree.Tree) (tree.Tree, error) {
		delete(cur.Chats, chatID)
		return cur, nil
	}}
}

// take drops every record of id: the source half of moving a row to the tree
// it belongs in.
func take(projectID, id string) edit {
	return edit{projectID: projectID, op: "taken", apply: func(cur tree.Tree) (tree.Tree, error) {
		rest, _ := cur.Without(id)
		return rest, nil
	}}
}

// give writes records over whatever the tree holds for their ids: the target
// half of a move, which carries the row's current placement.
func give(projectID string, records tree.Tree) edit {
	return edit{projectID: projectID, op: "given", apply: func(cur tree.Tree) (tree.Tree, error) {
		maps.Copy(cur.Nodes, records.Nodes)
		maps.Copy(cur.Folders, records.Folders)
		maps.Copy(cur.Chats, records.Chats)
		return cur, nil
	}}
}
