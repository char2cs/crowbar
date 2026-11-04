package mocks

import (
	"context"
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// NodePlacements is a fake project.NodePlacements: it holds every Node row
// minted so far and records the two placement writes apart — a renumber may
// write an index and must be unable to write a parent.
type NodePlacements struct {
	Rows      []tree.Node
	CreateErr error
	GetErr    error
	ListErr   error
	OrderErr  error
	PlaceErr  error
	// OrderErrForID and PlaceErrForID fail only that id's SetOrder/SetPlacement, so one
	// sibling's write can fail while another in the same operation succeeds.
	OrderErrForID map[string]error
	PlaceErrForID map[string]error
	Placed        []NodePlacementWrite
	Ordered       []NodeOrderWrite
	// Forgotten records every id Forget purged, in call order — so a test can
	// prove a rolled-back import's Node row was actually taken back out, not
	// merely that Forget was never asked to fail.
	Forgotten []string
	ForgetErr error
}

// NodePlacementWrite is one recorded call to SetPlacement: the node moved,
// and where to.
type NodePlacementWrite struct {
	ID       string
	ParentID string
	Order    int
}

// NodeOrderWrite is one recorded call to SetOrder: a node a densify
// renumbered, and the index it was given. It carries no parent, which is the
// whole point.
type NodeOrderWrite struct {
	ID    string
	Order int
}

// NewNodePlacements returns an empty NodePlacements.
func NewNodePlacements() *NodePlacements {
	return &NodePlacements{}
}

func (s *NodePlacements) Create(
	ctx context.Context,
	id string,
	kind tree.Kind,
	parentID string,
	order int,
) (tree.Node, error) {
	if s.CreateErr != nil {
		return tree.Node{}, s.CreateErr
	}
	for _, row := range s.Rows {
		if row.ID == id {
			return tree.Node{}, fmt.Errorf("node: create: exists: %w", asynxModels.ErrValidation)
		}
	}
	n := tree.Node{ID: id, Kind: kind, ParentID: parentID, Order: order}
	s.Rows = append(s.Rows, n)
	return n, nil
}

func (s *NodePlacements) GetNode(
	ctx context.Context,
	id string,
) (tree.Node, error) {
	if s.GetErr != nil {
		return tree.Node{}, s.GetErr
	}
	for _, n := range s.Rows {
		if n.ID == id {
			return n, nil
		}
	}
	return tree.Node{}, tree.ErrUnknownNode
}

func (s *NodePlacements) ListByParent(
	ctx context.Context,
	parentID string,
) ([]tree.Node, error) {
	if s.ListErr != nil {
		return nil, s.ListErr
	}
	rows := make([]tree.Node, 0, len(s.Rows))
	for _, n := range s.Rows {
		if n.ParentID == parentID {
			rows = append(rows, n)
		}
	}
	return rows, nil
}

func (s *NodePlacements) ListAll(
	ctx context.Context,
) ([]tree.Node, error) {
	if s.ListErr != nil {
		return nil, s.ListErr
	}
	return append([]tree.Node(nil), s.Rows...), nil
}

// SetOrder writes the index and leaves the parent as it stands. It refuses an id with no
// row, as the real command's Validate does; a silently no-op fake once hid a real bug.
func (s *NodePlacements) SetOrder(
	ctx context.Context,
	id string,
	order int,
) error {
	if s.OrderErr != nil {
		return s.OrderErr
	}
	if err := s.OrderErrForID[id]; err != nil {
		return err
	}
	for i := range s.Rows {
		if s.Rows[i].ID == id {
			s.Rows[i].Order = order
			s.Ordered = append(s.Ordered, NodeOrderWrite{ID: id, Order: order})
			return nil
		}
	}
	return fmt.Errorf("node: set order: no node: %w", asynxModels.ErrValidation)
}

func (s *NodePlacements) SetPlacement(
	ctx context.Context,
	id string,
	parentID string,
	order int,
) error {
	if s.PlaceErr != nil {
		return s.PlaceErr
	}
	if err := s.PlaceErrForID[id]; err != nil {
		return err
	}
	for i := range s.Rows {
		if s.Rows[i].ID == id {
			s.Rows[i].ParentID = parentID
			s.Rows[i].Order = order
			s.Placed = append(s.Placed, NodePlacementWrite{ID: id, ParentID: parentID, Order: order})
			return nil
		}
	}
	return fmt.Errorf("node: set placement: no node: %w", asynxModels.ErrValidation)
}

// Forget purges id's row and records the call even when no row matched: the real store
// does not error on an unknown id either.
func (s *NodePlacements) Forget(
	ctx context.Context,
	id string,
) error {
	if s.ForgetErr != nil {
		return s.ForgetErr
	}
	s.Forgotten = append(s.Forgotten, id)
	s.Rows = without(s.Rows, id, func(n tree.Node) string { return n.ID })
	return nil
}
