package migrate

import (
	"context"
	"maps"
	"time"
)

// Checkpoints lets a step's Apply record each finished unit of work under a key,
// so a run that stops part-way resumes without redoing it. It spans attempts:
// keys marked in earlier runs read as done. It is not safe for concurrent use.
type Checkpoints struct {
	ledger *ledger
	run    string
	step   ID
	now    func() time.Time
	done   map[string]Counts
}

func newCheckpoints(
	l *ledger,
	run string,
	step ID,
	now func() time.Time,
) *Checkpoints {
	return &Checkpoints{
		ledger: l,
		run:    run,
		step:   step,
		now:    now,
		done:   l.view.checkpoints(step.Number),
	}
}

// Done reports whether key was marked by this attempt or an earlier one.
func (c *Checkpoints) Done(
	key string,
) bool {
	_, ok := c.done[key]
	return ok
}

// Mark durably records that the unit of work under key is finished, with the
// counts it contributed. The key reads as done only once the record is synced.
func (c *Checkpoints) Mark(
	key string,
	n Counts,
) error {
	err := c.ledger.append(record{
		Kind:   kindCheckpoint,
		At:     c.now(),
		Run:    c.run,
		Step:   c.step.Number,
		Name:   c.step.Name,
		Key:    key,
		Counts: maps.Clone(n),
	})
	if err != nil {
		return err
	}
	if c.done[key] == nil {
		c.done[key] = Counts{}
	}
	c.done[key].Add(n)
	return nil
}

func (c *Checkpoints) total() Counts {
	sum := Counts{}
	for _, n := range c.done {
		sum.Add(n)
	}
	return sum
}

// Impl is the online half of a Step, paired with it by number at run time.
type Impl struct {
	// Apply does the work and may be re-run after any interruption, so it must
	// be idempotent. It returns ctx.Err() promptly once ctx is cancelled.
	Apply func(ctx context.Context, env Env, cp *Checkpoints) (Counts, error)
	// Verify checks the step's postconditions; an error halts the boot.
	Verify func(ctx context.Context, env Env) error
}

// Impls maps a Step.Number to its Impl.
type Impls map[int]Impl
