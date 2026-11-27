//go:build integration && unix

package scripted_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/tests/kit"
)

const (
	envScript = "FAKECLI_SCRIPT"
	envLog    = "FAKECLI_LOG"
)

type script struct {
	Turn   []step `yaml:"turn"`
	Resume string `yaml:"resume"`
}

type step struct {
	Tool       string `yaml:"tool"`
	Subagent   string `yaml:"subagent"`
	Permission string `yaml:"permission"`
	Compact    bool   `yaml:"compact"`
	Delta      string `yaml:"delta"`
	Say        string `yaml:"say"`
	Hang       bool   `yaml:"hang"`
	Crash      bool   `yaml:"crash"`
	SlowMS     int    `yaml:"slow_ms"`
	Drop       bool   `yaml:"drop"`
}

func loadScript() script {
	raw, err := os.ReadFile(os.Getenv(envScript))
	if err != nil {
		return script{}
	}
	var s script
	_ = yaml.Unmarshal(raw, &s)
	return s
}

func (s script) reply() string {
	for _, st := range s.Turn {
		if st.Say != "" {
			return st.Say
		}
	}
	return "ok"
}

var logMu sync.Mutex

func record(cli, kind string, fields map[string]any) {
	logMu.Lock()
	defer logMu.Unlock()
	entry := map[string]any{"cli": cli, "kind": kind, "pid": os.Getpid()}
	for k, v := range fields {
		entry[k] = v
	}
	line, _ := json.Marshal(entry)
	f, err := os.OpenFile(os.Getenv(envLog), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(append(line, '\n'))
}

type hook struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

func (h hook) run(payload map[string]any) string {
	body, _ := json.Marshal(payload)
	if h.Type == "http" {
		return postHook(h, body)
	}
	cmd := exec.CommandContext(context.Background(), "sh", "-c", h.Command)
	cmd.Stdin = strings.NewReader(string(body))
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

func postHook(h hook, body []byte) string {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range h.Headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(out))
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func providerHome(env string) string {
	dir, err := kit.ProviderHomeIn(env, filepath.Dir(os.Getenv(envScript)))
	if err != nil {
		record("fake", "unsafe-home", map[string]any{"err": err.Error()})
		say(err.Error() + "\n")
		os.Exit(4)
	}
	return dir
}

func writeSessionFile(path string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o750)
	_ = os.WriteFile(path, []byte("{}\n"), 0o600)
}

func say(s string) { _, _ = os.Stdout.WriteString(s) }

func hang() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	<-c
	os.Exit(0)
}

func pause(ms int) { time.Sleep(time.Duration(ms) * time.Millisecond) }

func argAfter(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func positionalsAfterDashes(args []string) string {
	for i, a := range args {
		if a == "--" {
			return strings.Join(args[i+1:], "\n\n")
		}
	}
	return ""
}
