package modeldiscovery

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func discoverDescriptor() *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe"}
	d.Spawn.Cmd = "true"
	d.Model = &spec.ModelSpec{Discover: &spec.ModelDiscoverSpec{
		Command: []string{"discover"},
		Adapter: spec.ModelDiscoverAdapterJSON,
	}}
	return d
}

func blockUntil(started, release chan struct{}) probeFunc {
	return func(context.Context, *spec.Descriptor, models.ProbeOptions, exec.Acquire) ([]Model, error) {
		close(started)
		<-release
		return []Model{{ID: "m1", Efforts: []string{"low"}}}, nil
	}
}

func TestRefresh_DoesNotBlockTheCaller(t *testing.T) {
	c := NewCache(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	c.probe = blockUntil(started, release)

	done := make(chan struct{})
	go func() {
		c.Refresh(discoverDescriptor(), "")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Refresh must return without waiting on the probe")
	}
	close(release)
	<-started
}

func TestAttemptRefresh_StoresASuccessfulProbe(t *testing.T) {
	c := NewCache(context.Background())
	c.probe = func(context.Context, *spec.Descriptor, models.ProbeOptions, exec.Acquire) ([]Model, error) {
		return []Model{{ID: "m1", Efforts: []string{"low", "high"}}, {ID: "m2"}}, nil
	}

	c.attemptRefresh(discoverDescriptor(), t.TempDir())

	assert.Equal(t, []string{"m1", "m2"}, c.Models("probe"))
	assert.Equal(t, []string{"low", "high"}, c.Efforts("probe", "m1"))
}

func TestAttemptRefresh_AFailedProbeKeepsThePreviousCatalogue(t *testing.T) {
	c := NewCache(context.Background())
	c.probe = func(context.Context, *spec.Descriptor, models.ProbeOptions, exec.Acquire) ([]Model, error) {
		return []Model{{ID: "m1"}}, nil
	}
	c.attemptRefresh(discoverDescriptor(), t.TempDir())
	require.Equal(t, []string{"m1"}, c.Models("probe"))

	c.entries["probe"] = entry{models: c.entries["probe"].models, fingerprint: "stale-forced"}
	c.probe = func(context.Context, *spec.Descriptor, models.ProbeOptions, exec.Acquire) ([]Model, error) {
		return nil, assert.AnError
	}
	c.attemptRefresh(discoverDescriptor(), t.TempDir())

	assert.Equal(t, []string{"m1"}, c.Models("probe"), "a failed refresh must not blank a working catalogue")
}

func TestAttemptRefresh_AFailedProbeLogsOnceNamingTheProvider(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	c := NewCache(context.Background())
	c.probe = func(context.Context, *spec.Descriptor, models.ProbeOptions, exec.Acquire) ([]Model, error) {
		return nil, context.DeadlineExceeded
	}
	d := discoverDescriptor()

	c.attemptRefresh(d, t.TempDir())
	c.attemptRefresh(d, t.TempDir())

	assert.Equal(t, 1, strings.Count(buf.String(), "level=WARN"), "one warning per failed attempt, none for the skipped one")
	assert.Contains(t, buf.String(), "provider="+d.ID)
	assert.Contains(t, buf.String(), "deadline exceeded")
}

func TestAttemptRefresh_SuccessWritesTheDiskCache(t *testing.T) {
	home := t.TempDir()
	c := NewCache(context.Background())
	c.probe = func(context.Context, *spec.Descriptor, models.ProbeOptions, exec.Acquire) ([]Model, error) {
		return []Model{{ID: "m1", Efforts: []string{"low"}}}, nil
	}

	c.attemptRefresh(discoverDescriptor(), home)

	raw, err := os.ReadFile(discoverCachePath(home, "probe"))
	require.NoError(t, err)
	var got []Model
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, []Model{{ID: "m1", Efforts: []string{"low"}}}, got)
}

func TestAttemptRefresh_CancelledLifecycleDoesNotWriteTheDiskCache(t *testing.T) {
	home := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	c := NewCache(ctx)
	c.probe = func(context.Context, *spec.Descriptor, models.ProbeOptions, exec.Acquire) ([]Model, error) {
		return []Model{{ID: "m1"}}, nil
	}
	cancel()

	c.attemptRefresh(discoverDescriptor(), home)

	assert.Empty(t, c.Models("probe"), "a cancelled lifecycle must not store a probe result")
	_, err := os.Stat(discoverCachePath(home, "probe"))
	assert.True(t, os.IsNotExist(err), "a cancelled lifecycle must not write the disk cache")
}

func TestAttemptRefresh_LifecycleCancelledMidProbeDoesNotWriteTheDiskCache(t *testing.T) {
	home := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	c := NewCache(ctx)
	started := make(chan struct{})
	release := make(chan struct{})
	c.probe = func(context.Context, *spec.Descriptor, models.ProbeOptions, exec.Acquire) ([]Model, error) {
		close(started)
		<-release
		return []Model{{ID: "m1"}}, nil
	}

	done := make(chan struct{})
	go func() {
		c.attemptRefresh(discoverDescriptor(), home)
		close(done)
	}()

	<-started
	cancel()
	close(release)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("attemptRefresh never returned")
	}

	assert.Empty(t, c.Models("probe"), "a lifecycle cancelled mid-probe must not store the result")
	_, err := os.Stat(discoverCachePath(home, "probe"))
	assert.True(t, os.IsNotExist(err), "a lifecycle cancelled mid-probe must not write the disk cache")
}

