package tree

import (
	"context"
	"errors"
	"testing"
)

type busyIDs map[string]bool

func (b busyIDs) ChatBusy(_ context.Context, chatID string) (bool, error) { return b[chatID], nil }

type busyFails struct{}

func (busyFails) ChatBusy(context.Context, string) (bool, error) {
	return false, errors.New("ledger down")
}

func TestGuardNotWorking_RefusesWhenAnyRowWorking(t *testing.T) {
	err := guardNotWorking(context.Background(), []string{"root", "child-1", "child-2"}, busyIDs{"child-2": true})
	if !errors.Is(err, ErrSubtreeWorking) {
		t.Fatalf("want ErrSubtreeWorking, got %v", err)
	}
}

func TestGuardNotWorking_AllowsWhenSubtreeIdle(t *testing.T) {
	err := guardNotWorking(context.Background(), []string{"root", "child-1"}, busyIDs{})
	if err != nil {
		t.Fatalf("unexpected error on an idle subtree: %v", err)
	}
}

func TestGuardNotWorking_RefusesWhenBusyCannotBeRead(t *testing.T) {
	err := guardNotWorking(context.Background(), []string{"root"}, busyFails{})
	if err == nil || errors.Is(err, ErrSubtreeWorking) {
		t.Fatalf("want the read's error, got %v", err)
	}
}
