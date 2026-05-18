package engine_test

import (
	"context"
	"testing"

	"github.com/rabbytesoftware/crowbar/api/internal/engine"
)

func TestEngineContainerNew(t *testing.T) {
	ctx := context.Background()
	c, err := engine.New(ctx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil container")
	}
}
