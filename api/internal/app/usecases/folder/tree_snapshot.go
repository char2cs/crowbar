package folder

import (
	"slices"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// member is one row in a sibling space, folder or workspace alike.
type member struct {
	id      string
	order   int
	created time.Time
}

// compareMembers orders a sibling space: the dense index first, then creation
// time, then id. The tiebreaks are what let rows a user has never ordered —
// every row carries 0 until the first drag at that level — render in creation
// order instead of jittering between two identical requests.
func compareMembers(
	a member,
	b member,
) int {
	if a.order != b.order {
		return a.order - b.order
	}
	if !a.created.Equal(b.created) {
		return a.created.Compare(b.created)
	}
	return strings.Compare(a.id, b.id)
}

// treeSnapshot is one repo's sidebar rows as of a single read, plus the set of
// rows a plan has changed. Every guard and every renumber runs against it, so an
// operation reads the world once and writes only what moved.
//
// The id→index maps exist because a renumber touches every row at a level and
// each touch resolves a row by id; without them the walk is quadratic in the
// repo's row count, which is exactly the shape that looks fine on a demo repo
// and stalls on a real one.
type treeSnapshot struct {
	folders     []domain.Folder
	workspaces  []domain.Workspace
	folderAt    map[string]int
	workspaceAt map[string]int
	dirty       map[string]bool
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
		dirty:       map[string]bool{},
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
	return t
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

func (t *treeSnapshot) add(
	row domain.Folder,
) {
	t.folderAt[row.ID] = len(t.folders)
	t.folders = append(t.folders, row)
	t.dirty[row.ID] = true
}

// drop removes a folder from the snapshot. The index map is rebuilt because the
// slice delete shifts every entry after the hole.
func (t *treeSnapshot) drop(
	id string,
) {
	if _, ok := t.folderAt[id]; !ok {
		return
	}
	delete(t.dirty, id)
	t.folders = slices.DeleteFunc(t.folders, func(f domain.Folder) bool { return f.ID == id })
	t.folderAt = make(map[string]int, len(t.folders))
	for i, f := range t.folders {
		t.folderAt[f.ID] = i
	}
}

// members returns container's sibling space in render order. A container id is a
// folder id, a workspace id, or "" for the repo root; folder ids and workspace
// ids are UUIDs and never collide, so one membership rule covers all three.
func (t *treeSnapshot) members(
	container string,
) []member {
	rows := make([]member, 0, len(t.folders)+len(t.workspaces))
	for _, f := range t.folders {
		if f.ParentID == container {
			rows = append(rows, member{id: f.ID, order: f.Order})
		}
	}
	for _, w := range t.workspaces {
		if t.containerOf(w) == container {
			rows = append(rows, member{id: w.ID, order: w.Order, created: w.CreatedAt})
		}
	}
	slices.SortFunc(rows, compareMembers)
	return rows
}

// indexOf returns a row's current position in its sibling space, or 0 when it is
// not there (a caller reordering a row it has just placed elsewhere).
func (t *treeSnapshot) indexOf(
	container string,
	id string,
) int {
	at := slices.IndexFunc(t.members(container), func(m member) bool { return m.id == id })
	return max(at, 0)
}

// reorder assigns dense 0..n-1 orders across container's whole sibling space,
// optionally lifting placed out and re-inserting it at target first. A target
// below zero means "placed is not being positioned here" — used to densify the
// level a row has just left.
func (t *treeSnapshot) reorder(
	container string,
	placed string,
	target int,
) {
	rows := t.members(container)
	if placed != "" && target >= 0 {
		rows = reinsert(rows, placed, target)
	}
	for i, row := range rows {
		t.setOrder(row.id, i)
	}
}

// reinsert lifts id out of rows and puts it back at target, clamped into range
// so a client index computed against a stale list lands at an end rather than
// failing the whole move.
func reinsert(
	rows []member,
	id string,
	target int,
) []member {
	at := slices.IndexFunc(rows, func(m member) bool { return m.id == id })
	if at < 0 {
		return rows
	}
	moved := rows[at]
	rows = slices.Delete(rows, at, at+1)
	return slices.Insert(rows, min(max(target, 0), len(rows)), moved)
}

func (t *treeSnapshot) setOrder(
	id string,
	order int,
) {
	if row := t.folder(id); row != nil {
		if row.Order != order {
			row.Order = order
			t.dirty[id] = true
		}
		return
	}
	row := t.workspace(id)
	if row != nil && row.Order != order {
		row.Order = order
		t.dirty[id] = true
	}
}

func (t *treeSnapshot) setFolderParent(
	id string,
	parentID string,
) {
	row := t.folder(id)
	if row == nil || row.ParentID == parentID {
		return
	}
	row.ParentID = parentID
	t.dirty[id] = true
}

func (t *treeSnapshot) setWorkspaceFolder(
	id string,
	folderID string,
) {
	row := t.workspace(id)
	if row == nil || row.FolderID == folderID {
		return
	}
	row.FolderID = folderID
	t.dirty[id] = true
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
// root.
func (t *treeSnapshot) reparentChildren(
	id string,
	parentID string,
) (folderDest, workspaceDest string) {
	workspaceDest = parentID
	if parentID != "" && t.folder(parentID) == nil {
		workspaceDest = ""
	}
	for _, f := range slices.Clone(t.folders) {
		if f.ParentID == id {
			t.setFolderParent(f.ID, parentID)
		}
	}
	for _, w := range t.workspaces {
		if w.FolderID == id {
			t.setWorkspaceFolder(w.ID, workspaceDest)
		}
	}
	t.drop(id)
	return parentID, workspaceDest
}

// reaches reports whether walking UP from container ever arrives at ancestor —
// the cycle test for every move. The visited set bounds the walk so an already
// corrupt edge is answered rather than spun on.
func (t *treeSnapshot) reaches(
	container string,
	ancestor string,
) bool {
	visited := map[string]bool{}
	for at := container; at != "" && !visited[at]; at = t.parentOf(at) {
		if at == ancestor {
			return true
		}
		visited[at] = true
	}
	return false
}

func (t *treeSnapshot) parentOf(
	id string,
) string {
	if row := t.folder(id); row != nil {
		return row.ParentID
	}
	if row := t.workspace(id); row != nil {
		return t.containerOf(*row)
	}
	return ""
}

// dirtyIDs returns the changed rows in a stable order, so a partial failure is
// reproducible rather than arbitrary.
func (t *treeSnapshot) dirtyIDs() []string {
	ids := make([]string, 0, len(t.dirty))
	for id := range t.dirty {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
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
