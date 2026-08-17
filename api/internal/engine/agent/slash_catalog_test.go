package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fixtureCatalogExecutor struct {
	mu      sync.Mutex
	outputs map[string][]byte
	errors  map[string]error
	calls   [][]string
}

func (f *fixtureCatalogExecutor) Run(_ context.Context, argv []string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string(nil), argv...))
	key := catalogArgvKey(argv)
	return append([]byte(nil), f.outputs[key]...), f.errors[key]
}

func (f *fixtureCatalogExecutor) called(argv ...string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, call := range f.calls {
		if catalogArgvKey(call) == catalogArgvKey(argv) {
			return true
		}
	}
	return false
}

func catalogArgvKey(argv []string) string { return strings.Join(argv, "\x00") }

func mustFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return raw
}

func TestCodexCatalogAdapterMapsOnlyBoundedSkillsSectionAndRedactsLocators(t *testing.T) {
	d, err := ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)
	runner := &fixtureCatalogExecutor{outputs: map[string][]byte{
		catalogArgvKey([]string{"debug", "prompt-input"}): mustFixture(t, "codex_prompt_input.json"),
	}, errors: map[string]error{}}

	got, err := probeSlashCatalog(context.Background(), d, runner)
	require.NoError(t, err)
	require.Equal(t, "codex", got.ProviderID)
	require.Equal(t, CatalogCompletenessModelVisible, got.Completeness)
	require.Len(t, got.Items, 2)
	require.Equal(t, "imagegen", got.Items[0].Label)
	require.Equal(t, "$imagegen ", got.Items[0].InsertText)
	require.Equal(t, "Generate or edit images.", got.Items[0].Description)
	require.Equal(t, "model-visible", got.Items[0].Source)
	require.Equal(t, "plugin-management:plugin-management", got.Items[1].Label)
	require.Equal(t, "$plugin-management:plugin-management ", got.Items[1].InsertText)
	require.Equal(t, "Discover installed plugins and connections.", got.Items[1].Description)
	require.NotContains(t, fmt.Sprint(got), "/Users/")
	require.NotContains(t, fmt.Sprint(got), "fixture-secret")
	require.Contains(t, got.Warnings[0], "model-visible")
}

func TestClaudeCatalogAdapterFansOutEnabledPluginsAndEmitsProviderInvocation(t *testing.T) {
	d, err := ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	runner := &fixtureCatalogExecutor{outputs: map[string][]byte{
		catalogArgvKey([]string{"plugin", "list", "--json"}):                                 mustFixture(t, "claude_plugin_list.json"),
		catalogArgvKey([]string{"plugin", "details", "superpowers@claude-plugins-official"}): mustFixture(t, "claude_plugin_details_superpowers.txt"),
		catalogArgvKey([]string{"plugin", "details", "empty-kit@claude-plugins-official"}):   mustFixture(t, "claude_plugin_details_empty.txt"),
	}, errors: map[string]error{}}

	got, err := probeSlashCatalog(context.Background(), d, runner)
	require.NoError(t, err)
	require.Equal(t, CatalogCompletenessPluginOnly, got.Completeness)
	require.Len(t, got.Items, 3)
	require.Equal(t, "superpowers:brainstorming", got.Items[0].Label)
	require.Equal(t, "/superpowers:brainstorming ", got.Items[0].InsertText)
	require.Equal(t, "superpowers", got.Items[0].Source)
	require.NotEmpty(t, got.Items[0].ID)
	require.False(t, runner.called("plugin", "details", "disabled-kit@claude-plugins-official"))
	require.NotContains(t, fmt.Sprint(got), "installPath")
	require.NotContains(t, fmt.Sprint(got), "/Users/")
	require.Contains(t, got.Warnings[0], "plugin skills only")
}

func TestClaudeCatalogDetailFailureIsPartialAndNeverLeaksRawError(t *testing.T) {
	d, err := ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	runner := &fixtureCatalogExecutor{
		outputs: map[string][]byte{
			catalogArgvKey([]string{"plugin", "list", "--json"}):                               mustFixture(t, "claude_plugin_list.json"),
			catalogArgvKey([]string{"plugin", "details", "empty-kit@claude-plugins-official"}): mustFixture(t, "claude_plugin_details_empty.txt"),
		},
		errors: map[string]error{
			catalogArgvKey([]string{"plugin", "details", "superpowers@claude-plugins-official"}): errors.New("token=raw-secret /Users/private/config.json"),
		},
	}

	got, err := probeSlashCatalog(context.Background(), d, runner)
	require.NoError(t, err)
	require.Empty(t, got.Items)
	joined := strings.Join(got.Warnings, " ")
	require.Contains(t, joined, "superpowers could not be inspected")
	require.NotContains(t, joined, "raw-secret")
	require.NotContains(t, joined, "/Users/")
}

