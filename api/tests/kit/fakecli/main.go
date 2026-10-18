//go:build integration

// Command fakecli stands in for a provider CLI in tests. It records how it was
// launched, optionally refuses a resume, and otherwise behaves like `cat`.
package main

import (
	"encoding/json"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

// launch mirrors kit.Launch. It is repeated here so this binary does not link
// the whole test kit.
type launch struct {
	RunnerID string   `json:"runnerId"`
	Token    string   `json:"token"`
	Argv     []string `json:"argv"`
	PID      int      `json:"pid"`
}

func main() {
	// Before the capture exists, so a SIGTERM sent the moment a test sees it is handled.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	dir := os.Getenv("FAKECLI_DIR")
	if dir == "" {
		os.Exit(2) // an unset capture dir would write into the working directory
	}
	rec := launch{
		RunnerID: os.Getenv("FAKECLI_SEG"),
		Token:    os.Getenv("FAKECLI_TOKEN"),
		Argv:     os.Args[1:],
		PID:      os.Getpid(),
	}
	if err := record(dir, rec); err != nil {
		os.Exit(2)
	}
	if code := refuseResume(dir, rec.Argv); code != 0 {
		os.Exit(code)
	}
	os.Exit(echoUntilStopped(stop))
}

// refuseResume prints the canned refusal and returns 1 when this launch resumes a
// session the test marked refused; 0 when it does not, 2 when printing fails.
func refuseResume(dir string, argv []string) int {
	id := resumeValue(argv)
	if id == "" {
		return 0
	}
	refusal, err := os.ReadFile(filepath.Join(dir, "refuse-"+id)) //nolint:gosec // G304: dir is the harness-set capture dir
	if err != nil {
		return 0
	}
	if _, err := os.Stdout.Write(refusal); err != nil {
		return 2
	}
	return 1
}

// record writes the capture under a temp name first, so a reader never sees half
// a file.
func record(dir string, rec launch) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "."+rec.RunnerID+".tmp")
	if err := os.WriteFile(tmp, raw, 0o600); err != nil { //nolint:gosec // G703: tmp is under the harness-set capture dir
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, rec.RunnerID+".json")) //nolint:gosec // G703: both paths are under the harness-set capture dir
}

// resumeValue is the session a launch resumes: the arg after --resume, or the arg
// after a leading `resume` positional.
func resumeValue(argv []string) string {
	for i, arg := range argv {
		if arg == "--resume" && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	for i, arg := range argv {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if arg == "resume" && i+1 < len(argv) {
			return argv[i+1]
		}
		return ""
	}
	return ""
}

// echoUntilStopped copies stdin to stdout and returns the exit code: 0 on EOF,
// 143 on SIGTERM or SIGINT.
func echoUntilStopped(stop <-chan os.Signal) int {
	copied := make(chan error, 1)
	go func() {
		_, err := io.Copy(os.Stdout, os.Stdin)
		copied <- err
	}()
	select {
	case <-copied: // EOF, or the read error a closed PTY gives: stdin is gone either way
		return 0
	case <-stop:
		return 143
	}
}
