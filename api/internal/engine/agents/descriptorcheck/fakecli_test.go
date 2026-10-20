//go:build unix

package descriptorcheck_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/testutil"
)

// The scripted fake CLI. The test binary re-executes itself as a provider CLI
// when FAKECLI_SCRIPT names a transcript, so a conformance run drives a real
// process over a real PTY, stdio and hook relay, with every behaviour —
// healthy or faulty — declared in testdata.
func TestMain(m *testing.M) {
	if script := os.Getenv("FAKECLI_SCRIPT"); script != "" {
		os.Exit(runFake(script, os.Args[1:]))
	}
	os.Exit(m.Run())
}

type transcript struct {
	Version string `yaml:"version"`
	Help    string `yaml:"help"`
	Boot    struct {
		Screen   string   `yaml:"screen"`
		ExitCode *int     `yaml:"exit_code"`
		Hooks    []string `yaml:"hooks"`
		// Modal parks the CLI on its boot screen, resuming or not.
		Modal bool `yaml:"modal"`
	} `yaml:"boot"`
	Turn struct {
		Hooks        []string `yaml:"hooks"`
		WriteSession bool     `yaml:"write_session"`
	} `yaml:"turn"`
	// UnknownResume is "exit" (the default), "hang" or "silent" (exits 1 saying nothing).
	UnknownResume string `yaml:"unknown_resume"`
	// Serve is "answer" (the default), "hang", "refuse" or "exit".
	Serve    string            `yaml:"serve"`
	Payloads map[string]string `yaml:"payloads"`
}

var defaultPayloads = map[string]string{
	"session_start": `{"session_id":"{session}"}`,
	"user_prompt":   `{"session_id":"{session}","prompt":"{prompt}"}`,
	"turn_stop":     `{"session_id":"{session}","last_assistant_message":"ok"}`,
}

func runFake(script string, args []string) int {
	raw, err := os.ReadFile(script)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	var tr transcript
	if err := yaml.Unmarshal(raw, &tr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	switch {
	case hasArg(args, "--version"):
		say(tr.Version + "\n")
		return 0
	case hasArg(args, "--help"):
		say(tr.Help + "\n")
		return 0
	case len(args) > 0 && args[0] == "app-server":
		return fakeServe(tr)
	}
	return fakeTUI(tr, args)
}

func fakeTUI(tr transcript, args []string) int {
	hooks := hookCommands(args)
	session := "11111111-2222-4333-8444-555555555555"
	say(tr.Boot.Screen)
	if tr.Boot.Modal {
		waitForKill()
	}
	if id := argAfter(args, "--resume"); id != "" {
		if !resumable(tr, id) {
			return 1
		}
		session = id
	}
	if tr.Boot.ExitCode != nil {
		return *tr.Boot.ExitCode
	}
	for _, event := range tr.Boot.Hooks {
		fireHook(tr, hooks, event, session, "")
	}
	lines := bufio.NewScanner(os.Stdin)
	for lines.Scan() {
		prompt := strings.TrimSpace(lines.Text())
		if prompt == "" {
			continue
		}
		if tr.Turn.WriteSession {
			_ = os.MkdirAll(filepath.Dir(sessionFile(session)), 0o750)
			_ = os.WriteFile(sessionFile(session), []byte("{}\n"), 0o600)
		}
		for _, event := range tr.Turn.Hooks {
			fireHook(tr, hooks, event, session, prompt)
		}
	}
	waitForKill()
	return 0
}

// resumable reports whether the session exists; an unknown one ends the
// CLI, or parks it when the transcript says the CLI hangs instead.
func resumable(tr transcript, id string) bool {
	if _, err := os.Stat(sessionFile(id)); err == nil {
		return true
	}
	if tr.UnknownResume == "hang" {
		waitForKill()
	}
	if tr.UnknownResume == "silent" {
		return false
	}
	say("No conversation found with session ID: " + id + "\n")
	return false
}

func say(s string) { _, _ = os.Stdout.WriteString(s) }

func fireHook(tr transcript, hooks map[string]string, event, session, prompt string) {
	cmd, ok := hooks[event]
	if !ok {
		return
	}
	payload := tr.Payloads[event]
	if payload == "" {
		payload = defaultPayloads[event]
	}
	payload = strings.NewReplacer("{session}", session, "{prompt}", prompt).Replace(payload)
	c := exec.CommandContext(context.Background(), "sh", "-c", cmd)
	c.Stdin = strings.NewReader(payload)
	_ = c.Run()
}

// fakeServe is the app-server on its own stdio: one JSON message per line,
// until its stdin closes.
func fakeServe(tr transcript) int {
	if tr.Serve == "exit" {
		return 1
	}
	serve := testutil.NewLinePeer(os.Stdin, os.Stdout)
	for {
		raw, err := serve.Read()
		if err != nil {
			return 0
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(raw, &req) != nil || req.ID == nil || tr.Serve == "hang" {
			continue
		}
		reply := map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}}
		if tr.Serve == "refuse" {
			delete(reply, "result")
			reply["error"] = map[string]any{"code": -32600, "message": "refused"}
		}
		if serve.WriteJSON(reply) != nil {
			return 0
		}
	}
}

func hookCommands(args []string) map[string]string {
	out := map[string]string{}
	for i := 0; i+1 < len(args); i++ {
		if args[i] != "--hook" {
			continue
		}
		if name, cmd, ok := strings.Cut(args[i+1], "="); ok {
			out[name] = cmd
		}
	}
	return out
}

func sessionFile(id string) string {
	return filepath.Join(os.Getenv("FAKECLI_HOME"), "sessions", id+".jsonl")
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func argAfter(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

// waitForKill parks the fake the way a live TUI sits: until it is killed.
func waitForKill() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGHUP)
	<-c
	os.Exit(0)
}
