package agents_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestList_EnumeratesTheShippedAgents(t *testing.T) {
	list, err := agents.New().List(context.Background(), "")

	require.NoError(t, err)
	ids := make([]string, 0, len(list))
	for _, a := range list {
		ids = append(ids, a.ID())
	}
	matches, err := filepath.Glob("internal/protocol/internal/descriptor/descriptors-v3/*.yaml")
	require.NoError(t, err)
	want := make([]string, 0, len(matches))
	for _, m := range matches {
		want = append(want, strings.TrimSuffix(filepath.Base(m), ".yaml"))
	}
	assert.ElementsMatch(t, want, ids)
}

func TestGet_UnknownAgentIsReported(t *testing.T) {
	_, err := agents.New().Get(context.Background(), "", "telepathy")

	assert.ErrorIs(t, err, agents.ErrUnknownAgent)
}

func TestRegression_GetCachesAResolvedDescriptorUntilItsOverrideFileChanges(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, "probe.yaml")
	require.NoError(t, os.WriteFile(path, []byte(minimalOverride), 0o600))

	svc := agents.New()
	ctx := context.Background()
	first, err := svc.Get(ctx, home, "probe")
	require.NoError(t, err)
	assert.Equal(t, "probe", first.ID())

	info, err := os.Stat(path)
	require.NoError(t, err)
	stableModTime := info.ModTime()

	require.NoError(t, os.WriteFile(path, []byte("id: [unclosed"), 0o600))
	require.NoError(t, os.Chtimes(path, stableModTime, stableModTime))

	_, err = svc.Get(ctx, home, "probe")
	require.NoError(t, err, "THE FIX: an unchanged mtime serves the cached descriptor, "+
		"never re-reading a file that Get has no reason to believe changed")

	changedModTime := stableModTime.Add(time.Second)
	require.NoError(t, os.Chtimes(path, changedModTime, changedModTime))

	_, err = svc.Get(ctx, home, "probe")
	assert.Error(t, err, "a real mtime change must invalidate the cache and surface the now-broken override")
}

func TestInjectionRegistry_RecognisesAnEchoOncePerRunner(t *testing.T) {
	e := agents.New()
	e.RecordInjection("runner-1", "handoff blob")

	remainder, found := e.ConsumeInjectedPrefix("runner-1", "handoff blob")
	assert.True(t, found)
	assert.Empty(t, remainder)
	_, found = e.ConsumeInjectedPrefix("runner-1", "handoff blob")
	assert.False(t, found)

	e.RecordInjection("runner-2", "other")
	e.ForgetRunner("runner-2")
	_, found = e.ConsumeInjectedPrefix("runner-2", "other")
	assert.False(t, found)
}

func manifestDescriptorYAML(url string) string {
	return `
id: synthetic
spawn:
  cmd: true
  interactive_required: true
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last
runtime:
  transport: hooks
  hooks:
    format: json
model:
  manifest:
    url: ` + url + `
    items_path: "providers.synthetic.models[]"
    item:
      id: "{id}"
      label: "{label}"
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--model", value: "{model}" }
`
}

func TestManifestFetch_DefaultsDisabledUntilTheSettingsGetterIsWired(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(
			`{"updatedAt":"2099-01-01T00:00:00Z","providers":{"synthetic":{"models":[` +
				`{"id":"m1","label":"M1"}]}}}`,
		))
	}))
	defer server.Close()
	realClient := http.DefaultClient
	http.DefaultClient = server.Client()
	t.Cleanup(func() { http.DefaultClient = realClient })

	home := t.TempDir()
	writeDescriptor(t, home, "synthetic", manifestDescriptorYAML(server.URL))
	svc := agents.New()

	a, err := svc.Get(context.Background(), home, "synthetic")
	require.NoError(t, err)
	assert.Empty(t, a.Models(), "no embedded fallback for this id, and fetch defaults disabled")
	assert.Equal(t, 0, calls, "a bare New() must never dial out")

	svc.SetManifestFetchEnabled(func() bool { return true })
	require.Eventually(t, func() bool {
		list, err := svc.List(context.Background(), home)
		if err != nil {
			return false
		}
		for _, p := range list {
			if p.ID() == "synthetic" {
				return len(p.Models()) == 1
			}
		}
		return false
	}, 2*time.Second, 5*time.Millisecond)
	assert.Positive(t, calls, "enabling the getter must let the next refresh reach the network")
}
