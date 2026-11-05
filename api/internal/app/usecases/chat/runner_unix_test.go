//go:build unix

package chat_test

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

func TestSlashCatalogRejectsResultWhenTheProviderChangesDuringProbe(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(f.ws.home, "descriptors"), 0o700))
	require.NoError(t, os.MkdirAll(f.ws.worktree, 0o700))

	// Two FIFOs, not two files. Opening one end of a FIFO blocks until the other end
	// is opened, which makes each handoff with the helper PROCESS a real signal —
	// there is no interval to pick and nothing to re-sample, so the test cannot be
	// wrong about how fast this machine is.
	started := filepath.Join(t.TempDir(), "started")
	release := filepath.Join(t.TempDir(), "release")
	require.NoError(t, syscall.Mkfifo(started, 0o600))
	require.NoError(t, syscall.Mkfifo(release, 0o600))
	t.Setenv("CROWBAR_CATALOG_HELPER_STARTED", started)
	t.Setenv("CROWBAR_CATALOG_HELPER_RELEASE", release)
	// If the test fails before it releases, the helper is parked on the read end.
	// O_NONBLOCK returns ENXIO instead of blocking when it has already exited, so
	// cleanup can never be the thing that hangs.
	t.Cleanup(func() {
		if w, err := os.OpenFile(release, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			_ = w.Close()
		}
	})

	descriptor := fmt.Sprintf(`
id: codex
spawn:
  cmd: %q
  interactive_required: true
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: message
runtime:
  transport: hooks
  hooks:
    format: json
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
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(withLifecycle(descriptor)), 0o600,
	))

	chatID, _ := f.spawn(t, "codex")
	probeDone := make(chan error, 1)
	go func() {
		_, err := f.usecase.SlashCatalog(f.ctx, chatID)
		probeDone <- err
	}()

	inFlight, err := os.Open(started)
	require.NoError(t, err, "the deterministic provider command must be in flight")
	require.NoError(t, inFlight.Close())

	_, switchErr := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, switchErr)
	f.wait()
	releaser, err := os.OpenFile(release, os.O_WRONLY, 0)
	require.NoError(t, err)
	require.NoError(t, releaser.Close())

	select {
	case err := <-probeDone:
		require.ErrorIs(t, err, agentusecase.ErrSlashCatalogSuperseded,
			"a catalogue of a provider the chat has left must never be returned")
	case <-time.After(3 * time.Second):
		t.Fatal("slash catalog did not finish after releasing its provider command")
	}
}
