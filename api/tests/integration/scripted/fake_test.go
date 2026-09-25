//go:build integration && unix

package scripted_test

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

// The scripted CLI. The test binary is linked in as `claude` and `codex`;
// when the daemon launches it, TestMain sees FAKECLI_SCRIPT and runs it as
// the CLI its name says, speaking that vendor's real wire — claude's hook
// commands from --settings, codex's -c hooks and its app-server JSON-RPC — with
// every behaviour, healthy or faulty, read from the script file at each turn,
// so a test can change what the NEXT turn does by rewriting the file.
const (
	envScript = "FAKECLI_SCRIPT"
	envLog    = "FAKECLI_LOG"
)

// script is one test's recorded CLI behaviour.
type script struct {
	// Turn is what every turn does after the prompt, in order.
	Turn []step `yaml:"turn"`
	// Resume is "refuse" to reject every resume, as a CLI whose session store
	// was lost or whose flags changed does.
	Resume string `yaml:"resume"`
}

// step is one thing a CLI does mid-turn. Exactly one field is set.
type step struct {
	Tool       string `yaml:"tool"`       // a tool call, by name
	Subagent   string `yaml:"subagent"`   // a subagent, by type
	Permission string `yaml:"permission"` // an ask for this tool, waiting on the answer
	Compact    bool   `yaml:"compact"`    // a context compaction
	Delta      string `yaml:"delta"`      // streamed reply text
	Say        string `yaml:"say"`        // the reply the turn closes with
	Hang       bool   `yaml:"hang"`       // stop producing anything, forever
	Crash      bool   `yaml:"crash"`      // die mid-turn
	SlowMS     int    `yaml:"slow_ms"`    // stall before the next step
	Drop       bool   `yaml:"drop"`       // close the api socket (codex app-server)
	Idle       bool   `yaml:"idle"`       // report idle and never close the turn (codex)
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

// reply is the text a turn closes with.
func (s script) reply() string {
	for _, st := range s.Turn {
		if st.Say != "" {
			return st.Say
		}
	}
	return "ok"
}

var logMu sync.Mutex

// record appends one fact about this process to the shared log the test reads.
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

// runHook runs one hook command the way both CLIs do: through the shell, the
// payload on stdin, waiting for it; its stdout is the hook's verdict.
func runHook(command string, payload map[string]any) string {
	body, _ := json.Marshal(payload)
	cmd := exec.Command("sh", "-c", command) //nolint:gosec // the command is the daemon's own hook wiring
	cmd.Stdin = strings.NewReader(string(body))
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func writeSessionFile(path string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o750)
	_ = os.WriteFile(path, []byte("{}\n"), 0o600)
}

func say(s string) { _, _ = os.Stdout.WriteString(s) }

// hang parks the CLI the way a wedged one sits: until it is killed.
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

// positionalsAfterDashes is the prompt a launch carries: every argument
// after the first bare "--".
func positionalsAfterDashes(args []string) string {
	for i, a := range args {
		if a == "--" {
			return strings.Join(args[i+1:], "\n\n")
		}
	}
	return ""
}
