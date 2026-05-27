package domain_test

import (
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestProjectDefaults(t *testing.T) {
	p := domain.Project{ID: "proj-1", Name: "test"}
	if p.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if p.CreatedAt.IsZero() {
		_ = time.Now()
	}
}

func TestWorkspacePhase(t *testing.T) {
	w := domain.Workspace{Phase: domain.PhaseBrainstorm}
	if w.Phase != domain.PhaseBrainstorm {
		t.Fatalf("expected PhaseBrainstorm, got %v", w.Phase)
	}
}
