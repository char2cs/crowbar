package tree

import (
	"errors"
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
)

func TestGuardNotWorking_RefusesWhenAnyRowWorking(t *testing.T) {
	work := inflight.NewWork()
	work.Set("child-2", true)
	err := guardNotWorking([]string{"root", "child-1", "child-2"}, work)
	if !errors.Is(err, ErrSubtreeWorking) {
		t.Fatalf("want ErrSubtreeWorking, got %v", err)
	}
}

func TestGuardNotWorking_AllowsWhenSubtreeIdle(t *testing.T) {
	work := inflight.NewWork()
	err := guardNotWorking([]string{"root", "child-1"}, work)
	if err != nil {
		t.Fatalf("unexpected error on an idle subtree: %v", err)
	}
}
