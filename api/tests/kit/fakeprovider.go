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
func FakeCLIPath(tb testing.TB) string {
	tb.Helper()
	fakeCLI.once.Do(func() {
		fakeCLI.dir, fakeCLI.err = os.MkdirTemp("", "fakecli-")
		if fakeCLI.err != nil {
			return
		}
		fakeCLI.path = filepath.Join(fakeCLI.dir, "fakecli")
		build := exec.CommandContext(context.Background(), "go", "build", //nolint:gosec // G204: fixed argv; the output path is a temp dir this call made
			"-tags", "integration", "-o", fakeCLI.path, "./tests/kit/fakecli")
		build.Dir = apiDir()
		if out, err := build.CombinedOutput(); err != nil {
			fakeCLI.err = fmt.Errorf("go build fakecli: %w: %s", err, out)
		}
	})
	require.NoError(tb, fakeCLI.err, "kit: build the fake provider CLI")
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
	ID         string // descriptor id the chat is created with
	Dir        string // capture directory the fake CLI writes into
	descriptor string // the installed <home>/descriptors/<id>.yaml
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
func InstallFakeProvider(tb testing.TB, home string, spec FakeSpec) FakeProvider {
	tb.Helper()
	// Crowbar never reads a provider's home; temp roots keep anything that would
	// resolve one off the developer's real one.
	tb.Setenv(ClaudeHomeEnv, tb.TempDir())
	tb.Setenv(CodexHomeEnv, tb.TempDir())

	id := spec.ID
	if id == "" {
		id = spec.Base
	}
	require.NotEmpty(tb, id, "kit: a fake provider needs an ID or a Base")
	dir := tb.TempDir()
	doc := fakeDocument(tb, spec, FakeCLIPath(tb), dir)
	if spec.Patch != nil {
		spec.Patch(doc)
	}
	doc["id"] = id

	raw, err := yaml.Marshal(doc)
	require.NoError(tb, err)
	descriptors := filepath.Join(home, "descriptors")
	require.NoError(tb, os.MkdirAll(descriptors, 0o750))
	path := filepath.Join(descriptors, id+".yaml")
	writeDescriptor(tb, path, raw)
	return FakeProvider{ID: id, Dir: dir, descriptor: path}
}

// writeDescriptor writes an override only once descriptorcheck accepts it.
func writeDescriptor(tb testing.TB, path string, raw []byte) {
	tb.Helper()
	// A refused override silently falls back to the shipped descriptor, which
	// would launch the real CLI.
	for _, f := range descriptorcheck.Validate(raw).Findings {
		require.NotEqual(tb, descriptorcheck.SeverityError, f.Severity,
			"kit: fake descriptor %s refused: %s (line %d): %s", path, f.Rule, f.Line, f.Message)
	}
	require.NoError(tb, os.WriteFile(path, raw, 0o600))
}

// BreakSpawn points every later launch at a CLI that does not exist. The new
// mtime is what makes the daemon reload the descriptor.
func (f FakeProvider) BreakSpawn(tb testing.TB) {
	tb.Helper()
	before, err := os.Stat(f.descriptor)
	require.NoError(tb, err)
	raw, err := os.ReadFile(f.descriptor)
	require.NoError(tb, err)
	var doc map[string]any
	require.NoError(tb, yaml.Unmarshal(raw, &doc))
	missing := filepath.Join(f.Dir, "missing-cli")
	block(tb, doc, "spawn")["cmd"] = missing
	block(tb, block(tb, doc, "runtime"), "spawn")["cmd"] = missing
	raw, err = yaml.Marshal(doc)
	require.NoError(tb, err)
	writeDescriptor(tb, f.descriptor, raw)
	bumped := before.ModTime().Add(time.Second)
	require.NoError(tb, os.Chtimes(f.descriptor, bumped, bumped))
}

func fakeDocument(tb testing.TB, spec FakeSpec, cli, dir string) map[string]any {
	tb.Helper()
	var doc map[string]any
	if spec.Base == "" {
		doc = bareDocument(cli)
	} else {
		doc = shippedDocument(tb, spec.Base, cli)
	}
	if spec.EventsYAML != "" {
		var events map[string]any
		require.NoError(tb, yaml.Unmarshal([]byte(spec.EventsYAML), &events), "kit: EventsYAML")
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
func shippedDocument(tb testing.TB, base, cli string) map[string]any {
	tb.Helper()
	raw, err := os.ReadFile(filepath.Join(apiDir(), shippedDescriptors, base+".yaml")) //nolint:gosec // G304: a checked-in descriptor under api/
	require.NoError(tb, err)
	var doc map[string]any
	require.NoError(tb, yaml.Unmarshal(raw, &doc))

	spawn := block(tb, doc, "spawn")
	spawn["cmd"] = cli
	delete(spawn, "forbid_flags")
	delete(spawn, "env")
	block(tb, block(tb, doc, "runtime"), "spawn")["cmd"] = cli
	delete(block(tb, doc, "presentation"), "slash_catalog")
	for _, key := range []string{
		"model", "effort", "permission_levels", "telemetry", "mcp_injection",
		"context_inject", "resume_context_inject", "terminal_prompts", "terminal_notices",
	} {
		delete(doc, key)
	}
	return doc
}

func block(tb testing.TB, parent map[string]any, key string) map[string]any {
	tb.Helper()
	child, ok := parent[key].(map[string]any)
	require.True(tb, ok, "kit: descriptor has no %q block", key)
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
func (f FakeProvider) AwaitLaunch(tb testing.TB, runnerID string) Launch {
	tb.Helper()
	var l Launch
	require.Eventually(tb, func() bool {
		raw, err := os.ReadFile(filepath.Join(f.Dir, runnerID+".json")) //nolint:gosec // G304: the capture dir this provider made
		return err == nil && json.Unmarshal(raw, &l) == nil
	}, 10*time.Second, 20*time.Millisecond, "kit: fake CLI for runner %s never launched", runnerID)
	return l
}

// RefuseResume makes every later launch that resumes sessionID print line and
// exit 1.
func (f FakeProvider) RefuseResume(tb testing.TB, sessionID, line string) {
	tb.Helper()
	require.NoError(tb, os.WriteFile(filepath.Join(f.Dir, "refuse-"+sessionID), []byte(line+"\n"), 0o600))
}

// Kill sends SIGTERM to the launch's process: an exit with no hook.
func (l Launch) Kill(tb testing.TB) {
	tb.Helper()
	proc, err := os.FindProcess(l.PID)
	require.NoError(tb, err)
	require.NoError(tb, proc.Signal(syscall.SIGTERM))
}

// PostHook posts payload to <baseURL>/chats/runners/<runner>/hooks/<provider>/<event>
// with the runner token. baseURL is the server URL plus the repo or home route
// prefix. The body is read and closed, then kept readable on the response.
func (l Launch) PostHook(tb testing.TB, baseURL, provider, event string, payload []byte) *http.Response {
	tb.Helper()
	url := fmt.Sprintf("%s/chats/runners/%s/hooks/%s/%s", baseURL, l.RunnerID, provider, event)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(payload))
	require.NoError(tb, err)
	req.Header.Set(runnerTokenHeader, l.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(tb, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(tb, err)
	require.NoError(tb, resp.Body.Close())
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

// ReplayHooks posts each *.json in dir (lexical order) through l once per route of its
// hook_event_name, after replacing every key of rewrite with its value in the raw bytes.
// A payload with no route is skipped and its file name returned.
func (l Launch) ReplayHooks(tb testing.TB, baseURL, provider, dir string,
	routes map[string][]string, rewrite map[string]string,
) (responses []*http.Response, skipped []string) {
	tb.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(tb, err)
	from := make([]string, 0, len(rewrite))
	for old := range rewrite {
		from = append(from, old)
	}
	sort.Strings(from)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name())) //nolint:gosec // G304: a fixture file the caller named
		require.NoError(tb, err)
		for _, old := range from {
			raw = bytes.ReplaceAll(raw, []byte(old), []byte(rewrite[old]))
		}
		var head struct {
			Name string `json:"hook_event_name"`
		}
		require.NoError(tb, json.Unmarshal(raw, &head), entry.Name())
		events := routes[head.Name]
		if len(events) == 0 {
			skipped = append(skipped, entry.Name())
			continue
		}
		for _, event := range events {
			responses = append(responses, l.PostHook(tb, baseURL, provider, event, raw)) //nolint:bodyclose // PostHook closes the body and returns a buffered copy
		}
	}
	return responses, skipped
}
