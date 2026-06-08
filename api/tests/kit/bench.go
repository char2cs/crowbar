//go:build integration

package kit

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// BenchmarkResult holds the p50 and p99 latency percentiles for a named run.
type BenchmarkResult struct {
	Name string
	P50  time.Duration
	P99  time.Duration
}

type benchBaseline struct {
	Version int                         `json:"version"`
	Results map[string]benchBaselineRow `json:"results"`
}

type benchBaselineRow struct {
	P50NS int64 `json:"p50ns"`
	P99NS int64 `json:"p99ns"`
}

// RunBenchmark runs fn n times, records each duration, and returns p50/p99.
func RunBenchmark(
	t testing.TB,
	name string,
	n int,
	fn func(),
) BenchmarkResult {
	t.Helper()
	durations := make(
		[]time.Duration,
		0,
		n,
	)
	for range n {
		start := time.Now()
		fn()
		durations = append(
			durations,
			time.Since(start),
		)
	}
	less := func(
		i int,
		j int,
	) bool {
		return durations[i] < durations[j]
	}
	sort.Slice(
		durations,
		less,
	)
	return BenchmarkResult{
		Name: name,
		P50: percentile(
			durations,
			50,
		),
		P99: percentile(
			durations,
			99,
		),
	}
}

// AssertNoRegression checks result against the saved baseline. On UPDATE_BASELINE=1
// it updates the baseline instead of asserting. The threshold is 1.25× p99.
func AssertNoRegression(
	t *testing.T,
	result BenchmarkResult,
) {
	t.Helper()
	path := baselinePath(t)

	if os.Getenv("UPDATE_BASELINE") == "1" {
		updateBaseline(
			t,
			path,
			result,
		)
		return
	}

	baseline := loadBaseline(
		t,
		path,
	)
	row, ok := baseline.Results[result.Name]
	if !ok {
		t.Logf(
			"bench: no baseline for %q; skipping regression check (run with UPDATE_BASELINE=1 to create)",
			result.Name,
		)
		return
	}

	threshold := time.Duration(float64(row.P99NS) * 1.25)
	if result.P99 > threshold {
		t.Errorf(
			"bench: %s p99 regression: baseline=%v threshold=%v actual=%v",
			result.Name,
			time.Duration(row.P99NS),
			threshold,
			result.P99,
		)
	}
}

func percentile(
	sorted []time.Duration,
	pct int,
) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := ((len(sorted) - 1) * pct) / 100
	return sorted[idx]
}

func baselinePath(
	t testing.TB,
) string {
	t.Helper()
	if p := os.Getenv("BENCH_BASELINE"); p != "" {
		return p
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Logf(
			"bench: os.Getwd failed: %v; using \".\" as working directory",
			err,
		)
		wd = "."
	}
	return filepath.Join(
		wd,
		"integration",
		"bench",
		"baseline.json",
	)
}

func loadBaseline(
	t *testing.T,
	path string,
) benchBaseline {
	t.Helper()
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return benchBaseline{
			Version: 1,
			Results: map[string]benchBaselineRow{},
		}
	}
	if err != nil {
		t.Fatalf(
			"bench: load baseline: %v",
			err,
		)
	}
	var b benchBaseline
	err = json.Unmarshal(raw, &b)
	if err != nil {
		t.Fatalf(
			"bench: parse baseline: %v",
			err,
		)
	}
	return b
}

func updateBaseline(
	t *testing.T,
	path string,
	result BenchmarkResult,
) {
	t.Helper()
	baseline := loadBaseline(
		t,
		path,
	)
	if baseline.Results == nil {
		baseline.Results = make(map[string]benchBaselineRow)
	}
	baseline.Results[result.Name] = benchBaselineRow{
		P50NS: result.P50.Nanoseconds(),
		P99NS: result.P99.Nanoseconds(),
	}
	raw, err := json.MarshalIndent(
		baseline,
		"",
		"  ",
	)
	if err != nil {
		t.Fatalf(
			"bench: marshal baseline: %v",
			err,
		)
	}
	if err := os.MkdirAll(
		filepath.Dir(path),
		0o755,
	); err != nil {
		t.Fatalf(
			"bench: mkdir baseline dir: %v",
			err,
		)
	}
	if err := os.WriteFile(
		path,
		raw,
		0o644,
	); err != nil {
		t.Fatalf(
			"bench: write baseline: %v",
			err,
		)
	}
	t.Logf(
		"bench: baseline updated for %q: p50=%v p99=%v",
		result.Name,
		result.P50,
		result.P99,
	)
}
