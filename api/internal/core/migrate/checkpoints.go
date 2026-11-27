package migrate

import (
	"context"
	"maps"
	"time"
)

type Checkpoints struct {
	ledger  *ledger
	run     string
	step    ID
	now     func() time.Time
	done    Marks
	notes   map[string]Note
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

func (c *Checkpoints) Marked(
	key string,
) (Counts, bool) {
	n, ok := c.done[key]
	return maps.Clone(n), ok
}

func (c *Checkpoints) Settled(
	key string,
) bool {
	return c.settled[key]
}

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

func (c *Checkpoints) Noted(
	key string,
) (Note, bool) {
	n, ok := c.notes[key]
	return n, ok
}

func (c *Checkpoints) Done(
	key string,
) bool {
	return c.done.Done(key)
}

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

type Impl struct {
	Apply  func(ctx context.Context, env Env, cp *Checkpoints) (Counts, error)
	Verify func(ctx context.Context, env Env) error
}

type Impls map[int]Impl

func (i Impls) implements(
	number int,
) bool {
	return i[number].Apply != nil
}