func TestRefresh_SeedsSynchronouslyFromDisk(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, writeDiscoverCache(home, "probe", []Model{{ID: "cached-1", Efforts: []string{"low"}}}))

	c := NewCache(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	c.probe = blockUntil(started, release)

	c.Refresh(discoverDescriptor(), home)

	assert.Equal(t, []string{"cached-1"}, c.Models("probe"), "disk seed must be visible before the probe ever runs")
	assert.Equal(t, []string{"low"}, c.Efforts("probe", "cached-1"))

	close(release)
	c.Close()
}

func TestWriteDiscoverCache_AnUnwritableHomeIsAnError(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	require.NoError(t, os.WriteFile(home, nil, 0o600))

	assert.Error(t, writeDiscoverCache(home, "probe", []Model{{ID: "m"}}))
}

func TestRefresh_FailedProbeKeepsTheDiskSeededCatalogue(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, writeDiscoverCache(home, "probe", []Model{{ID: "cached-1"}}))

	c := NewCache(context.Background())
	probed := make(chan struct{})
	c.probe = func(context.Context, *spec.Descriptor, models.ProbeOptions, exec.Acquire) ([]Model, error) {
		defer close(probed)
		return nil, assert.AnError
	}

	c.Refresh(discoverDescriptor(), home)
	require.Equal(t, []string{"cached-1"}, c.Models("probe"), "disk seed must be visible synchronously")

	select {
	case <-probed:
	case <-time.After(2 * time.Second):
		t.Fatal("background probe never ran")
	}

	assert.Equal(t, []string{"cached-1"}, c.Models("probe"), "a failed probe must not blank the disk-seeded catalogue")
}

func TestAttemptRefresh_SkipsWhenFreshAndFingerprintUnchanged(t *testing.T) {
	c := NewCache(context.Background())
	var calls atomic.Int32
	c.probe = func(context.Context, *spec.Descriptor, models.ProbeOptions, exec.Acquire) ([]Model, error) {
		calls.Add(1)
		return []Model{{ID: "m1"}}, nil
	}
	d := discoverDescriptor()

	c.attemptRefresh(d, t.TempDir())
	c.attemptRefresh(d, t.TempDir())

	assert.EqualValues(t, 1, calls.Load(), "a fresh, fingerprint-matched entry must not be re-probed")
}

func TestAttemptRefresh_NoOpForADescriptorWithNoDiscoverBlock(t *testing.T) {
	c := NewCache(context.Background())
	c.attemptRefresh(&spec.Descriptor{ID: "no-discovery"}, t.TempDir())

	assert.Empty(t, c.Models("no-discovery"))
}

func TestStore_SeedsModelsAndEfforts(t *testing.T) {
	c := NewCache(context.Background())

	c.Store("probe", []Model{{ID: "m1", Efforts: []string{"low"}}, {ID: "m2", Efforts: []string{"high"}}})

	assert.Equal(t, []string{"m1", "m2"}, c.Models("probe"))
	assert.Equal(t, []string{"high"}, c.Efforts("probe", "m2"))
}

func TestDefaultModel_EmptyWhenNoRowIsFlaggedDefault(t *testing.T) {
	c := NewCache(context.Background())

	c.Store("probe", []Model{{ID: "m1"}, {ID: "m2"}})

	assert.Empty(t, c.DefaultModel("probe"), "no row flagged Default — unknown, not m1 by position")
}

func TestDefaultModel_ReportsTheFlaggedRow(t *testing.T) {
	c := NewCache(context.Background())

	c.Store("probe", []Model{{ID: "m1"}, {ID: "m2", Default: true}})

	assert.Equal(t, "m2", c.DefaultModel("probe"))
}

func TestAttemptRefresh_AFailedProbeKeepsThePreviousDefaultModel(t *testing.T) {
	c := NewCache(context.Background())
	c.probe = func(context.Context, *spec.Descriptor, models.ProbeOptions, exec.Acquire) ([]Model, error) {
		return []Model{{ID: "m1", Default: true}}, nil
	}
	c.attemptRefresh(discoverDescriptor(), t.TempDir())
	require.Equal(t, "m1", c.DefaultModel("probe"))

	c.entries["probe"] = entry{
		models: c.entries["probe"].models, defaultModel: c.entries["probe"].defaultModel,
		fingerprint: "stale-forced",
	}
	c.probe = func(context.Context, *spec.Descriptor, models.ProbeOptions, exec.Acquire) ([]Model, error) {
		return nil, assert.AnError
	}
	c.attemptRefresh(discoverDescriptor(), t.TempDir())

	assert.Equal(t, "m1", c.DefaultModel("probe"))
}

