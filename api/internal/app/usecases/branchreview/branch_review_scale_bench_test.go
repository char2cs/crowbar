package branchreview_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/perf"
)

// Scale benchmarks for the branch-review hot path, backing the attribution
// phase of docs/superpowers/specs/2026-07-27-diff-subsystem-at-scale-design.md.
//
// GetFiles is the single hottest endpoint in the system: an always-mounted
// sidebar hook calls it on every git-status tick (250ms debounce, so up to
// ~4Hz during agent activity) with the Branch Review pane CLOSED. The spec's
// third invariant says no recurring-timer request may be O(diff size). These
// benchmarks are how that claim is checked rather than asserted.
//
// The diff shape mirrors scripts/perf/gen-big-diff-fixture.sh: scattered files
// (every Nth line changed, so hunks are many and separated by context), dense
// whole-file rewrites, and one monster file. A uniform rewrite would collapse
// each file to a single hunk and quietly understate hunk-shaped work.
//
// Run with:
//
//	go test -tags noEmbed -run='^$' -bench=Scale -benchtime=5x \
//	    ./internal/app/usecases/branchreview/...

// scatterEvery matches the fixture generator: 10 lines apart, chosen against
// git's default 3 lines of context so consecutive changes stay separate hunks
// instead of merging into one.
const scatterEvery = 10

type scaleSpec struct {
	name           string
	scatteredFiles int
	denseFiles     int
	lines          int
	monsterLines   int
}

// scaleLine is a pure function of (salt, index) — no randomness, so a rerun
// produces byte-identical content and benchmark numbers stay comparable.
func scaleLine(salt string, i int) string {
	return fmt.Sprintf("%s line %d token %d\n", salt, i, (i*2654435761+len(salt))%1000003)
}

func scaleText(salt string, lines int) string {
	var b strings.Builder
	b.Grow(lines * 40)
	for i := 1; i <= lines; i++ {
		b.WriteString(scaleLine(salt, i))
	}
	return b.String()
}

func scaleTextScattered(baseSalt, headSalt string, lines, every int) string {
	var b strings.Builder
	b.Grow(lines * 40)
	for i := 1; i <= lines; i++ {
		salt := baseSalt
		if i%every == 0 {
			salt = headSalt
		}
		b.WriteString(scaleLine(salt, i))
	}
	return b.String()
}

func writeScaleFile(
	b *testing.B,
	root string,
	rel string,
	content string,
) {
	b.Helper()
	full := filepath.Join(root, rel)
	require.NoError(b, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(b, os.WriteFile(full, []byte(content), 0o600))
}

// seedScaleDiff writes the base content on the base branch and the modified
// content on the feature worktree, committing both, so the resulting
// base..feature diff is a genuine content diff rather than an add-only one.
func seedScaleDiff(
	b *testing.B,
	h *benchReviewHarness,
	spec scaleSpec,
) {
	b.Helper()

	for i := range spec.scatteredFiles {
		rel := fmt.Sprintf("src/scattered/pkg%d/file%d.ts", i%20, i)
		writeScaleFile(b, h.repoPath, rel, scaleText(fmt.Sprintf("base%d", i), spec.lines))
	}
	for i := range spec.denseFiles {
		rel := fmt.Sprintf("src/dense/pkg%d/file%d.ts", i%10, i)
		writeScaleFile(b, h.repoPath, rel, scaleText(fmt.Sprintf("base%d", i), spec.lines))
	}
	benchGitRun(b, h.repoPath, "add", "-A")
	benchGitRun(b, h.repoPath, "commit", "-m", "scale: base tree")

	// The feature worktree must see the new base commits before it can modify
	// them, so rebase it onto the updated base branch.
	baseBranch := trimNewline(benchGitRun(b, h.repoPath, "rev-parse", "--abbrev-ref", "HEAD"))
	benchGitRun(b, h.featurePath, "rebase", baseBranch)

	for i := range spec.scatteredFiles {
		rel := fmt.Sprintf("src/scattered/pkg%d/file%d.ts", i%20, i)
		writeScaleFile(b, h.featurePath, rel, scaleTextScattered(
			fmt.Sprintf("base%d", i), fmt.Sprintf("head%d", i), spec.lines, scatterEvery))
	}
	for i := range spec.denseFiles {
		rel := fmt.Sprintf("src/dense/pkg%d/file%d.ts", i%10, i)
		writeScaleFile(b, h.featurePath, rel, scaleText(fmt.Sprintf("head%d", i), spec.lines))
	}
	if spec.monsterLines > 0 {
		writeScaleFile(b, h.featurePath, "src/huge/monster.ts",
			scaleText("huge", spec.monsterLines))
	}
	benchGitRun(b, h.featurePath, "add", "-A")
	benchGitRun(b, h.featurePath, "commit", "-m", "scale: feature tree")
}

// reportPerfSamples aggregates the daemon timing ring by sample name and logs
// it. This is the breakdown attribution needs: ns/op alone says the call is
// slow, not which subprocess or lock inside it is responsible.
func reportPerfSamples(b *testing.B) {
	b.Helper()

	type agg struct {
		n     int
		total float64
		max   float64
	}
	byName := map[string]*agg{}
	for _, s := range perf.Snapshot() {
		a, ok := byName[s.Name]
		if !ok {
			a = &agg{}
			byName[s.Name] = a
		}
		a.n++
		a.total += s.DurationMS
		if s.DurationMS > a.max {
			a.max = s.DurationMS
		}
	}
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		return byName[names[i]].total > byName[names[j]].total
	})
	for _, n := range names {
		a := byName[n]
		b.Logf("  %-24s calls=%-4d total=%8.1fms mean=%7.1fms max=%7.1fms",
			n, a.n, a.total, a.total/float64(a.n), a.max)
	}
}

