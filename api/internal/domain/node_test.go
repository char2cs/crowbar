package domain_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestNode_ZeroValueIsRootedAtIndexZero(t *testing.T) {
	var n domain.Node
	if n.ParentID != "" {
		t.Fatalf("zero-value Node must have no parent")
	}
	if n.Order != 0 {
		t.Fatalf("zero-value Node must have Order 0")
	}
	if n.Kind != "" {
		t.Fatalf("zero-value Node must have no kind")
	}
}

func TestNode_HasKind(t *testing.T) {
	n := domain.Node{Kind: domain.NodeKindFolder}
	if n.Kind != domain.NodeKindFolder {
		t.Fatalf("Kind not settable/readable")
	}
}

func TestNodeKind_ClosedTaxonomy(t *testing.T) {
	want := []domain.NodeKind{
		domain.NodeKindChat,
		domain.NodeKindFolder,
		domain.NodeKindWorkspace,
		domain.NodeKindRepo,
	}
	seen := map[domain.NodeKind]bool{}
	for _, k := range want {
		if k == "" {
			t.Fatalf("node kind constant is empty")
		}
		if seen[k] {
			t.Fatalf("node kind constant %q is duplicated", k)
		}
		seen[k] = true
	}
}