func TestModels_UnresolvedProviderIsEmptyNotNilPanic(t *testing.T) {
	c := NewCache(context.Background())

	assert.Empty(t, c.Models("never-seen"))
	assert.Empty(t, c.Efforts("never-seen", "m1"))
}

func manifestDescriptor() *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe"}
	d.Model = &spec.ModelSpec{Manifest: &spec.ModelManifestSpec{
		URL:       "https://example.invalid/manifest.json",
		ItemsPath: "providers.test.models[]",
		Item:      spec.ModelItemMapping{ID: "{id}", Label: "{label}", Efforts: "efforts[]"},
	}}
	return d
}

func TestRefreshManifest_SeedsSynchronouslyFromEmbedded(t *testing.T) {
	c := NewCache(context.Background())
	embedded := []byte(
		`{"updatedAt":"2026-01-01T00:00:00Z","providers":{"test":{"models":[` +
			`{"id":"m1","label":"M1","efforts":["low","high"]}]}}}`,
	)

	c.RefreshManifest(manifestDescriptor(), t.TempDir(), embedded, false)

	assert.Equal(t, []string{"m1"}, c.Models("probe"))
	assert.Equal(t, []string{"low", "high"}, c.Efforts("probe", "m1"))
}

func TestRefreshManifest_DoesNotBlockTheCaller(t *testing.T) {
	c := NewCache(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	c.manifest = func(context.Context, *spec.ModelManifestSpec, []byte, string, bool) ([]Model, error) {
		close(started)
		<-release
		return []Model{{ID: "late"}}, nil
	}

	done := make(chan struct{})
	go func() {
		c.RefreshManifest(manifestDescriptor(), t.TempDir(), nil, true)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RefreshManifest must return without waiting on the fetch")
	}
	close(release)
	<-started
}

func TestAttemptManifestRefresh_AFailedFetchKeepsThePreviousCatalogue(t *testing.T) {
	c := NewCache(context.Background())
	c.manifest = func(context.Context, *spec.ModelManifestSpec, []byte, string, bool) ([]Model, error) {
		return []Model{{ID: "m1", Efforts: []string{"low"}}}, nil
	}
	d := manifestDescriptor()
	home := t.TempDir()
	c.attemptManifestRefresh(d, home, nil, true)
	require.Equal(t, []string{"m1"}, c.Models("probe"))

	c.entries["probe"] = entry{models: c.entries["probe"].models, fingerprint: "stale-forced"}
	c.manifest = func(context.Context, *spec.ModelManifestSpec, []byte, string, bool) ([]Model, error) {
		return nil, assert.AnError
	}
	c.attemptManifestRefresh(d, home, nil, true)

	assert.Equal(t, []string{"m1"}, c.Models("probe"), "a failed refresh must not blank a working catalogue")
}

func TestCache_DefaultModel_EmptyForManifestSource(t *testing.T) {
	c := NewCache(context.Background())
	embedded := []byte(
		`{"updatedAt":"2026-01-01T00:00:00Z","providers":{"test":{"models":[` +
			`{"id":"m1","label":"M1"},{"id":"m2","label":"M2"}]}}}`,
	)

	c.RefreshManifest(manifestDescriptor(), t.TempDir(), embedded, false)

	assert.Empty(t, c.DefaultModel("probe"))
}

func TestCache_Efforts_UnionKeyForManifestSource(t *testing.T) {
	c := NewCache(context.Background())
	c.Store("probe", []Model{
		{ID: "m1", Efforts: []string{"low", "high"}},
		{ID: "m2", Efforts: []string{"high", "max"}},
	})

	assert.ElementsMatch(t, []string{"low", "high", "max"}, c.Efforts("probe", ""))
}

func TestRegression_CloseJoinsAManifestRefreshStillOwingItsDiskWrite(t *testing.T) {
	home := t.TempDir()
	d := manifestDescriptor()
	c := NewCache(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	c.manifest = func(_ context.Context, man *spec.ModelManifestSpec, _ []byte, homeDir string, _ bool) ([]Model, error) {
		close(started)
		<-release
		if err := writeManifestCache(manifestCachePath(homeDir, man.URL), []byte(`{}`)); err != nil {
			return nil, err
		}
		return []Model{{ID: "m1"}}, nil
	}

	c.RefreshManifest(d, home, nil, true)
	<-started
	close(release)

	c.Close()

	_, err := os.Stat(manifestCachePath(home, d.Model.Manifest.URL))
	require.NoError(t, err,
		"Close must not return while a forked refresh still owes a write under home")
}

func TestRegression_RefreshAfterCloseForksNothing(t *testing.T) {
	home := t.TempDir()
	c := NewCache(context.Background())
	var probed atomic.Bool
	c.manifest = func(context.Context, *spec.ModelManifestSpec, []byte, string, bool) ([]Model, error) {
		probed.Store(true)
		return nil, nil
	}

	c.Close()
	c.Close()
	c.RefreshManifest(manifestDescriptor(), home, nil, true)
	c.Close()

	assert.False(t, probed.Load(), "a refresh kicked after Close must never fork")
}
