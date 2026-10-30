// Package tree stores the Tree aggregate: one per project (aggregate id = the
// project id), holding where every sidebar row of that project hangs. Every
// write is one command against the whole project tree, sent with SendWait, and
// Get is the only read.
package tree

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// Repository is the Tree aggregate's store. Every write goes through send, so
// it keeps each project's decoded tree as the last write returned it.
type Repository struct {
	ax asynx.Asynx[tree.Tree]

	mu   sync.Mutex
	held map[string]heldTree
	// writes is bumped by every write that ends, so a miss read during one is not held.
	writes map[string]int64
	// epoch is bumped by every write that may have committed unseen and by
	// Forget: a write begun in an earlier epoch is not held.
	epoch map[string]int64
	// inflight counts each project's writes under way: a read then may see one
	// whose result has not come back, so nothing it or an older write saw fills
	// an empty slot until they all end.
	inflight map[string]int
	// newest is the highest version any write came back with.
	newest map[string]int64
}

// heldTree is a decoded tree and the event version it is at; a tree read on a
// miss has no known version (-1), so any later write's result replaces it.
type heldTree struct {
	tree    tree.Tree
	version int64
}

// New builds the repository over the Tree's asynx instance; it must be the only
// writer of that instance, or what it holds can fall behind.
func New(ax asynx.Asynx[tree.Tree]) *Repository {
	return &Repository{
		ax: ax, held: map[string]heldTree{}, writes: map[string]int64{},
		epoch: map[string]int64{}, inflight: map[string]int{}, newest: map[string]int64{},
	}
}

// Get is projectID's tree; a project nothing was placed in has an empty one.
func (r *Repository) Get(ctx context.Context, projectID string) (tree.Tree, error) {
	r.mu.Lock()
	h, ok := r.held[projectID]
	gen, quiet := r.writes[projectID], r.inflight[projectID] == 0
	r.mu.Unlock()
	if ok {
		return h.tree.Clone(), nil
	}
	t, err := r.ax.Get(ctx, projectID)
	if errors.Is(err, asynxModels.ErrNotFound) {
		t, err = tree.Tree{}, nil
	}
	if err != nil {
		return tree.Tree{}, fmt.Errorf("tree: get %s: %w", projectID, err)
	}
	t = tree.Empty(projectID).Merge(t)
	r.mu.Lock()
	defer r.mu.Unlock()
	_, filled := r.held[projectID]
	if quiet && !filled && r.writes[projectID] == gen && r.inflight[projectID] == 0 {
		r.held[projectID] = heldTree{tree: t.Clone(), version: -1}
	}
	return t, nil
}

// begin marks a write of projectID under way and returns the epoch it began in.
func (r *Repository) begin(projectID string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inflight[projectID]++
	return r.epoch[projectID]
}

// end closes a write begun in epoch. A committed one replaces an older held
// tree, or fills an empty slot once no write is under way and it is the newest;
// one that may have committed unseen empties the slot and moves the epoch on.
func (r *Repository) end(projectID string, epoch int64, evt asynxModels.Event[tree.Tree], err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inflight[projectID]--
	r.writes[projectID]++
	switch {
	case err != nil && refused(err):
	case err != nil:
		r.epoch[projectID]++
		delete(r.held, projectID)
	case r.epoch[projectID] != epoch:
	default:
		r.newest[projectID] = max(r.newest[projectID], evt.Version)
		r.holdLocked(projectID, evt)
	}
}

func (r *Repository) holdLocked(projectID string, evt asynxModels.Event[tree.Tree]) {
	h, ok := r.held[projectID]
	if ok && h.version >= evt.Version {
		return
	}
	if !ok && (r.inflight[projectID] > 0 || evt.Version < r.newest[projectID]) {
		return
	}
	r.held[projectID] = heldTree{tree: tree.Empty(projectID).Merge(evt.Aggregate), version: evt.Version}
}

// refused is a write its command turned down before anything was committed.
func refused(err error) bool {
	return errors.Is(err, asynxModels.ErrValidation) || errors.Is(err, tree.ErrUnknownNode)
}

func (r *Repository) evict(projectID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes[projectID]++
	r.epoch[projectID]++
	delete(r.held, projectID)
}