type concurrencyCatalogExecutor struct {
	inventory []byte
	active    atomic.Int32
	maximum   atomic.Int32
}

func (f *concurrencyCatalogExecutor) Run(ctx context.Context, argv []string) ([]byte, error) {
	if len(argv) >= 3 && argv[0] == "plugin" && argv[1] == "list" {
		return f.inventory, nil
	}
	active := f.active.Add(1)
	defer f.active.Add(-1)
	for {
		maximum := f.maximum.Load()
		if active <= maximum || f.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	select {
	case <-time.After(25 * time.Millisecond):
		return []byte("  Skills (1): one\n"), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestInventoryDetailFanoutNeverExceedsFourCommands(t *testing.T) {
	d, err := ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	rows := make([]map[string]any, 12)
	for i := range rows {
		rows[i] = map[string]any{"id": fmt.Sprintf("plugin-%d@market", i), "enabled": true}
	}
	inventory, err := json.Marshal(rows)
	require.NoError(t, err)
	runner := &concurrencyCatalogExecutor{inventory: inventory}

	got, err := probeSlashCatalog(context.Background(), d, runner)
	require.NoError(t, err)
	require.Len(t, got.Items, 12)
	require.LessOrEqual(t, runner.maximum.Load(), int32(4))
	require.Greater(t, runner.maximum.Load(), int32(1), "details should actually fan out")
}

func TestCatalogNormalizationCapsItemsAndRedactsSensitiveDescription(t *testing.T) {
	candidates := make([]catalogCandidate, 205)
	for i := range candidates {
		candidates[i] = catalogCandidate{
			name:        fmt.Sprintf("skill-%03d", i),
			description: "read /Users/private/project/file.txt with token=super-secret and sk-fixture123456789",
			mapping: CatalogItemMapping{
				Label: "{name}", Description: "{description}", InsertText: "${name} ", Source: "safe",
			},
		}
	}
	items, warnings := normalizeCatalogItems(candidates, 200)
	require.Len(t, items, 200)
	require.Len(t, warnings, 1)
	require.NotContains(t, items[0].Description, "/Users/")
	require.NotContains(t, items[0].Description, "super-secret")
	require.NotContains(t, items[0].Description, "sk-fixture")
}

func TestBoundedCatalogExecutorCapsAggregateStdoutAndStderr(t *testing.T) {
	stdoutExecutor := helperExecutor(t, "bytes", 10, 100)
	stdoutExecutor.env = append(stdoutExecutor.env, "SLASH_HELPER_BYTES=6")
	_, err := stdoutExecutor.Run(context.Background(), helperArgv())
	require.NoError(t, err)
	_, err = stdoutExecutor.Run(context.Background(), helperArgv())
	require.ErrorIs(t, err, ErrSlashCatalogOutputLimit)

	stderrExecutor := helperExecutor(t, "stderr", 100, 5)
	stderrExecutor.env = append(stderrExecutor.env, "SLASH_HELPER_BYTES=6")
	_, err = stderrExecutor.Run(context.Background(), helperArgv())
	require.ErrorIs(t, err, ErrSlashCatalogOutputLimit)
}

func TestBoundedCatalogExecutorCancelsAndRedactsCommandFailure(t *testing.T) {
	sleeper := helperExecutor(t, "sleep", 1<<20, 1<<20)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := sleeper.Run(ctx, helperArgv())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), 2*time.Second)

	failure := helperExecutor(t, "fail", 1<<20, 1<<20)
	_, err = failure.Run(context.Background(), helperArgv())
	require.ErrorIs(t, err, ErrSlashCatalogCommandFailed)
	require.NotContains(t, err.Error(), "raw-secret")
	require.NotContains(t, err.Error(), "/Users/")
}

func TestBoundedCatalogExecutorAcquiresAndReleasesCallerProcessPermit(t *testing.T) {
	acquireErr := errors.New("process budget cancelled")
	blocked := helperExecutor(t, "fail", 1<<20, 1<<20)
	var blockedAcquires atomic.Int32
	blocked.acquireProcess = func(context.Context) (func(), error) {
		blockedAcquires.Add(1)
		return nil, acquireErr
	}
	_, err := blocked.Run(context.Background(), helperArgv())
	require.ErrorIs(t, err, acquireErr)
	require.Equal(t, int32(1), blockedAcquires.Load())

	failure := helperExecutor(t, "fail", 1<<20, 1<<20)
	var acquired atomic.Int32
	var released atomic.Int32
	failure.acquireProcess = func(context.Context) (func(), error) {
		acquired.Add(1)
		return func() { released.Add(1) }, nil
	}
	_, err = failure.Run(context.Background(), helperArgv())
	require.ErrorIs(t, err, ErrSlashCatalogCommandFailed)
	require.Equal(t, int32(1), acquired.Load())
	require.Equal(t, int32(1), released.Load(), "the permit must be released on command failure")
}

func TestProbeSlashCatalogUsesEffectivePathCwdAndClearedEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable symlink and POSIX PATH semantics")
	}
	binDir := t.TempDir()
	providerName := "catalog-provider-for-test"
	require.NoError(t, os.Symlink(os.Args[0], filepath.Join(binDir, providerName)))
	t.Setenv("PATH", "/usr/bin:/bin") // simulate the packaged app's original launchd PATH

	d, err := ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)
	d.Spawn.Cmd = providerName
	d.Spawn.Env.Clear = []string{"DROP_ME"}
	d.Presentation.SlashCatalog.Pipeline.Command = helperArgv()
	worktree := t.TempDir()
	env := []string{
		"PATH=" + binDir,
		"GO_WANT_SLASH_CATALOG_HELPER=catalog-env",
		"EXPECTED_CWD=" + worktree,
		"KEEP_ME=yes",
		"DROP_ME=provider-secret",
	}

	got, err := ProbeSlashCatalog(context.Background(), d, SlashCatalogProbeOptions{Cwd: worktree, Env: env})
	require.NoError(t, err)
	require.Len(t, got.Items, 1)
	require.Equal(t, "environment-correct", got.Items[0].Label,
		"probe must resolve argv[0] from the repaired child PATH, run in the worktree, and clear descriptor env")
}

