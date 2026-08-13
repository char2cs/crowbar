package folder

import (
	"slices"

	"github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// treeSnapshot is one repo's sidebar rows as of a single read, paired with the
// kind-agnostic plan that does the ordering. Every guard and every renumber runs
// against it, so an operation reads the world once and writes only what moved.
//
// The split is the point. The plan knows a parent id and a dense index and
// nothing else; this type keeps everything the plan must not learn — that a
// workspace has TWO parent edges, that only one of them is a sidebar gesture,
// and that the other one is git lineage a drag may never rewrite. Placement
// therefore lives in the plan alone and is stamped back onto a domain row only
// when that row is about to be returned or written.
type treeSnapshot struct {
	folders     []domain.Folder
	workspaces  []domain.Workspace
	folderAt    map[string]int
	workspaceAt map[string]int
	plan        tree.Tree
}

// newTreeSnapshot builds the snapshot, dropping the workspace rows that are not
// sidebar rows: tombstoned records, and the repo home (the frontend lifts it out
// of the tree and opens it from the repo header, so it must not hold a slot in
// any sibling space).
func newTreeSnapshot(
	folders []domain.Folder,
	workspaces []domain.Workspace,
) *treeSnapshot {
	t := &treeSnapshot{
		folders:     folders,
		folderAt:    make(map[string]int, len(folders)),
		workspaceAt: make(map[string]int, len(workspaces)),
	}
	for i, f := range folders {
		t.folderAt[f.ID] = i
	}
	for _, ws := range workspaces {
		if ws.Status == domain.WorkspaceStatusDeleted || ws.IsDefault {
			continue
		}
		t.workspaceAt[ws.ID] = len(t.workspaces)
		t.workspaces = append(t.workspaces, ws)
	}
	t.plan = tree.New(t.nodes())
	return t
}

// nodes flattens both row kinds into the one sibling space they share. A folder
// hands over its ParentID as it stands; a workspace hands over the container the
// fork overlay resolves it to, which is the only edge of the two the plan is
// ever allowed to see.
func (t *treeSnapshot) nodes() []tree.Node {
	nodes := make([]tree.Node, 0, len(t.folders)+len(t.workspaces))
	for _, f := range t.folders {
		nodes = append(nodes, tree.Node{ID: f.ID, ParentID: f.ParentID, Order: f.Order})
	}
	for _, w := range t.workspaces {
		nodes = append(nodes, tree.Node{
			ID:        w.ID,
			ParentID:  t.containerOf(w),
			Order:     w.Order,
			CreatedAt: w.CreatedAt,
		})
	}
	return nodes
}

func (t *treeSnapshot) folder(
	id string,
) *domain.Folder {
	at, ok := t.folderAt[id]
	if !ok {
		return nil
	}
	return &t.folders[at]
}

func (t *treeSnapshot) workspace(
	id string,
) *domain.Workspace {
	at, ok := t.workspaceAt[id]
	if !ok {
		return nil
	}
	return &t.workspaces[at]
}

// placedFolder returns a folder row with the placement the plan settled on
// stamped onto it. Nothing else reads a stored ParentID or Order once a plan is
// running: they are the plan's answer, and a row that skipped this step would be
// saved with the placement it had before the drag.
func (t *treeSnapshot) placedFolder(
	id string,
) *domain.Folder {
	row := t.folder(id)
	node, ok := t.plan.Node(id)
	if row == nil || !ok {
		return nil
	}
	row.ParentID = node.ParentID
	row.Order = node.Order
	return row
}

// placedWorkspace returns a workspace row with the plan's index stamped on. Only
// the index: a workspace's container is derived from FolderID and the fork
// lineage, so writing the plan's parent id back would be writing an edge the
// sidebar is not allowed to touch.
func (t *treeSnapshot) placedWorkspace(
	id string,
) *domain.Workspace {
	row := t.workspace(id)
	node, ok := t.plan.Node(id)
	if row == nil || !ok {
		return nil
	}
	row.Order = node.Order
	return row
}

func (t *treeSnapshot) add(
	row domain.Folder,
) {
	t.folderAt[row.ID] = len(t.folders)
	t.folders = append(t.folders, row)
	t.plan.Add(tree.Node{ID: row.ID, ParentID: row.ParentID, Order: row.Order})
}

// drop removes a folder from the snapshot and from the plan. The index map is
// rebuilt because the slice delete shifts every entry after the hole.
func (t *treeSnapshot) drop(
	id string,
) {
	if _, ok := t.folderAt[id]; !ok {
		return
	}
	t.plan.Drop(id)
	t.folders = slices.DeleteFunc(t.folders, func(f domain.Folder) bool { return f.ID == id })
	t.folderAt = make(map[string]int, len(t.folders))
	for i, f := range t.folders {
		t.folderAt[f.ID] = i
	}
}

// setWorkspaceFolder writes the organisational edge and tells the plan where the
// row now renders. The two are separate calls because they are separate facts: a
// compatible folder may sit between a workspace and its fork parent, in which
// case the container changes and the lineage does not, and filing a row back to
// the root clears FolderID while leaving it in its fork parent's space. The Touch
// covers the second case, where the stored edge moved but the container did not
// and the write would otherwise be dropped from the plan.
func (t *treeSnapshot) setWorkspaceFolder(
	id string,
	folderID string,
) {
	row := t.workspace(id)
	if row == nil || row.FolderID == folderID {
		return
	}
	row.FolderID = folderID
	t.plan.SetParent(id, t.containerFor(*row, folderID))
	t.plan.Touch(id)
}

// reparentChildren empties a folder about to be deleted, handing what it held to
// the folder's own parent, and returns the two destination containers.
//
// They differ in one case, and it is the case that makes a cascade wrong twice
// over: a folder may hang off a WORKSPACE, and a workspace's only expressible
// containers are a folder or the repo root — the other edge into a workspace is
// the fork lineage, which a sidebar gesture must never rewrite. So child folders
// follow the deleted folder's parent wherever it is, while child workspaces
// follow it only when it is another folder, and otherwise surface at the repo
// root. The workspaces move first for exactly that reason: whatever is still a
// child by the time the plan reparents is a row that may follow its siblings.
func (t *treeSnapshot) reparentChildren(
	id string,
	parentID string,
) (folderDest, workspaceDest string) {
	workspaceDest = parentID
	if parentID != "" && t.folder(parentID) == nil {
		workspaceDest = ""
	}
	for _, w := range t.workspaces {
		if w.FolderID == id {
			t.setWorkspaceFolder(w.ID, workspaceDest)
		}
	}
	t.plan.Reparent(id, parentID)
	t.drop(id)
	return parentID, workspaceDest
}

// visibleWorkspaceParent returns the lineage parent that is also a row in this
// sidebar tree. Repo home is deliberately absent from the snapshot, so a row
// forked from it belongs to the visible root just like a row with no ParentID.
func (t *treeSnapshot) visibleWorkspaceParent(
	ws domain.Workspace,
) string {
	if ws.ParentID != "" && t.workspace(ws.ParentID) != nil {
		return ws.ParentID
	}
	return ""
}

// folderWorkspaceAnchor returns the nearest workspace above a folder. Empty is
// the repo root. The visited set makes corrupt persisted folder cycles degrade
// to root instead of spinning while a placement is being validated.
func (t *treeSnapshot) folderWorkspaceAnchor(
	folderID string,
) string {
	visited := map[string]bool{}
	for at := folderID; at != "" && !visited[at]; {
		visited[at] = true
		if row := t.folder(at); row != nil {
			at = row.ParentID
			continue
		}
		if t.workspace(at) != nil {
			return at
		}
		return ""
	}
	return ""
}

// containerFor resolves the sibling space for ws if it carried folderID. A
// compatible folder may sit between a workspace and its fork child, while the
// git ParentID remains untouched. An incompatible stale edge falls back to the
// real visible fork parent so the tree remains renderable.
func (t *treeSnapshot) containerFor(
	ws domain.Workspace,
	folderID string,
) string {
	parent := t.visibleWorkspaceParent(ws)
	if folderID != "" && t.folder(folderID) != nil && t.folderWorkspaceAnchor(folderID) == parent {
		return folderID
	}
	return parent
}

func (t *treeSnapshot) containerOf(
	ws domain.Workspace,
) string {
	return t.containerFor(ws, ws.FolderID)
}

// folderAnchorAfterMove answers the workspace anchor a folder chain would have
// after moving `moved` to `destination`, plus whether that chain actually
// crosses the moved row. A workspace encountered first owns the deeper folder
// space and shields it from an ancestor folder move.
func (t *treeSnapshot) folderAnchorAfterMove(
	folderID string,
	moved string,
	destination string,
) (string, bool) {
	visited := map[string]bool{}
	touched := false
	for at := folderID; at != "" && !visited[at]; {
		visited[at] = true
		if at == moved {
			touched = true
			at = destination
			continue
		}
		if row := t.folder(at); row != nil {
			at = row.ParentID
			continue
		}
		if t.workspace(at) != nil {
			return at, touched
		}
		return "", touched
	}
	return "", touched
}

// folderMovePreservesForks keeps a folder move from carrying any filed
// workspace away from its real fork-parent space. Empty folders remain freely
// movable; folders containing rows can still move anywhere under the same
// workspace anchor.
func (t *treeSnapshot) folderMovePreservesForks(
	moved string,
	destination string,
) bool {
	for _, ws := range t.workspaces {
		if ws.FolderID == "" {
			continue
		}
		anchor, touched := t.folderAnchorAfterMove(ws.FolderID, moved, destination)
		if touched && anchor != t.visibleWorkspaceParent(ws) {
			return false
		}
	}
	return true
}
