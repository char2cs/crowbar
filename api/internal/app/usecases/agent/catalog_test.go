package agent_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
)

func TestSlashCatalogRejectsResultWhenLiveRunnerChangesDuringProbe(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(f.ws.home, "descriptors"), 0o700))
	require.NoError(t, os.MkdirAll(f.ws.worktree, 0o700))

	started := filepath.Join(t.TempDir(), "started")
	release := filepath.Join(t.TempDir(), "release")
	t.Setenv("CROWBAR_CATALOG_HELPER_STARTED", started)
	t.Setenv("CROWBAR_CATALOG_HELPER_RELEASE", release)
	t.Cleanup(func() { _ = os.WriteFile(release, nil, 0o600) })

	descriptor := fmt.Sprintf(`
id: codex
spawn:
  cmd: %q
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: message }
presentation:
  slash_catalog:
    completeness: complete
    timeout_ms: 5000
    max_stdout_bytes: 1048576
    max_stderr_bytes: 65536
    max_items: 20
    pipeline:
      adapter: json_text_section
      command: ["-test.run=^TestSlashCatalogPlacementHelperProcess$"]
      text_path: "[].content[].text"
      start_marker: "<skills>"
      end_marker: "</skills>"
      item_pattern: '(?m)^- (?P<name>[^:]+): (?P<description>.*)$'
      item:
        label: "{name}"
        description: "{description}"
        insert_text: "${name} "
        source: "test"
`, os.Args[0])
	require.NoError(t, os.WriteFile(
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(descriptor), 0o600,
	))

	chatID, runnerID := f.spawn(t, "codex")
	probeDone := make(chan error, 1)
	go func() {
		_, err := f.usecase.SlashCatalog(f.ctx, chatID)
		probeDone <- err
	}()

	require.Eventually(t, func() bool {
		_, err := os.Stat(started)
		return err == nil
	}, 2*time.Second, 10*time.Millisecond, "the deterministic provider command must be in flight")

	_, err := f.runners.Exit(f.ctx, runnerID, time.Now())
	require.NoError(t, err)
	f.wait()
	require.NoError(t, os.WriteFile(release, nil, 0o600))

	select {
	case err := <-probeDone:
		require.ErrorIs(t, err, agentusecase.ErrSlashCatalogSuperseded,
			"a result from a TUI that no longer holds the chat must never be returned")
	case <-time.After(3 * time.Second):
		t.Fatal("slash catalog did not finish after releasing its provider command")
	}
}

func TestSlashCatalogPlacementHelperProcess(t *testing.T) {
	started := os.Getenv("CROWBAR_CATALOG_HELPER_STARTED")
	release := os.Getenv("CROWBAR_CATALOG_HELPER_RELEASE")
	if started == "" || release == "" {
		return
	}
	if err := os.WriteFile(started, nil, 0o600); err != nil {
		os.Exit(2)
	}
	deadline := time.Now().Add(4 * time.Second)
	for {
		if _, err := os.Stat(release); err == nil {
			break
		}
		if time.Now().After(deadline) {
			os.Exit(3)
		}
		time.Sleep(5 * time.Millisecond)
	}
	_, _ = fmt.Fprint(os.Stdout, `[{"content":[{"text":"<skills>\n- current: Current skill\n</skills>"}]}]`)
	os.Exit(0)
}

func TestSlashCatalog_MapsEveryEngineFailureToItsOwnAppError(t *testing.T) {
	testCases := []struct {
		name    string
		command string
		want    error
	}{
		{

			name:    "provider command unavailable",
			command: `"/nonexistent/crowbar-not-a-real-cli"`,
			want:    agentusecase.ErrSlashCatalogUnavailable,
		},
		{

			name:    "malformed output",
			command: `"/usr/bin/true"`,
			want:    agentusecase.ErrSlashCatalogMalformed,
		},
		{

			name:    "command failed",
			command: `"/usr/bin/false"`,
			want:    agentusecase.ErrSlashCatalogCommand,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			require.NoError(t, os.MkdirAll(filepath.Join(f.ws.home, "descriptors"), 0o700))
			require.NoError(t, os.MkdirAll(f.ws.worktree, 0o700))
			writeCatalogDescriptor(t, f, tc.command)

			chatID, _ := f.spawn(t, "codex")

			_, err := f.usecase.SlashCatalog(f.ctx, chatID)

			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestSlashCatalog_UnsupportedWhenTheProviderDeclaresNoCatalogue(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(f.ws.home, "descriptors"), 0o700))
	require.NoError(t, os.MkdirAll(f.ws.worktree, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(`
id: codex
spawn:
  cmd: /usr/bin/true
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: message }
`), 0o600))

	chatID, _ := f.spawn(t, "codex")

	_, err := f.usecase.SlashCatalog(f.ctx, chatID)

	require.ErrorIs(t, err, agentusecase.ErrSlashCatalogUnsupported)
}

func TestSlashCatalog_RefusesAChatWithNoLiveCLI(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	_, err := f.usecase.SlashCatalog(f.ctx, chatID)

	require.ErrorIs(t, err, agentusecase.ErrSlashCatalogNoLiveTUI)
}

func writeCatalogDescriptor(t *testing.T, f testFixture, command string) {
	t.Helper()
	require.NoError(t, os.WriteFile(
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(fmt.Sprintf(`
id: codex
spawn:
  cmd: %s
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: message }
presentation:
  slash_catalog:
    completeness: complete
    timeout_ms: 5000
    pipeline:
      adapter: json_text_section
      command: ["--version"]
      text_path: "[].content[].text"
      start_marker: "<skills>"
      end_marker: "</skills>"
      item_pattern: '(?m)^- (?P<name>[^:]+)$'
      item:
        label: "{name}"
        insert_text: "${name} "
        source: "test"
`, command)), 0o600))
}
