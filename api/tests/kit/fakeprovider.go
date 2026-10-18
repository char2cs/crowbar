//go:build integration

package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

// shippedDescriptors is where the descriptors the daemon embeds live, from api/.
const shippedDescriptors = "internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3"

// runnerTokenHeader is the header an HTTP hook proves its runner with.
const runnerTokenHeader = "X-Crowbar-Runner-Token"

var fakeCLI struct {
	once sync.Once
	dir  string
	path string
	err  error
}

// apiDir is the api/ module root, found from this file so tests need no cwd.
func apiDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// FakeCLIPath builds tests/kit/fakecli once per test binary and returns the binary.
func FakeCLIPath(t testing.TB) string {
	t.Helper()
	fakeCLI.once.Do(func() {
		fakeCLI.dir, fakeCLI.err = os.MkdirTemp("", "fakecli-")
		if fakeCLI.err != nil {
			return
		}
		fakeCLI.path = filepath.Join(fakeCLI.dir, "fakecli")
		build := exec.CommandContext(context.Background(), "go", "build",
			"-tags", "integration", "-o", fakeCLI.path, "./tests/kit/fakecli")
		build.Dir = apiDir()
		if out, err := build.CombinedOutput(); err != nil {
			fakeCLI.err = fmt.Errorf("go build fakecli: %w: %s", err, out)
		}
	})
	require.NoError(t, fakeCLI.err, "kit: build the fake provider CLI")
	return fakeCLI.path
}

// removeFakeCLI drops the built binary once the test binary's run is over.
func removeFakeCLI() {
	if fakeCLI.dir == "" {
		return
	}
	if err := os.RemoveAll(fakeCLI.dir); err != nil {
		fmt.Fprintf(os.Stderr, "kit: remove the fake CLI build dir: %v\n", err)
	}
}

// FakeProvider is a provider descriptor installed under <home>/descriptors whose
// process is the fake CLI.
type FakeProvider struct {
	ID  string // descriptor id the chat is created with
	Dir string // capture directory the fake CLI writes into
}

// FakeSpec selects what the installed descriptor is built from.
type FakeSpec struct {
	Base       string                   // shipped descriptor to keep ("claude"); "" = EventsYAML alone
	ID         string                   // installed id; defaults to Base
	EventsYAML string                   // replaces the events: block when non-empty (stub providers)
	Patch      func(doc map[string]any) // test-specific edit of the decoded document
}

// InstallFakeProvider writes the descriptor and points the daemon's provider
// homes at temp dirs. Tests that call it must not run in parallel.
func InstallFakeProvider(t testing.TB, home string, spec FakeSpec) FakeProvider {
	t.Helper()
	// Until a resume stops globbing the provider's transcript root, an unset root
	// would be the real home; a temp root keeps "not found" without reading it.
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())

	id := spec.ID
	if id == "" {
		id = spec.Base
	}
	require.NotEmpty(t, id, "kit: a fake provider needs an ID or a Base")
	dir := t.TempDir()
	doc := fakeDocument(t, spec, FakeCLIPath(t), dir)
	if spec.Patch != nil {
		spec.Patch(doc)
	}
	doc["id"] = id

	raw, err := yaml.Marshal(doc)
	require.NoError(t, err)
	// A refused override silently falls back to the shipped descriptor, which
	// would launch the real CLI.
	for _, f := range descriptorcheck.Validate(raw).Findings {
		require.NotEqual(t, descriptorcheck.SeverityError, f.Severity,
			"kit: fake descriptor %q refused: %s (line %d): %s", id, f.Rule, f.Line, f.Message)
	}
	descriptors := filepath.Join(home, "descriptors")
	require.NoError(t, os.MkdirAll(descriptors, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(descriptors, id+".yaml"), raw, 0o644))
	return FakeProvider{ID: id, Dir: dir}
}

func fakeDocument(t testing.TB, spec FakeSpec, cli, dir string) map[string]any {
	t.Helper()
	var doc map[string]any
	if spec.Base == "" {
		doc = bareDocument(cli)
	} else {
		doc = shippedDocument(t, spec.Base, cli)
	}
	if spec.EventsYAML != "" {
		var events map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(spec.EventsYAML), &events), "kit: EventsYAML")
		doc["events"] = events
	}
	doc["hooks_injection"] = []any{
		setEnv("FAKECLI_DIR", dir),
		setEnv("FAKECLI_SEG", "{segid}"),
		setEnv("FAKECLI_TOKEN", "{runner_token}"),
		// The only hook-command shape descriptorcheck accepts as wiring the hooks.
		setEnv("FAKECLI_HOOK", "{hook_curl} {hook_url}"),
	}
	return doc
}

func bareDocument(cli string) map[string]any {
	message := []any{pass("--"), pass("{message}")}
	return map[string]any{
		"spawn": map[string]any{"cmd": cli, "interactive_required": true},
		"runtime": map[string]any{
			"transport": "hooks",
			"hooks":     map[string]any{"format": "json", "delivery": "http"},
		},
		"presentation": map[string]any{"prompt_submit": map[string]any{
			"strategy": "restart_tui", "fresh": message, "resume": message,
		}},
		"session": map[string]any{"resume": map[string]any{"arg": "--resume {id}"}},
	}
}

