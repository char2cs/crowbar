package commands

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// The user's lock override, and the one thing that makes it worth having: it has
// to survive the provider poll that runs a minute later.
//
// Before it existed, `locked` was the provider's word alone — nextProviderStatus
// re-derived it from the protected flag on every sync. A user who unlocked main
// would find it locked again on the next tick, so the tests below are written
// around a SYNC following the toggle, not around the toggle by itself.

func ptr(b bool) *bool { return &b }

func TestSetLockUnlocksAProtectedBranch(t *testing.T) {
	ws := &domain.Workspace{ID: "w1", WorktreePath: "/w", Status: domain.WorkspaceStatusLocked}

	got := SetLock{ID: "w1", Locked: ptr(false), Protected: true}.EmitEvent(ws)

	if got.Status != domain.WorkspaceStatusNew {
		t.Errorf("status = %q, want new — unlocking has to put something in place of locked", got.Status)
	}
	if got.LockOverride == nil || *got.LockOverride {
		t.Error("the override must be persisted as false, not left nil")
	}
}

func TestSetLockLocksAnOrdinaryBranch(t *testing.T) {
	// Any branch, including a fork child the provider has no opinion about.
	ws := &domain.Workspace{ID: "w1", WorktreePath: "/w", ParentID: "w0", Status: domain.WorkspaceStatusNew}

	got := SetLock{ID: "w1", Locked: ptr(true), Protected: false}.EmitEvent(ws)

	if got.Status != domain.WorkspaceStatusLocked {
		t.Errorf("status = %q, want locked", got.Status)
	}
}

func TestSetLockClearedHandsTheQuestionBackToTheProvider(t *testing.T) {
	ws := &domain.Workspace{ID: "w1", WorktreePath: "/w", Status: domain.WorkspaceStatusNew, LockOverride: ptr(false)}

	got := SetLock{ID: "w1", Locked: nil, Protected: true}.EmitEvent(ws)

	if got.LockOverride != nil {
		t.Error("clearing the override must leave nil, the no-opinion state")
	}
	if got.Status != domain.WorkspaceStatusLocked {
		t.Errorf("status = %q, want locked — the provider protects this branch", got.Status)
	}
}

func TestUnlockSurvivesTheNextProviderPoll(t *testing.T) {
	// The regression this whole feature turns on.
	unlocked := ptr(false)

	got := nextProviderStatus(domain.WorkspaceStatusNew, unlocked, true, false, "")

	if got == domain.WorkspaceStatusLocked {
		t.Fatal("the provider re-locked a branch the user deliberately unlocked")
	}
}

func TestLockSurvivesAnIncomingPRStatus(t *testing.T) {
	// The other direction: a branch the user locked must not be quietly unlocked
	// by a PR opening on it.
	locked := ptr(true)

	got := nextProviderStatus(domain.WorkspaceStatusLocked, locked, false, true, "open")

	if got != domain.WorkspaceStatusLocked {
		t.Fatalf("status = %q, want locked", got)
	}
}

func TestProviderStillLocksWithoutAnOverride(t *testing.T) {
	// Automatic locking is untouched: no opinion means the provider decides,
	// exactly as it always has.
	got := nextProviderStatus(domain.WorkspaceStatusNew, nil, true, false, "")

	if got != domain.WorkspaceStatusLocked {
		t.Fatalf("status = %q, want locked", got)
	}
}

func TestSetLockRefusesWhatCannotBeLocked(t *testing.T) {
	cases := map[string]*domain.Workspace{
		"home workspace":               {ID: "w1", Kind: domain.WorkspaceKindHome, WorktreePath: "/w"},
		"placeholder with no worktree": {ID: "w1", Status: domain.WorkspaceStatusLocked},
	}
	for name, ws := range cases {
		t.Run(name, func(t *testing.T) {
			if err := (SetLock{ID: "w1", Locked: ptr(false)}).Validate(ws); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
}

func TestSetLockNeverResurrectsADeletedRow(t *testing.T) {
	ws := &domain.Workspace{ID: "w1", WorktreePath: "/w", Status: domain.WorkspaceStatusDeleted}

	got := SetLock{ID: "w1", Locked: ptr(true), Protected: false}.EmitEvent(ws)

	if got.Status != domain.WorkspaceStatusDeleted {
		t.Fatalf("status = %q, want deleted to stay terminal", got.Status)
	}
}
