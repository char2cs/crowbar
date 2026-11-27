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

const scatterEvery = 10

type scaleSpec struct {
	name           string
	scatteredFiles int
	denseFiles     int
	lines          int
	monsterLines   int
}

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

	baseBranch := trimNewline(benchGitRun(b, h.repoPath, "rev-parse", "--abbrev-ref", "HEAD"))
	benchGitRun(b, h.featurePath, "rebase", baseBranch)

	for i := range spec.scatteredFiles {
		rel := fmt.Sprintf("src/scattered/pkg%d/file%d.ts", i%20, i)
		writeScaleFile(b, h.featurePath, rel, scaleTextScattered(
			fmt.Sprintf("base%d", i), fmt.Sprintf("head%d", i), spec.lines, scatterEvery,
		))
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
	{name: "200files_100k_lines", scatteredFiles: 150, denseFiles: 50, lines: 500, monsterLines: 50_000},
}

func BenchmarkBranchReview_GetFiles_Scale(b *testing.B) {
	for _, spec := range scaleSpecs {
		b.Run(spec.name, func(b *testing.B) {
			ctx := context.Background()
			h := newBenchReviewHarness(b)
			seedScaleDiff(b, h, spec)

			_, err := h.uc.GetFiles(ctx, h.wsID, "")
			require.NoError(b, err)

			perf.Reset()
			perf.SetEnabled(true)
			b.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

			b.ResetTimer()
			for range b.N {
				files, err := h.uc.GetFiles(ctx, h.wsID, "")
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

func BenchmarkBranchReview_GetFiles_Contention(b *testing.B) {
	const readers = 16

	for _, spec := range scaleSpecs {
		b.Run(spec.name, func(b *testing.B) {
			ctx := context.Background()
			h := newBenchReviewHarness(b)
			seedScaleDiff(b, h, spec)

			_, err := h.uc.GetFiles(ctx, h.wsID, "")
			require.NoError(b, err)

			perf.Reset()
			perf.SetEnabled(true)
			b.Cleanup(func() { perf.SetEnabled(false); perf.Reset() })

			b.ResetTimer()
			for i := range b.N {
				var wg sync.WaitGroup
				for range readers {
					wg.Add(1)
					go func() {
						defer wg.Done()
						if _, err := h.uc.GetFiles(ctx, h.wsID, ""); err != nil {
							b.Errorf("GetFiles: %v", err)
						}
					}()
				}
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