// shippedDocument is the embedded descriptor with the fake as its process and
// everything the fake cannot take (flags, models, MCP, prompts) removed.
func shippedDocument(t testing.TB, base, cli string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(apiDir(), shippedDescriptors, base+".yaml"))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &doc))

	spawn := block(t, doc, "spawn")
	spawn["cmd"] = cli
	delete(spawn, "forbid_flags")
	delete(spawn, "env")
	block(t, block(t, doc, "runtime"), "spawn")["cmd"] = cli
	delete(block(t, doc, "presentation"), "slash_catalog")
	for _, key := range []string{
		"model", "effort", "permission_levels", "telemetry", "mcp_injection",
		"context_inject", "resume_context_inject", "terminal_prompts", "terminal_notices",
	} {
		delete(doc, key)
	}
	return doc
}

func block(t testing.TB, parent map[string]any, key string) map[string]any {
	t.Helper()
	child, ok := parent[key].(map[string]any)
	require.True(t, ok, "kit: descriptor has no %q block", key)
	return child
}

func setEnv(name, value string) map[string]any {
	return map[string]any{"set_env": map[string]any{"name": name, "value": value}}
}

func pass(positional string) map[string]any {
	return map[string]any{"pass_arg": map[string]any{"positional": positional}}
}

// Launch is what one fake CLI process recorded at start.
type Launch struct {
	RunnerID string   `json:"runnerId"`
	Token    string   `json:"token"`
	Argv     []string `json:"argv"`
	PID      int      `json:"pid"`
}

// AwaitLaunch blocks until the fake CLI for runnerID has written its capture.
func (f FakeProvider) AwaitLaunch(t testing.TB, runnerID string) Launch {
	t.Helper()
	var l Launch
	require.Eventually(t, func() bool {
		raw, err := os.ReadFile(filepath.Join(f.Dir, runnerID+".json"))
		return err == nil && json.Unmarshal(raw, &l) == nil
	}, 10*time.Second, 20*time.Millisecond, "kit: fake CLI for runner %s never launched", runnerID)
	return l
}

// RefuseResume makes every later launch that resumes sessionID print line and
// exit 1.
func (f FakeProvider) RefuseResume(t testing.TB, sessionID, line string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(f.Dir, "refuse-"+sessionID), []byte(line+"\n"), 0o644))
}

// Kill sends SIGTERM to the launch's process: an exit with no hook.
func (l Launch) Kill(t testing.TB) {
	t.Helper()
	proc, err := os.FindProcess(l.PID)
	require.NoError(t, err)
	require.NoError(t, proc.Signal(syscall.SIGTERM))
}

// PostHook posts payload to <baseURL>/chats/runners/<runner>/hooks/<provider>/<event>
// with the runner token. baseURL is the server URL plus the repo or home route
// prefix. The body is read and closed, then kept readable on the response.
func (l Launch) PostHook(t testing.TB, baseURL, provider, event string, payload []byte) *http.Response {
	t.Helper()
	url := fmt.Sprintf("%s/chats/runners/%s/hooks/%s/%s", baseURL, l.RunnerID, provider, event)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set(runnerTokenHeader, l.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp
}

// ClaudeHookRoutes maps a recorded payload's hook_event_name to the canonical
// events the shipped claude descriptor posts it as. Notification is registered
// twice there, so it goes to both.
var ClaudeHookRoutes = map[string][]string{
	"SessionStart":       {"session_start"},
	"UserPromptSubmit":   {"user_prompt"},
	"Stop":               {"turn_stop"},
	"StopFailure":        {"turn_failed"},
	"MessageDisplay":     {"message_delta"},
	"PreToolUse":         {"tool_pre"},
	"PostToolUse":        {"tool_post"},
	"PostToolUseFailure": {"tool_fail"},
	"SubagentStart":      {"subagent_pre"},
	"SubagentStop":       {"subagent_post"},
	"PermissionRequest":  {"permission"},
	"Elicitation":        {"elicitation"},
	"PreCompact":         {"compact_pre"},
	"PostCompact":        {"compact_post"},
	"SessionEnd":         {"session_end"},
	"Notification":       {"notification", "idle"},
}

// ReplayHooks posts every *.json file in dir (lexical order) through l, once per
// route in routes[payload.hook_event_name], after replacing every key of rewrite
// with its value in the raw bytes. A payload whose hook name has no route is
// skipped and its file name returned in skipped.
func (l Launch) ReplayHooks(t testing.TB, baseURL, provider, dir string,
	routes map[string][]string, rewrite map[string]string,
) (responses []*http.Response, skipped []string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	from := make([]string, 0, len(rewrite))
	for old := range rewrite {
		from = append(from, old)
	}
	sort.Strings(from)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		require.NoError(t, err)
		for _, old := range from {
			raw = bytes.ReplaceAll(raw, []byte(old), []byte(rewrite[old]))
		}
		var head struct {
			Name string `json:"hook_event_name"`
		}
		require.NoError(t, json.Unmarshal(raw, &head), entry.Name())
		events := routes[head.Name]
		if len(events) == 0 {
			skipped = append(skipped, entry.Name())
			continue
		}
		for _, event := range events {
			responses = append(responses, l.PostHook(t, baseURL, provider, event, raw))
		}
	}
	return responses, skipped
}
