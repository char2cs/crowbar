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
