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
	if id := resumeValue(rec.Argv); id != "" {
		if refusal, err := os.ReadFile(filepath.Join(dir, "refuse-"+id)); err == nil {
			if _, err := os.Stdout.Write(refusal); err != nil {
				os.Exit(2)
			}
			os.Exit(1)
		}
	}
	os.Exit(echoUntilStopped())
}

// record writes the capture under a temp name first, so a reader never sees half
// a file.
func record(dir string, rec launch) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "."+rec.RunnerID+".tmp")
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, rec.RunnerID+".json"))
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
func echoUntilStopped() int {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
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
