package exec_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
)

// helperMode re-enters the test binary as the child process. It is the standard
// way to get a real, controllable subprocess without shipping fixture binaries or
// depending on the shape of any system utility.
const helperMode = "CROWBAR_EXEC_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(helperMode) {
	case "":
		os.Exit(m.Run())
	case "stdout":
		fmt.Fprint(os.Stdout, os.Getenv("HELPER_PAYLOAD"))
	case "stderr":
		fmt.Fprint(os.Stderr, os.Getenv("HELPER_PAYLOAD"))
	case "flood":
		n, _ := strconv.Atoi(os.Getenv("HELPER_PAYLOAD"))
		chunk := strings.Repeat("x", 1024)
		for range n {
			fmt.Fprint(os.Stdout, chunk)
		}
	case "exit":
		code, _ := strconv.Atoi(os.Getenv("HELPER_PAYLOAD"))
		os.Exit(code)
	case "sleep":
		ms, _ := strconv.Atoi(os.Getenv("HELPER_PAYLOAD"))
		time.Sleep(time.Duration(ms) * time.Millisecond)
	case "pwd":
		wd, _ := os.Getwd()
		// macOS routes temp dirs through /private; resolve so the assertion
		// compares real paths rather than symlink spellings.
		resolved, err := filepath.EvalSymlinks(wd)
		if err == nil {
			wd = resolved
		}
		fmt.Fprint(os.Stdout, wd)
	}
	os.Exit(0)
}

func helperRunner(t *testing.T, mode, payload string, opts exec.Options) *exec.Runner {
	t.Helper()
	if opts.Executable == "" {
		opts.Executable = os.Args[0]
	}
	if opts.Cwd == "" {
		opts.Cwd = t.TempDir()
	}
	if opts.MaxStdout == 0 {
		opts.MaxStdout = 1 << 20
	}
	if opts.MaxStderr == 0 {
		opts.MaxStderr = 1 << 20
	}
	opts.Env = append(os.Environ(), helperMode+"="+mode, "HELPER_PAYLOAD="+payload)
	return exec.New(opts)
}

func TestRunner_Run_ReturnsStdout(t *testing.T) {
	r := helperRunner(t, "stdout", "hello world", exec.Options{})

	out, err := r.Run(context.Background(), nil)

	require.NoError(t, err)
	assert.Equal(t, "hello world", string(out))
}

func TestRunner_Run_RunsInTheRequestedDirectory(t *testing.T) {
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	r := helperRunner(t, "pwd", "", exec.Options{Cwd: dir})

	out, err := r.Run(context.Background(), nil)

	require.NoError(t, err)
	assert.Equal(t, resolved, string(out))
}

func TestRunner_Run_NonZeroExitCarriesTheCodeAndNoOutput(t *testing.T) {
	r := helperRunner(t, "exit", "3", exec.Options{})

	_, err := r.Run(context.Background(), nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, exec.ErrCommandFailed)
	code, ok := exec.ExitCode(err)
	require.True(t, ok)
	assert.Equal(t, 3, code)
	assert.NotContains(t, err.Error(), os.Args[0], "an error must never leak the executable path")
}

func TestRunner_Run_MissingExecutableIsUnavailable(t *testing.T) {
	r := helperRunner(t, "stdout", "", exec.Options{
		Executable: filepath.Join(t.TempDir(), "definitely-not-here"),
	})

	_, err := r.Run(context.Background(), nil)

	assert.ErrorIs(t, err, exec.ErrCommandUnavailable)
}

func TestRunner_Run_StdoutBeyondTheCeilingIsRefused(t *testing.T) {
	r := helperRunner(t, "flood", "64", exec.Options{MaxStdout: 4096})

	_, err := r.Run(context.Background(), nil)

	assert.ErrorIs(t, err, exec.ErrOutputLimit)
}

func TestRunner_Run_StderrBeyondTheCeilingIsRefused(t *testing.T) {
	r := helperRunner(t, "stderr", strings.Repeat("e", 4096), exec.Options{MaxStderr: 16})

	_, err := r.Run(context.Background(), nil)

	assert.ErrorIs(t, err, exec.ErrOutputLimit)
}

func TestRunner_Run_BudgetIsSharedAcrossCommands(t *testing.T) {
	r := helperRunner(t, "stdout", strings.Repeat("a", 60), exec.Options{MaxStdout: 100})

	_, err := r.Run(context.Background(), nil)
	require.NoError(t, err, "the first command fits inside the shared budget")

	_, err = r.Run(context.Background(), nil)
	assert.ErrorIs(t, err, exec.ErrOutputLimit,
		"a fanned-out pipeline must not exceed its ceiling by splitting the work")
}

func TestRunner_Run_CancelledContextIsReportedAsCancellation(t *testing.T) {
	r := helperRunner(t, "sleep", "5000", exec.Options{})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := r.Run(ctx, nil)
		done <- err
	}()
	cancel()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(30 * time.Second):
		t.Fatal("a cancelled command must not outlive its context")
	}
}

func TestRunner_Run_DeadlineKillsTheCommand(t *testing.T) {
	r := helperRunner(t, "sleep", "5000", exec.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := r.Run(ctx, nil)

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 30*time.Second)
}

func TestRunner_Run_AcquireIsHeldForTheCommandAndAlwaysReleased(t *testing.T) {
	var held, released int
	r := helperRunner(t, "stdout", "ok", exec.Options{
		Acquire: func(context.Context) (func(), error) {
			held++
			return func() { released++ }, nil
		},
	})

	_, err := r.Run(context.Background(), nil)

	require.NoError(t, err)
	assert.Equal(t, 1, held)
	assert.Equal(t, 1, released)
}

func TestRunner_Run_AcquireFailureRunsNoCommand(t *testing.T) {
	sentinel := errors.New("budget exhausted")
	r := helperRunner(t, "exit", "9", exec.Options{
		Acquire: func(context.Context) (func(), error) { return nil, sentinel },
	})

	_, err := r.Run(context.Background(), nil)

	assert.ErrorIs(t, err, sentinel)
}

func TestRunner_Run_AcquireIsReleasedEvenWhenTheCommandFails(t *testing.T) {
	released := 0
	r := helperRunner(t, "exit", "1", exec.Options{
		Acquire: func(context.Context) (func(), error) {
			return func() { released++ }, nil
		},
	})

	_, err := r.Run(context.Background(), nil)

	require.Error(t, err)
	assert.Equal(t, 1, released, "a failed command must not leak its permit")
}

func TestExecutable_AbsolutePathIsUsedVerbatim(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "claude")

	assert.Equal(t, abs, exec.Executable(abs, nil))
}

func TestExecutable_SearchesTheChildPathNotTheParents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable bit semantics differ on windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "fakecli")
	require.NoError(t, os.WriteFile(target, []byte("#!/bin/sh\n"), 0o700))

	got := exec.Executable("fakecli", []string{"PATH=" + dir})

	assert.Equal(t, target, got)
}

func TestExecutable_SkipsDirectoriesAndNonExecutables(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable bit semantics differ on windows")
	}
	shadow := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(shadow, "fakecli"), 0o700))
	plain := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(plain, "fakecli"), nil, 0o600))
	real := t.TempDir()
	target := filepath.Join(real, "fakecli")
	require.NoError(t, os.WriteFile(target, []byte("#!/bin/sh\n"), 0o700))

	got := exec.Executable("fakecli", []string{
		"PATH=" + strings.Join([]string{shadow, plain, real}, string(filepath.ListSeparator)),
	})

	assert.Equal(t, target, got)
}
