// Package migratetest holds test doubles and ledger readers for the migration
// engine. It is apart from testutil because the engine's own tests use that.
package migratetest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

// NoPostOpen is the post-open runner of an app a test builds without a
// migration session: it runs none of the steps it is handed.
func NoPostOpen() migrate.PostOpenRunner {
	return noPostOpen{}
}

type noPostOpen struct{}

func (noPostOpen) RunPostOpen(
	context.Context,
	migrate.Impls,
) error {
	return nil
}

// Applied returns, by step number, the counts of every step.done record in
// h's ledger in the order they were written; none when there is no ledger.
func Applied(
	tb testing.TB,
	h home.Home,
) map[int][]migrate.Counts {
	tb.Helper()
	raw, err := os.ReadFile(filepath.Join(h.MigrateDir(), "ledger.jsonl"))
	if errors.Is(err, fs.ErrNotExist) {
		return map[int][]migrate.Counts{}
	}
	if err != nil {
		tb.Fatalf("migratetest: read ledger: %v", err)
	}
	out := map[int][]migrate.Counts{}
	lines := bufio.NewScanner(bytes.NewReader(raw))
	for lines.Scan() {
		var r struct {
			Kind   string         `json:"kind"`
			Step   int            `json:"step"`
			Counts migrate.Counts `json:"counts"`
		}
		if err := json.Unmarshal(lines.Bytes(), &r); err != nil {
			tb.Fatalf("migratetest: ledger line %q: %v", lines.Text(), err)
		}
		if r.Kind == "step.done" {
			out[r.Step] = append(out[r.Step], r.Counts)
		}
	}
	if err := lines.Err(); err != nil {
		tb.Fatalf("migratetest: scan ledger: %v", err)
	}
	return out
}
