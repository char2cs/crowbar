package commands

import (
	"errors"
	"fmt"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func ptr(b bool) *bool { return &b }

func TestSetLockUnlocksAProtectedBranch(t *testing.T) {
	ws := &domain.Workspace{ID: "w1", WorktreePath: "/w", Status: domain.WorkspaceStatusLocked, Provisioning: domain.WorkspaceProvisioned}

	got := SetLock{ID: "w1", Locked: ptr(false), Protected: true}.EmitEvent(ws)

	if got.Status != domain.WorkspaceStatusNew {
		t.Errorf("status = %q, want new — unlocking has to put something in place of locked", got.Status)
	}
	if got.LockOverride == nil || *got.LockOverride {
		t.Error("the override must be persisted as false, not left nil")
	}
}

func TestSetLockLocksAnOrdinaryBranch(t *testing.T) {
	ws := &domain.Workspace{ID: "w1", WorktreePath: "/w", ParentID: "w0", Status: domain.WorkspaceStatusNew, Provisioning: domain.WorkspaceProvisioned}

	got := SetLock{ID: "w1", Locked: ptr(true), Protected: false}.EmitEvent(ws)

	if got.Status != domain.WorkspaceStatusLocked {
		t.Errorf("status = %q, want locked", got.Status)
	}
}

func TestSetLockClearedHandsTheQuestionBackToTheProvider(t *testing.T) {
	ws := &domain.Workspace{ID: "w1", WorktreePath: "/w", Status: domain.WorkspaceStatusNew, LockOverride: ptr(false), Provisioning: domain.WorkspaceProvisioned}

	got := SetLock{ID: "w1", Locked: nil, Protected: true}.EmitEvent(ws)

	if got.LockOverride != nil {
		t.Error("clearing the override must leave nil, the no-opinion state")
	}
	if got.Status != domain.WorkspaceStatusLocked {
		t.Errorf("status = %q, want locked — the provider protects this branch", got.Status)
	}
}

func TestUnlockSurvivesTheNextProviderPoll(t *testing.T) {
	unlocked := ptr(false)

	got := nextProviderStatus(domain.WorkspaceStatusNew, unlocked, true, false, "")

	if got == domain.WorkspaceStatusLocked {
		t.Fatal("the provider re-locked a branch the user deliberately unlocked")
	}
}

func TestLockSurvivesAnIncomingPRStatus(t *testing.T) {
	locked := ptr(true)

	got := nextProviderStatus(domain.WorkspaceStatusLocked, locked, false, true, "open")

	if got != domain.WorkspaceStatusLocked {
		t.Fatalf("status = %q, want locked", got)
	}
}

func TestProviderStillLocksWithoutAnOverride(t *testing.T) {
	got := nextProviderStatus(domain.WorkspaceStatusNew, nil, true, false, "")

	if got != domain.WorkspaceStatusLocked {
		t.Fatalf("status = %q, want locked", got)
	}
}

func TestSetLockRefusesWhatCannotBeLocked(t *testing.T) {
	cases := map[string]struct {
		ws  *domain.Workspace
		why string
	}{
		"home workspace": {
			&domain.Workspace{ID: "w1", Kind: domain.WorkspaceKindHome, WorktreePath: "/w", Provisioning: domain.WorkspaceShared},
			"The project's home checkout has no branch to lock.",
		},
		"placeholder with no worktree": {
			&domain.Workspace{ID: "w1", Status: domain.WorkspaceStatusLocked, Provisioning: domain.WorkspacePlaceholder},
			"This branch has no worktree of its own, so it can't be locked or unlocked.",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := (SetLock{ID: "w1", Locked: ptr(false)}).Validate(tc.ws)
			want := problem.Problem{Code: problem.Unprocessable, Message: tc.why}
			if got := problem.Of(fmt.Errorf("workspace: set lock: %w", err)); got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
			if !errors.Is(err, asynxModels.ErrValidation) {
				t.Fatal("still the store's validation refusal")
			}
		})
	}
}

func TestSetLockNeverResurrectsADeletedRow(t *testing.T) {
	ws := &domain.Workspace{ID: "w1", WorktreePath: "/w", Status: domain.WorkspaceStatusDeleted, Provisioning: domain.WorkspaceProvisioned}

	got := SetLock{ID: "w1", Locked: ptr(true), Protected: false}.EmitEvent(ws)

	if got.Status != domain.WorkspaceStatusDeleted {
		t.Fatalf("status = %q, want deleted to stay terminal", got.Status)
	}
}

func TestSetLockEventContract(t *testing.T) {
	c := SetLock{ID: "w1"}

	if got := c.AggregateID(); got != "w1" {
		t.Errorf("AggregateID() = %q, want the workspace id", got)
	}
	if got := c.EventName(); got != "workspace.lock_set.w1" {
		t.Errorf("EventName() = %q", got)
	}
	if !c.ShouldSnapshot() {
		t.Error("ShouldSnapshot() = false, want true")
	}
}

func TestSetLockValidateRefusesAMissingAggregate(t *testing.T) {
	if err := (SetLock{ID: "w1"}).Validate(nil); err == nil {
		t.Fatal("expected a validation error for a workspace that is not there")
	}
}