var scaleSpecs = []scaleSpec{
	// Roughly the shape of a large real branch, sized so setup stays tolerable.
	{name: "200files_100k_lines", scatteredFiles: 150, denseFiles: 50, lines: 500, monsterLines: 50_000},
}

// BenchmarkBranchReview_GetFiles_Scale measures the always-on hot path — the
// one a git tick hits with the review pane closed.
func BenchmarkBranchReview_GetFiles_Scale(b *testing.B) {
	for _, spec := range scaleSpecs {
		b.Run(spec.name, func(b *testing.B) {
			ctx := context.Background()
			h := newBenchReviewHarness(b)
			seedScaleDiff(b, h, spec)

			// Warm: the first call pays page-cache costs that would otherwise
			// land entirely on the first measured iteration.
			_, err := h.uc.GetFiles(ctx, h.wsID)
			require.NoError(b, err)

			perf.Reset()
			perf.SetEnabled(true)
			b.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

			b.ResetTimer()
			for range b.N {
				files, err := h.uc.GetFiles(ctx, h.wsID)
				if err != nil {
					b.Fatalf("GetFiles: %v", err)
				}
				if len(files) == 0 {
					b.Fatal("unexpected empty file summary")
				}
			}
			b.StopTimer()

			reportPerfSamples(b)
		})
	}
}

// BenchmarkBranchReview_GetFiles_Contention measures lock hold and wait times
// with concurrent readers and one engine-level writer.
//
// It was written to demonstrate writer starvation: lockRepoRead takes a
// sync.RWMutex read lock, and Go's RWMutex is writer-preferring, so once a
// Lock() is waiting, new RLock() callers block until the writer has come and
// gone. On a repo where reads hold the lock for hundreds of milliseconds, one
// writer should therefore delay every read queued behind it.
//
// It does NOT demonstrate that. As of 2026-07-27, at 200 files / 100k lines
// with 16 concurrent readers, both lock.read.wait and lock.write.wait measure
// 0.0ms across every run. The writer (StageFile, ~30-70ms) consistently
// acquires and releases before the readers reach their own acquisition —
// GetFiles resolves the diff ref first, so readers spend their first ~25ms in
// merge-base before ever asking for the lock.
//
// The honest reading: writer starvation remains a documented property of
// sync.RWMutex, not something measured here. What IS measured, and what
// matters, is lock.read.hold — mean ~180-230ms and max ~520ms at a tenth of
// target scale, against a 100ms budget. Long holds are the precondition for
// starvation; this harness simply never won the race needed to observe it.
//
// Forcing the overlap would need either a sleep or a hook inside the lock
// itself. The former is banned in this repo and the latter would measure the
// instrument rather than the system, so the gap is left documented instead of
// papered over.
//
// This is a measurement, not an assertion — it reports numbers and makes no
// timing-based claim that could flake.
func BenchmarkBranchReview_GetFiles_Contention(b *testing.B) {
	const readers = 16

	for _, spec := range scaleSpecs {
		b.Run(spec.name, func(b *testing.B) {
			ctx := context.Background()
			h := newBenchReviewHarness(b)
			seedScaleDiff(b, h, spec)

			_, err := h.uc.GetFiles(ctx, h.wsID)
			require.NoError(b, err)

			perf.Reset()
			perf.SetEnabled(true)
			b.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

			b.ResetTimer()
			for i := range b.N {
				var wg sync.WaitGroup
				// Concurrent readers, exactly as the sidebar's tick storm
				// produces them.
				for range readers {
					wg.Add(1)
					go func() {
						defer wg.Done()
						if _, err := h.uc.GetFiles(ctx, h.wsID); err != nil {
							b.Errorf("GetFiles: %v", err)
						}
					}()
				}
				// One writer per round, interleaved with the readers. It must go
				// through the ENGINE (StageFile takes lockRepo) — shelling out
				// to git directly would bypass the mutex entirely and measure
				// nothing, which is exactly what an earlier version of this
				// benchmark did.
				wg.Add(1)
				go func(round int) {
					defer wg.Done()
					rel := fmt.Sprintf("src/churn/round%d.ts", round)
					writeScaleFile(b, h.featurePath, rel, "churn\n")
					if err := h.engine.StageFile(ctx, h.featurePath, rel); err != nil {
						b.Errorf("StageFile: %v", err)
					}
				}(i)
				wg.Wait()
			}
			b.StopTimer()

			reportPerfSamples(b)
		})
	}
}

// BenchmarkBranchReview_Get_Scale measures the full line-level review read
// model at the same scale — the payload the spec replaces with a windowed
// path. Contrast with GetFiles above: the gap between them is the cost the
// sidebar currently avoids only because the review pane gates it.
func BenchmarkBranchReview_Get_Scale(b *testing.B) {
	for _, spec := range scaleSpecs {
		b.Run(spec.name, func(b *testing.B) {
			ctx := context.Background()
			h := newBenchReviewHarness(b)
			seedScaleDiff(b, h, spec)

			_, err := h.uc.Get(ctx, h.wsID)
			require.NoError(b, err)

			perf.Reset()
			perf.SetEnabled(true)
			b.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

			b.ResetTimer()
			for range b.N {
				review, err := h.uc.Get(ctx, h.wsID)
				if err != nil {
					b.Fatalf("Get: %v", err)
				}
				_ = review
			}
			b.StopTimer()

			reportPerfSamples(b)
		})
	}
}
