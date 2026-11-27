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
	"slices"
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

const shippedDescriptors = "internal/engine/agents/internal/protocol/internal/descriptor/descriptors-v3"

const recordedServeTurns = "internal/engine/agents/internal/protocol/testdata/fixtures"

const runnerTokenHeader = "X-Crowbar-Runner-Token"

var fakeCLI struct {
	once sync.Once
	dir  string
	path string
	err  error
}

func apiDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

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

func removeFakeCLI() {
	if fakeCLI.dir == "" {
		return
	}
	if err := os.RemoveAll(fakeCLI.dir); err != nil {
		fmt.Fprintf(os.Stderr, "kit: remove the fake CLI build dir: %v\n", err)
	}
}

type FakeProvider struct {
	ID         string
	Dir        string
	descriptor string
}

type FakeSpec struct {
	Base       string
	ID         string
	EventsYAML string
	Patch      func(doc map[string]any)
	Turn       []string
	Composer   *Composer
}

func InstallFakeProvider(tb testing.TB, home string, spec FakeSpec) FakeProvider {
	tb.Helper()
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
	require.NoError(tb, os.MkdirAll(descriptors, 0o700))
	path := filepath.Join(descriptors, id+".yaml")
	writeDescriptor(tb, path, raw)
	return FakeProvider{ID: id, Dir: dir, descriptor: path}
}

func writeDescriptor(tb testing.TB, path string, raw []byte) {
	tb.Helper()
	for _, f := range descriptorcheck.Validate(raw).Findings {
		require.NotEqual(tb, descriptorcheck.SeverityError, f.Severity,
			"kit: fake descriptor %s refused: %s (line %d): %s", path, f.Rule, f.Line, f.Message)
	}
	require.NoError(tb, os.WriteFile(path, raw, 0o600))
}

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
	if api, ok := block(tb, doc, "runtime")["api"].(map[string]any); ok {
		fixtures := filepath.Join(apiDir(), recordedServeTurns, spec.Base)
		serve := []any{cli, "app-server", dir, "{segid}", fixtures}
		for _, name := range spec.Turn {
			serve = append(serve, name)
		}
		api["serve"] = serve
		if attach, ok := api["attach"].([]any); ok && len(attach) > 0 {
			attach[0] = cli
		}
	}
	injection := []any{
		setEnv("FAKECLI_DIR", dir),
		setEnv("FAKECLI_SEG", "{segid}"),
		setEnv("FAKECLI_TOKEN", "{runner_token}"),
		setEnv("FAKECLI_HOOK", "{hook_curl} {hook_url}"),
	}
	for _, kv := range composerEnv(spec.Composer) {
		injection = append(injection, setEnv(kv[0], kv[1]))
	}
	doc["hooks_injection"] = injection
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

type Launch struct {
	RunnerID string   `json:"runnerId"`
	Token    string   `json:"token"`
	Argv     []string `json:"argv"`
	PID      int      `json:"pid"`
	Cwd      string   `json:"cwd"`
	dir      string
}

func (f FakeProvider) AwaitLaunch(tb testing.TB, runnerID string) Launch {
	tb.Helper()
	var l Launch
	require.Eventually(tb, func() bool {
		raw, err := os.ReadFile(filepath.Join(f.Dir, runnerID+".json")) //nolint:gosec // G304: the capture dir this provider made
		return err == nil && json.Unmarshal(raw, &l) == nil
	}, 10*time.Second, 20*time.Millisecond, "kit: fake CLI for runner %s never launched", runnerID)
	l.dir = f.Dir
	return l
}

func (f FakeProvider) LaunchedRunners(tb testing.TB) []string {
	tb.Helper()
	entries, err := os.ReadDir(f.Dir)
	require.NoError(tb, err)
	var runners []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".json") && !strings.HasPrefix(name, "onquit-") {
			runners = append(runners, strings.TrimSuffix(name, ".json"))
		}
	}
	sort.Strings(runners)
	return runners
}

func (f FakeProvider) RefuseResume(tb testing.TB, sessionID, line string) {
	tb.Helper()
	require.NoError(tb, os.WriteFile(filepath.Join(f.Dir, "refuse-"+sessionID), []byte(line+"\n"), 0o600))
}

func (l Launch) Kill(tb testing.TB) {
	tb.Helper()
	proc, err := os.FindProcess(l.PID)
	require.NoError(tb, err)
	require.NoError(tb, proc.Signal(syscall.SIGTERM))
}

func (l Launch) OnQuit(tb testing.TB, f FakeProvider, baseURL, provider, event string, payload []byte) {
	tb.Helper()
	raw, err := json.Marshal(map[string]string{
		"url":  fmt.Sprintf("%s/chats/runners/%s/hooks/%s/%s", baseURL, l.RunnerID, provider, event),
		"body": string(payload),
	})
	require.NoError(tb, err)
	require.NoError(tb, os.WriteFile(filepath.Join(f.Dir, "onquit-"+l.RunnerID+".json"), raw, 0o600))
}

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

var claudeHookRoutes = map[string][]string{
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
	"TaskCreated":        {"plan_step"},
	"TaskCompleted":      {"plan_step"},
}

func ClaudeHookRoutes(hookEventName, toolName string) []string {
	routes := claudeHookRoutes[hookEventName]
	if hookEventName == "PostToolUse" && toolName == "TaskUpdate" {
		routes = append(slices.Clone(routes), "plan_step")
	}
	return routes
}

func (l Launch) ReplayHooks(tb testing.TB, baseURL, provider, dir string,
	routes func(hookEventName, toolName string) []string, rewrite map[string]string,
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
			Tool string `json:"tool_name"`
		}
		require.NoError(tb, json.Unmarshal(raw, &head), entry.Name())
		events := routes(head.Name, head.Tool)
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
