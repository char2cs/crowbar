//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

type launch struct {
	RunnerID string   `json:"runnerId"`
	Token    string   `json:"token"`
	Argv     []string `json:"argv"`
	PID      int      `json:"pid"`
	Cwd      string   `json:"cwd"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "app-server" {
		os.Exit(serveApp(os.Args[2:]))
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	dir := os.Getenv("FAKECLI_DIR")
	if dir == "" {
		os.Exit(2)
	}
	cwd, err := os.Getwd()
	if err != nil {
		os.Exit(2)
	}
	rec := launch{
		RunnerID: os.Getenv("FAKECLI_SEG"),
		Token:    os.Getenv("FAKECLI_TOKEN"),
		Argv:     os.Args[1:],
		PID:      os.Getpid(),
		Cwd:      cwd,
	}
	if err := record(dir, rec); err != nil {
		os.Exit(2)
	}
	if code := refuseResume(dir, rec.Argv); code != 0 {
		os.Exit(code)
	}
	var code int
	if os.Getenv("FAKECLI_COMPOSER") != "" {
		code = composeUntilStopped(stop, composerFromEnv(os.Stdout, dir, rec.RunnerID))
	} else {
		code = echoUntilStopped(stop)
	}
	if code == exitStopped {
		postOnQuit(dir, rec)
	}
	os.Exit(code)
}

const exitStopped = 143

func postOnQuit(dir string, rec launch) {
	raw, err := os.ReadFile(filepath.Join(dir, "onquit-"+rec.RunnerID+".json")) //nolint:gosec // G304: dir is the harness-set capture dir
	if err != nil {
		return
	}
	var hook struct {
		URL  string `json:"url"`
		Body string `json:"body"`
	}
	if json.Unmarshal(raw, &hook) != nil {
		return
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, hook.URL, bytes.NewReader([]byte(hook.Body)))
	if err != nil {
		return
	}
	req.Header.Set("X-Crowbar-Runner-Token", rec.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	if err := resp.Body.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "fakecli: close the quit hook's answer: %v\n", err)
	}
}

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

func echoUntilStopped(stop <-chan os.Signal) int {
	copied := make(chan error, 1)
	go func() {
		_, err := io.Copy(os.Stdout, os.Stdin)
		copied <- err
	}()
	select {
	case <-copied:
		return 0
	case <-stop:
		return exitStopped
	}
}
