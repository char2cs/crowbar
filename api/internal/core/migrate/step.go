package migrate

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

type Phase uint8

const (
	PreOpen Phase = iota + 1
	PostOpen
)

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

type ID struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

type Env struct {
	Home    home.Home
	Log     *slog.Logger
	Earlier Marks
	Notes   map[string]Note
	History History
}

type Note struct {
	Detail string
	Counts Counts
}

type Marks map[string]Counts

func (m Marks) Done(
	key string,
) bool {
	_, ok := m[key]
	return ok
}

type Counts map[string]int64

func (c Counts) Add(
	o Counts,
) {
	for label, n := range o {
		c[label] += n
	}
}

type Step struct {
	Number int
	Name   string
	Phase  Phase
	From   int
	To     int
	Plan   func(ctx context.Context, env Env) (Plan, error)
	Recurs bool
}

func (s Step) ID() ID {
	return ID{Number: s.Number, Name: s.Name}
}

func (s Step) bumps() bool {
	return s.To == s.From+1
}
