package mocks

import (
	"context"
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type NodePlacements struct {
	Rows          []tree.Node
	CreateErr     error
	GetErr        error
	ListErr       error
	OrderErr      error
	PlaceErr      error
	OrderErrForID map[string]error
	PlaceErrForID map[string]error
	Placed        []NodePlacementWrite
	Ordered       []NodeOrderWrite
	Forgotten     []string
	ForgetErr     error
}

type NodePlacementWrite struct {
	ID       string
	ParentID string
	Order    int
}

type NodeOrderWrite struct {
	ID    string
	Order int
}

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
