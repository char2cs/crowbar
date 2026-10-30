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

	mu     sync.Mutex
	held   map[string]heldTree
	writes map[string]int64 // per project, bumped by every write and Forget
}

// heldTree is a decoded tree and the event version it is at; a tree read on a
// miss has no known version (-1), so any write's result replaces it.
type heldTree struct {
	tree    tree.Tree
	version int64
}

// New builds the repository over the Tree's asynx instance; it must be the only
// writer of that instance, or what it holds can fall behind.
func New(ax asynx.Asynx[tree.Tree]) *Repository {
	return &Repository{ax: ax, held: map[string]heldTree{}, writes: map[string]int64{}}
}

// Get is projectID's tree; a project nothing was placed in has an empty one.
func (r *Repository) Get(ctx context.Context, projectID string) (tree.Tree, error) {
	r.mu.Lock()
	h, ok := r.held[projectID]
	gen := r.writes[projectID]
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
	// A write that finished during the read is newer than what was read.
	if _, filled := r.held[projectID]; !filled && r.writes[projectID] == gen {
		r.held[projectID] = heldTree{tree: t.Clone(), version: -1}
	}
	return t, nil
}

// hold records what a write returned, unless a later write already did.
func (r *Repository) hold(projectID string, t tree.Tree, version int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes[projectID]++
	if h, ok := r.held[projectID]; ok && h.version > version {
		return
	}
	r.held[projectID] = heldTree{tree: tree.Empty(projectID).Merge(t), version: version}
}

func (r *Repository) evict(projectID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes[projectID]++
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
	evt, err := r.ax.SendWait(ctx, c)
	if err != nil {
		// A refused write changed nothing, but one that failed after its commit
		// leaves the held tree unknown: read it again.
		r.evict(c.projectID)
		return tree.Tree{}, fmt.Errorf("tree: %s %s: %w", c.op, c.projectID, err)
	}
	r.hold(c.projectID, evt.Aggregate, evt.Version)
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
