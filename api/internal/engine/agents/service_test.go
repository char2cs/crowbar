package agents_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	assert.Equal(t, []string{"claude", "codex"}, ids)
}

func TestGet_UnknownAgentIsReported(t *testing.T) {
	_, err := agents.New().Get(context.Background(), "", "telepathy")

	assert.ErrorIs(t, err, agents.ErrUnknownAgent)
}

// TestRegression_GetCachesAResolvedDescriptorUntilItsOverrideFileChanges is
// the fix for a live-reported bug: Codex's streaming view visibly stalled
// mid-reply, self-correcting only once the turn ended, while Claude streamed
// smoothly. Get sits on the ingest hot path (once per hook, including once
// per streamed delta), and protocol.Resolve is deliberately uncached — a full
// disk read, YAML parse and rule-validate on every call, over a millisecond
// for codex's own descriptor, measured. Codex's much higher per-second event
// rate over its api-transport connection paid that cost often enough to fall
// behind the incoming stream; Claude's hooks-paced rate never did. This
// proves the cache holds an override's resolved descriptor stable across
// repeated Get calls — corrupting the override on disk WITHOUT changing its
// mtime must not be noticed, because noticing would mean Resolve ran again.
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

	// Corrupted, but pinned back to the EXACT mtime Get already saw — no
	// sleep, no timing dependency: the cache is asked to distinguish "this
	// file's content is different" (which it cannot, and must not try to)
	// from "this file's mtime is different" (which it can, cheaply).
	require.NoError(t, os.WriteFile(path, []byte("id: [unclosed"), 0o600))
	require.NoError(t, os.Chtimes(path, stableModTime, stableModTime))

	_, err = svc.Get(ctx, home, "probe")
	require.NoError(t, err, "THE FIX: an unchanged mtime serves the cached descriptor, "+
		"never re-reading a file that Get has no reason to believe changed")

	// Now genuinely invalidate it: a real mtime change must still be caught,
	// or this would not be a cache with correct invalidation — it would just
	// be permanently stale, and a developer's edited override would never
	// take effect without a daemon restart.
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

// manifestDescriptorYAML is a synthetic model.manifest: descriptor pointing
// at url — the shared embedded model-manifest.json has no "synthetic"
// provider key, so this id's embedded half always resolves empty, letting a
// test tell "the network was actually consulted" apart from "the embedded
// fallback still applies".
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

// TestManifestFetch_DefaultsDisabledUntilTheSettingsGetterIsWired proves the
// engine-side half of the fetch toggle (agents.go's own doc on
// SetManifestFetchEnabled): a bare New(), never wired to a settings getter,
// must NEVER dial out as a side effect of resolving a descriptor — only the
// embedded/disk halves apply. Wiring a getter that reports true is what
// switches the network half on.
func TestManifestFetch_DefaultsDisabledUntilTheSettingsGetterIsWired(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		// updatedAt must beat the REAL embedded model-manifest.json's own
		// timestamp (ProbeManifest picks whichever candidate is freshest),
		// so far in the future it can never accidentally lose that race.
		_, _ = w.Write([]byte(
			`{"updatedAt":"2099-01-01T00:00:00Z","providers":{"synthetic":{"models":[` +
				`{"id":"m1","label":"M1"}]}}}`,
		))
	}))
	defer server.Close()
	// model.manifest.url must be https: (rules.modelManifest) — httptest's
	// plain server can't satisfy that, so this borrows the TLS variant's own
	// trusting client for the one real net/http call ProbeManifest makes.
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
	// List, not Get: Get short-circuits on its own descriptor cache when the
	// override file's mtime is unchanged (agents.go's own doc on
	// descriptorCacheEntry), so it would never re-trigger
	// refreshModelsIfDeclared here. List always does. The refresh itself is
	// async, so poll the real signal (the model actually landing) instead
	// of sleeping.
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
