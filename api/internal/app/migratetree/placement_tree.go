package migratetree

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	repotree "github.com/char2cs/crowbar/api/internal/app/repositories/tree"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// namedInHalt caps how many ids a failed Verify names.
const namedInHalt = 10

// PlacementTreeImpl copies every retired placement record into its project's
// Tree, a project per checkpoint. It deletes nothing, so a rolled-back daemon
// still reads the records it left.
func PlacementTreeImpl(ports *repotree.Ports, src LegacySources) migrate.Impl {
	return migrate.Impl{
		Apply: func(ctx context.Context, _ migrate.Env, cp *migrate.Checkpoints) (migrate.Counts, error) {
			h, err := ReadLegacy(ctx, src)
			if err != nil {
				return nil, err
			}
			return nil, seedAll(ctx, ports, h, cp)
		},
		Verify: func(ctx context.Context, _ migrate.Env) error {
			h, err := ReadLegacy(ctx, src)
			if err != nil {
				return err
			}
			return verify(ctx, ports, h)
		},
	}
}

func seedAll(ctx context.Context, ports *repotree.Ports, h LegacyHome, cp *migrate.Checkpoints) error {
	trees, rehung := convert(h)
	for _, t := range trees {
		key := "project:" + t.ProjectID
		if cp.Done(key) {
			continue
		}
		if _, err := ports.Seed(ctx, t); err != nil {
			return fmt.Errorf("migratetree: seed %s: %w", t.ProjectID, err)
		}
		counts := migrate.Counts{
			"nodes": int64(len(t.Nodes)), "folders": int64(len(t.Folders)), "chats": int64(len(t.Chats)),
		}
		if n := rehung[t.ProjectID]; n > 0 {
			counts["rehung_to_root"] = int64(n)
		}
		if err := cp.Mark(key, counts); err != nil {
			return fmt.Errorf("migratetree: mark %s: %w", t.ProjectID, err)
		}
	}
	return nil
}

// verify checks every record the conversion names is held by a Tree and no
// held row hangs off a parent nothing names.
func verify(ctx context.Context, ports *repotree.Ports, h LegacyHome) error {
	want, _ := convert(h)
	ids := []string{tree.Unplaced}
	for _, p := range h.Projects {
		ids = append(ids, p.ID)
	}
	held := tree.Empty("")
	for _, id := range ids {
		t, err := ports.Tree(ctx, id)
		if err != nil {
			return fmt.Errorf("migratetree: verify: %w", err)
		}
		held = held.Merge(t)
	}
	var missing []string
	for _, t := range want {
		missing = append(missing, absent(t.Nodes, held.Nodes, "node")...)
		missing = append(missing, absent(t.Folders, held.Folders, "folder")...)
		missing = append(missing, absent(t.Chats, held.Chats, "chat")...)
	}
	if err := listed("placement records were not copied", missing); err != nil {
		return err
	}
	known := knownIDs(h, held)
	var dangling []string
	for id, n := range held.Nodes {
		if n.ParentID != "" && !known[n.ParentID] {
			dangling = append(dangling, id)
		}
	}
	for id, p := range held.Chats {
		if p.ParentID != "" && !known[p.ParentID] {
			dangling = append(dangling, id)
		}
	}
	return listed("rows hang off a parent nothing names", dangling)
}

func absent[V any](want, held map[string]V, kind string) []string {
	var out []string
	for id := range want {
		if _, ok := held[id]; !ok {
			out = append(out, kind+" "+id)
		}
	}
	return out
}

func listed(what string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	slices.Sort(ids)
	named := strings.Join(ids[:min(len(ids), namedInHalt)], ", ")
	if len(ids) > namedInHalt {
		named += fmt.Sprintf(" and %d more", len(ids)-namedInHalt)
	}
	return fmt.Errorf("%d %s: %s", len(ids), what, named)
}

func knownIDs(h LegacyHome, held tree.Tree) map[string]bool {
	known := map[string]bool{}
	for _, ids := range [][]string{mapKeys(held.Nodes), mapKeys(held.Folders), mapKeys(held.Chats)} {
		for _, id := range ids {
			known[id] = true
		}
	}
	for _, w := range h.Workspaces {
		known[w.ID] = true
	}
	for _, r := range h.Repos {
		known[r.ID] = true
	}
	for _, c := range h.Chats {
		known[c.ID] = true
	}
	return known
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	return out
}

