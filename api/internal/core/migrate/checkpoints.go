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
	done   Marks
	notes  map[string]Note
	// settled are the keys marked before the step last completed.
	settled map[string]bool
}

func newCheckpoints(
	l *ledger,
	run string,
	step ID,
	now func() time.Time,
) *Checkpoints {
	return &Checkpoints{
		ledger:  l,
		run:     run,
		step:    step,
		now:     now,
		done:    l.view.checkpoints(step.Number, step.Name),
		notes:   l.view.notes(step.Number, step.Name),
		settled: l.view.settled(step.Number, step.Name),
	}
}

// Marked is the counts marked under key, when it is done.
func (c *Checkpoints) Marked(
	key string,
) (Counts, bool) {
	n, ok := c.done[key]
	return maps.Clone(n), ok
}

// Settled reports whether key was marked before the step last completed, so
// that completion's Verify already checked it.
func (c *Checkpoints) Settled(
	key string,
) bool {
	return c.settled[key]
}

// Note durably records n under key without marking key done; Noted reads
// back the latest, in this attempt or an earlier one.
func (c *Checkpoints) Note(
	key string,
	n Note,
) error {
	err := c.ledger.append(record{
		Kind:   kindNote,
		At:     c.now(),
		Run:    c.run,
		Step:   c.step.Number,
		Name:   c.step.Name,
		Key:    key,
		Counts: n.Counts,
		Detail: n.Detail,
	})
	if err != nil {
		return err
	}
	c.notes[key] = Note{Detail: n.Detail, Counts: maps.Clone(n.Counts)}
	return nil
}

// Noted is the latest note under key.
func (c *Checkpoints) Noted(
	key string,
) (Note, bool) {
	n, ok := c.notes[key]
	return n, ok
}

// Done reports whether key was marked by this attempt or an earlier one.
func (c *Checkpoints) Done(
	key string,
) bool {
	return c.done.Done(key)
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
		Counts: n,
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

func (i Impls) implements(
	number int,
) bool {
	return i[number].Apply != nil
}
