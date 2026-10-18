// Package migrate is the migration engine: a validated catalog of steps that
// move a Crowbar home from one on-disk layout to the next. It knows no store
// and no domain; steps bring that knowledge.
package migrate

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// Phase says when in the boot a step runs relative to opening the databases.
type Phase uint8

const (
	// PreOpen steps run before any database opens and are the only ones that
	// may change the layout version.
	PreOpen Phase = iota + 1
	// PostOpen steps run once the databases are open, at the current layout.
	PostOpen
)

// String names the phase as it appears in plans and ledger records.
func (p Phase) String() string {
	switch p {
	case PreOpen:
		return "pre-open"
	case PostOpen:
		return "post-open"
	default:
		return fmt.Sprintf("phase(%d)", uint8(p))
	}
}

// ID identifies a step in ledger records and plans.
type ID struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

// Env is what a step may touch: the home it migrates and a logger, and what the
// step's earlier attempts checkpointed.
type Env struct {
	Home home.Home
	Log  *slog.Logger
	// Earlier lets a Plan, which has no Checkpoints, see how far an attempt that
	// was stopped got.
	Earlier Marks
}

// Marks are the keys a step checkpointed, with the counts marked under each.
type Marks map[string]Counts

// Done reports whether key was checkpointed.
func (m Marks) Done(
	key string,
) bool {
	_, ok := m[key]
	return ok
}

// Counts tallies what a step did, by a step-chosen label.
type Counts map[string]int64

// Add sums o into c. c must be non-nil.
func (c Counts) Add(
	o Counts,
) {
	for label, n := range o {
		c[label] += n
	}
}

// Step is the offline half of a migration: its identity, where it sits in the
// layout chain, and a read-only plan. A bump step has To == From+1; an
// in-layout step has To == From.
type Step struct {
	Number int
	Name   string
	Phase  Phase
	From   int
	To     int
	// Plan runs without opening any store and must not write.
	Plan func(ctx context.Context, env Env) (Plan, error)
}

// ID returns the step's number and name.
func (s Step) ID() ID {
	return ID{Number: s.Number, Name: s.Name}
}

func (s Step) bumps() bool {
	return s.To == s.From+1
}