func TestProbeSlashCatalogMapsWholeProbeDeadline(t *testing.T) {
	d, err := ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)
	d.Spawn.Cmd = os.Args[0]
	d.Presentation.SlashCatalog.TimeoutMS = 40
	d.Presentation.SlashCatalog.Pipeline.Command = helperArgv()

	_, err = ProbeSlashCatalog(context.Background(), d, SlashCatalogProbeOptions{
		Cwd: t.TempDir(),
		Env: []string{"GO_WANT_SLASH_CATALOG_HELPER=sleep"},
	})
	require.ErrorIs(t, err, ErrSlashCatalogTimeout)
}

func helperArgv() []string {
	return []string{"-test.run=^TestSlashCatalogHelperProcess$"}
}

func helperExecutor(t *testing.T, mode string, stdoutBudget, stderrBudget int) *boundedCatalogExecutor {
	t.Helper()
	return &boundedCatalogExecutor{
		executable:   os.Args[0],
		cwd:          t.TempDir(),
		env:          []string{"GO_WANT_SLASH_CATALOG_HELPER=" + mode},
		stdoutBudget: newOutputBudget(stdoutBudget),
		stderrBudget: newOutputBudget(stderrBudget),
	}
}

func TestSlashCatalogHelperProcess(t *testing.T) {
	mode := os.Getenv("GO_WANT_SLASH_CATALOG_HELPER")
	if mode == "" {
		return
	}
	switch mode {
	case "bytes":
		count, _ := strconv.Atoi(os.Getenv("SLASH_HELPER_BYTES"))
		_, _ = fmt.Fprint(os.Stdout, strings.Repeat("x", count))
	case "stderr":
		count, _ := strconv.Atoi(os.Getenv("SLASH_HELPER_BYTES"))
		_, _ = fmt.Fprint(os.Stderr, strings.Repeat("x", count))
	case "sleep":
		time.Sleep(30 * time.Second)
	case "fail":
		_, _ = fmt.Fprint(os.Stderr, "token=raw-secret /Users/private/config.json")
		os.Exit(7)
	case "catalog-env":
		cwd, _ := os.Getwd()
		actualCwd, _ := filepath.EvalSymlinks(cwd)
		expectedCwd, _ := filepath.EvalSymlinks(os.Getenv("EXPECTED_CWD"))
		pwd, _ := filepath.EvalSymlinks(os.Getenv("PWD"))
		cwdOK := actualCwd == expectedCwd && pwd == expectedCwd
		keepOK := os.Getenv("KEEP_ME") == "yes"
		dropOK := os.Getenv("DROP_ME") == ""
		name := fmt.Sprintf("environment-wrong-%t-%t-%t", cwdOK, keepOK, dropOK)
		if cwdOK && keepOK && dropOK {
			name = "environment-correct"
		}
		text := "<skills_instructions>\n- " + name + ": verified\n</skills_instructions>"
		_ = json.NewEncoder(os.Stdout).Encode([]any{map[string]any{
			"content": []any{map[string]any{"text": text}},
		}})
	default:
		os.Exit(8)
	}
	os.Exit(0)
}