// convert files every record in the Tree of the project it belongs to, at its
// legacy parent and order; one whose parent no record names hangs at its
// scope's root. It returns the trees by project and the rows re-hung in each.
func convert(h LegacyHome) ([]tree.Tree, map[string]int) {
	pl := newPlacer(h)
	trees := map[string]tree.Tree{}
	rehung := map[string]int{}
	into := func(id string) tree.Tree {
		p := pl.projectOf(id, map[string]bool{})
		if p == "" {
			p = tree.Unplaced
		}
		if _, ok := trees[p]; !ok {
			trees[p] = tree.Empty(p)
		}
		return trees[p]
	}
	for _, n := range h.Nodes {
		t := into(n.ID)
		if n.ParentID != "" && !pl.known[n.ParentID] {
			n.ParentID = ""
			rehung[t.ProjectID]++
		}
		t.Nodes[n.ID] = n
	}
	for _, f := range pl.folders {
		into(f.ID).Folders[f.ID] = f
	}
	for _, c := range h.Chats {
		t := into(c.ID)
		p := c.Placement
		if p.ParentID != "" && !pl.known[p.ParentID] {
			p.ParentID = ""
			rehung[t.ProjectID]++
		}
		t.Chats[c.ID] = p
	}
	out := make([]tree.Tree, 0, len(trees))
	for _, t := range trees {
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b tree.Tree) int { return cmp.Compare(a.ProjectID, b.ProjectID) })
	return out, rehung
}

// oldestProject is the project whose home was made first: a project keeps no
// creation time of its own, and its home is made with it.
func oldestProject(h LegacyHome) string {
	var oldest *domain.Workspace
	for i := range h.Workspaces {
		w := &h.Workspaces[i]
		if w.Kind != domain.WorkspaceKindHome {
			continue
		}
		if oldest == nil || w.CreatedAt.Before(oldest.CreatedAt) ||
			(w.CreatedAt.Equal(oldest.CreatedAt) && w.ProjectID < oldest.ProjectID) {
			oldest = w
		}
	}
	if oldest == nil {
		return ""
	}
	return oldest.ProjectID
}

// placer names each record's project by the rules the live Locator follows: a
// workspace and a repo directly, a chat through its workspace, a folder through
// its repo or home, and anything else through its parent.
type placer struct {
	wsProject, repoProject, chatWS, homeOf map[string]string
	parent                                 map[string]string
	children                               map[string][]string
	folders                                map[string]tree.Folder
	known                                  map[string]bool
}

func newPlacer(h LegacyHome) *placer {
	pl := &placer{
		wsProject: map[string]string{}, repoProject: map[string]string{}, chatWS: map[string]string{},
		homeOf: map[string]string{}, parent: map[string]string{}, children: map[string][]string{},
		folders: map[string]tree.Folder{}, known: map[string]bool{},
	}
	for _, w := range h.Workspaces {
		pl.wsProject[w.ID], pl.known[w.ID] = w.ProjectID, true
		if w.Kind == domain.WorkspaceKindHome {
			pl.homeOf[w.ProjectID] = w.ID
		}
	}
	for _, r := range h.Repos {
		pl.repoProject[r.ID], pl.known[r.ID] = r.ProjectID, true
	}
	for _, c := range h.Chats {
		pl.chatWS[c.ID], pl.known[c.ID] = c.WorkspaceID, true
		pl.hang(c.ID, c.Placement.ParentID)
	}
	for _, n := range h.Nodes {
		pl.known[n.ID] = true
		pl.hang(n.ID, n.ParentID)
	}
	for _, f := range h.Folders {
		pl.folders[f.ID], pl.known[f.ID] = f, true
	}
	fallback := pl.homeOf[oldestProject(h)]
	for id, f := range pl.folders {
		if f.RepoID == "" && f.HomeID == "" {
			f.HomeID = cmp.Or(pl.homeNear(id, map[string]bool{}), fallback)
			pl.folders[id] = f
		}
	}
	return pl
}

// hang records id under parent; a Node row's parent wins over the chat's copy.
func (pl *placer) hang(id, parent string) {
	if parent == "" {
		return
	}
	pl.parent[id] = parent
	pl.children[parent] = append(pl.children[parent], id)
}

func (pl *placer) projectOf(id string, seen map[string]bool) string {
	if seen[id] {
		return ""
	}
	seen[id] = true
	if p, ok := pl.wsProject[id]; ok {
		return p
	}
	if p, ok := pl.repoProject[id]; ok {
		return p
	}
	if p := pl.wsProject[pl.chatWS[id]]; p != "" {
		return p
	}
	if f, ok := pl.folders[id]; ok {
		if p := cmp.Or(pl.repoProject[f.RepoID], pl.wsProject[f.HomeID]); p != "" {
			return p
		}
	}
	if parent := pl.parent[id]; parent != "" {
		return pl.projectOf(parent, seen)
	}
	return ""
}

// homeNear is the home a legacy home folder's neighbours name: a chat above
// or beneath it names its workspace, a folder its home, a repo its project's.
func (pl *placer) homeNear(id string, seen map[string]bool) string {
	seen[id] = true
	for _, n := range append([]string{pl.parent[id]}, pl.children[id]...) {
		if home := pl.homeOfRow(n, seen); home != "" {
			return home
		}
	}
	return ""
}

func (pl *placer) homeOfRow(id string, seen map[string]bool) string {
	if id == "" || seen[id] {
		return ""
	}
	if ws, ok := pl.chatWS[id]; ok {
		return ws
	}
	if f, ok := pl.folders[id]; ok {
		return cmp.Or(f.HomeID, pl.homeNear(id, seen))
	}
	if p, ok := pl.repoProject[id]; ok {
		return pl.homeOf[p]
	}
	return ""
}
