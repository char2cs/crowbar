package paths

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// All *At variants accept an explicit homeDir so they never touch the real ~/.crowbar.

func TestEventsAt_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := EventsAt(dir)
	if err != nil {
		t.Fatalf("EventsAt(%q) error: %v", dir, err)
	}
	want := filepath.Join(dir, ".crowbar", "state", "events")
	if got != want {
		t.Errorf("EventsAt(%q) = %q; want %q", dir, got, want)
	}
	if _, err := os.Stat(got); os.IsNotExist(err) {
		t.Errorf("EventsAt: directory %q was not created", got)
	}
}

func TestEventsAt_ExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	// Call twice — second call should succeed even though directory exists.
	first, err := EventsAt(dir)
	if err != nil {
		t.Fatalf("first EventsAt: %v", err)
	}
	second, err := EventsAt(dir)
	if err != nil {
		t.Fatalf("second EventsAt: %v", err)
	}
	if first != second {
		t.Errorf("repeated EventsAt returned different paths: %q vs %q", first, second)
	}
}

func TestStoreAt_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := StoreAt(dir)
	if err != nil {
		t.Fatalf("StoreAt(%q) error: %v", dir, err)
	}
	want := filepath.Join(dir, ".crowbar", "state", "store")
	if got != want {
		t.Errorf("StoreAt(%q) = %q; want %q", dir, got, want)
	}
	if _, err := os.Stat(got); os.IsNotExist(err) {
		t.Errorf("StoreAt: directory %q was not created", got)
	}
}

func TestRunsAt_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := RunsAt(dir)
	if err != nil {
		t.Fatalf("RunsAt(%q) error: %v", dir, err)
	}
	want := filepath.Join(dir, ".crowbar", "runs")
	if got != want {
		t.Errorf("RunsAt(%q) = %q; want %q", dir, got, want)
	}
	if _, err := os.Stat(got); os.IsNotExist(err) {
		t.Errorf("RunsAt: directory %q was not created", got)
	}
}

func TestLogsAt_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := LogsAt(dir)
	if err != nil {
		t.Fatalf("LogsAt(%q) error: %v", dir, err)
	}
	want := filepath.Join(dir, ".crowbar", "logs")
	if got != want {
		t.Errorf("LogsAt(%q) = %q; want %q", dir, got, want)
	}
	if _, err := os.Stat(got); os.IsNotExist(err) {
		t.Errorf("LogsAt: directory %q was not created", got)
	}
}

// --- Non-At variants (use process home; just verify non-empty return + dir exists) ---

func TestEvents_ReturnsNonEmptyAndCreates(t *testing.T) {
	got, err := Events()
	if err != nil {
		t.Fatalf("Events() error: %v", err)
	}
	if got == "" {
		t.Error("Events() returned empty path")
	}
	if _, err := os.Stat(got); os.IsNotExist(err) {
		t.Errorf("Events(): directory %q was not created", got)
	}
}

func TestStore_ReturnsNonEmptyAndCreates(t *testing.T) {
	got, err := Store()
	if err != nil {
		t.Fatalf("Store() error: %v", err)
	}
	if got == "" {
		t.Error("Store() returned empty path")
	}
	if _, err := os.Stat(got); os.IsNotExist(err) {
		t.Errorf("Store(): directory %q was not created", got)
	}
}

func TestRuns_ReturnsNonEmptyAndCreates(t *testing.T) {
	got, err := Runs()
	if err != nil {
		t.Fatalf("Runs() error: %v", err)
	}
	if got == "" {
		t.Error("Runs() returned empty path")
	}
	if _, err := os.Stat(got); os.IsNotExist(err) {
		t.Errorf("Runs(): directory %q was not created", got)
	}
}

func TestLogs_ReturnsNonEmptyAndCreates(t *testing.T) {
	got, err := Logs()
	if err != nil {
		t.Fatalf("Logs() error: %v", err)
	}
	if got == "" {
		t.Error("Logs() returned empty path")
	}
	if _, err := os.Stat(got); os.IsNotExist(err) {
		t.Errorf("Logs(): directory %q was not created", got)
	}
}

// --- Ensure error path: path is a file, not a directory ---

func TestEnsure_ErrorWhenPathIsFile(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file at the path that ensure() will try to mkdir.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// ensure() tries MkdirAll on a path whose parent is a file; that will fail.
	blocked := filepath.Join(blocker, "child")
	_, err := ensure(blocked)
	if err == nil {
		t.Error("ensure() should have returned an error when path is inside a file")
	}
}

// --- Concurrent calls for the same path must not race ---

func TestEventsAt_ConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	const goroutines = 30
	results := make([]string, goroutines)
	errors := make([]error, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			results[i], errors[i] = EventsAt(dir)
		}()
	}
	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Errorf("goroutine %d: EventsAt error: %v", i, err)
		}
	}
	first := results[0]
	for i, r := range results {
		if r != first {
			t.Errorf("goroutine %d: result %q != %q", i, r, first)
		}
	}
}

func TestStoreAt_ConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	const goroutines = 30
	results := make([]string, goroutines)
	errors := make([]error, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			results[i], errors[i] = StoreAt(dir)
		}()
	}
	wg.Wait()
	for i, err := range errors {
		if err != nil {
			t.Errorf("goroutine %d: StoreAt error: %v", i, err)
		}
	}
}

func TestRunsAt_ConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	const goroutines = 30
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			_, errs[i] = RunsAt(dir)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: RunsAt error: %v", i, err)
		}
	}
}

func TestLogsAt_ConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	const goroutines = 30
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			_, errs[i] = LogsAt(dir)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: LogsAt error: %v", i, err)
		}
	}
}

// Verify each At function returns a distinct subdirectory.
func TestAt_DistinctPaths(t *testing.T) {
	dir := t.TempDir()
	events, err := EventsAt(dir)
	if err != nil {
		t.Fatalf("EventsAt: %v", err)
	}
	store, err := StoreAt(dir)
	if err != nil {
		t.Fatalf("StoreAt: %v", err)
	}
	runs, err := RunsAt(dir)
	if err != nil {
		t.Fatalf("RunsAt: %v", err)
	}
	logs, err := LogsAt(dir)
	if err != nil {
		t.Fatalf("LogsAt: %v", err)
	}

	paths := []string{events, store, runs, logs}
	seen := map[string]bool{}
	for _, p := range paths {
		if seen[p] {
			t.Errorf("duplicate path returned: %q", p)
		}
		seen[p] = true
	}
}