// Seed merges t into its project's tree: every record it lacks is added, every
// record it has is kept as written.
func (r *Repository) Seed(ctx context.Context, t tree.Tree) (tree.Tree, error) {
	return r.send(ctx, seed(t))
}

// Put creates or repositions a node.
func (r *Repository) Put(ctx context.Context, projectID string, n tree.Node) (tree.Tree, error) {
	return r.send(ctx, putNode(projectID, n))
}

// Create mints a node; one the tree already holds is refused.
func (r *Repository) Create(ctx context.Context, projectID string, n tree.Node) (tree.Tree, error) {
	return r.send(ctx, createNode(projectID, n))
}

// Move re-places a node the tree holds; one it does not hold is refused.
func (r *Repository) Move(ctx context.Context, projectID, id, parentID string, order int) (tree.Tree, error) {
	return r.send(ctx, moveNode(projectID, id, parentID, order, false))
}

// Order changes a held node's order and keeps the parent it holds when it lands.
func (r *Repository) Order(ctx context.Context, projectID, id string, order int) (tree.Tree, error) {
	return r.send(ctx, moveNode(projectID, id, "", order, true))
}

// Delete removes a node; one the tree does not hold is tree.ErrUnknownNode.
func (r *Repository) Delete(ctx context.Context, projectID, id string) (tree.Tree, error) {
	return r.send(ctx, deleteNode(projectID, id))
}

// PutFolder writes a folder's identity.
func (r *Repository) PutFolder(ctx context.Context, projectID string, f tree.Folder) (tree.Tree, error) {
	return r.send(ctx, putFolder(projectID, f))
}

// DeleteFolder removes a folder's identity; an unknown one is tree.ErrUnknownNode.
func (r *Repository) DeleteFolder(ctx context.Context, projectID, id string) (tree.Tree, error) {
	return r.send(ctx, deleteFolder(projectID, id))
}

// PutChat writes a chat's own placement.
func (r *Repository) PutChat(
	ctx context.Context,
	projectID string,
	chatID string,
	p tree.Placement,
) (tree.Tree, error) {
	return r.send(ctx, putChat(projectID, chatID, p))
}

// OrderChat changes a chat's order and keeps the parent it holds when it lands.
func (r *Repository) OrderChat(ctx context.Context, projectID, chatID string, order int) (tree.Tree, error) {
	return r.send(ctx, orderChat(projectID, chatID, order))
}

// DropChat removes a chat's own placement; none is no error.
func (r *Repository) DropChat(ctx context.Context, projectID, chatID string) (tree.Tree, error) {
	return r.send(ctx, dropChat(projectID, chatID))
}

// Take removes every record of id; none is no error.
func (r *Repository) Take(ctx context.Context, projectID, id string) (tree.Tree, error) {
	return r.send(ctx, take(projectID, id))
}

// Give writes records into projectID's tree over what it holds for their ids.
func (r *Repository) Give(ctx context.Context, projectID string, records tree.Tree) (tree.Tree, error) {
	return r.send(ctx, give(projectID, records))
}

// Forget erases projectID's tree; a project without one is no error.
func (r *Repository) Forget(ctx context.Context, projectID string) error {
	err := r.ax.Forget(ctx, projectID)
	r.evict(projectID)
	if err != nil && !errors.Is(err, asynxModels.ErrValidation) {
		return fmt.Errorf("tree: forget %s: %w", projectID, err)
	}
	return nil
}

func (r *Repository) send(ctx context.Context, c edit) (tree.Tree, error) {
	epoch := r.begin(c.projectID)
	evt, err := r.ax.SendWait(ctx, c)
	r.end(c.projectID, epoch, evt, err)
	if err != nil {
		return tree.Tree{}, fmt.Errorf("tree: %s %s: %w", c.op, c.projectID, err)
	}
	return evt.Aggregate, nil
}

// schemaVersion is tree.Tree's event schema; bump it with an upcaster when a
// field history patches goes.
const schemaVersion = 1

// Schema stamps the Tree's events with their schema version.
func Schema(b *asynx.Builder[tree.Tree]) {
	b.WithSchemaVersion(schemaVersion)
}

// WaitPublish waits until every committed Tree event reached its subscribers.
func (r *Repository) WaitPublish() {
	r.ax.WaitPublish()
}
