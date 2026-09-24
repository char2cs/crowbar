package modeldiscovery

import (
	"context"
	"encoding/json"
	"os"
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

// blockUntil is a probeFunc that signals started, then blocks until release
// is closed — the real-signal alternative to a sleep for proving Refresh
// does not block its caller.
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
		// home="": this test never waits for the backgrounded attemptRefresh to
		// fully finish (only that Refresh itself returns promptly), so a real
		// tempdir would race its own t.Cleanup against that goroutine's later
		// disk-cache write.
		c.Refresh(discoverDescriptor(), "")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Refresh must return without waiting on the probe")
	}
	close(release)
	<-started // drain, avoid a leaked goroutine complaint
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

// TestAttemptRefresh_SuccessWritesTheDiskCache proves a successful probe
// persists its catalogue to discoverCachePath, so the NEXT cold start has
// something to seed from.
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

// TestAttemptRefresh_CancelledLifecycleDoesNotWriteTheDiskCache proves a
// refresh whose lifecycle is ALREADY cancelled before it even starts never
// stores or writes — even though the probe itself succeeds and ignores its
// own ctx, attemptRefresh's own ctx.Err() check is what refuses the result.
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

// TestAttemptRefresh_LifecycleCancelledMidProbeDoesNotWriteTheDiskCache is
// the closer-to-real-bug shape: the lifecycle is cancelled WHILE the probe
// is still running (the goroutine is in flight, exactly as it was when a
// test's t.TempDir() teardown raced it), not before it starts. The probe
// still succeeds — deliberately unaware of its own cancellation, the
// worst case — so only attemptRefresh's own post-probe ctx.Err() check
// stands between this and the leaked write the bug report captured.
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

// TestRefresh_SeedsSynchronouslyFromDisk is the actual bug this cache exists
// to fix: on a cold cache with a populated disk file, the FIRST Models() call
// after Refresh must already see it — never empty until the first live probe
// lands. The probe is left blocked on release so the assertion can only pass
// if the disk seed ran on Refresh's own calling goroutine, not the fork.
//
// afterRefresh is used only so this test can wait for the background fork to
// fully finish (including its own disk write) before returning — otherwise
// t.TempDir()'s cleanup can race that still-running write.
func TestRefresh_SeedsSynchronouslyFromDisk(t *testing.T) {
	home := t.TempDir()
	writeDiscoverCache(home, "probe", []Model{{ID: "cached-1", Efforts: []string{"low"}}})

	c := NewCache(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	c.probe = blockUntil(started, release)
	settled := make(chan struct{})
	c.afterRefresh = func(string) { close(settled) }

	c.Refresh(discoverDescriptor(), home)

	assert.Equal(t, []string{"cached-1"}, c.Models("probe"), "disk seed must be visible before the probe ever runs")
	assert.Equal(t, []string{"low"}, c.Efforts("probe", "cached-1"))

	close(release)
	select {
	case <-settled:
	case <-time.After(2 * time.Second):
		t.Fatal("background refresh never settled")
	}
}

// TestRefresh_FailedProbeKeepsTheDiskSeededCatalogue proves a FAILED live
// probe never empties the catalogue the synchronous disk seed just served —
// the same "never blank a resolved catalogue" contract
// TestAttemptRefresh_AFailedProbeKeepsThePreviousCatalogue pins for an
// in-memory seed, here for a disk-seeded one.
func TestRefresh_FailedProbeKeepsTheDiskSeededCatalogue(t *testing.T) {
	home := t.TempDir()
	writeDiscoverCache(home, "probe", []Model{{ID: "cached-1"}})

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

// TestRefreshManifest_SeedsSynchronouslyFromEmbedded proves the embedded
// half resolves on the CALLING goroutine, before RefreshManifest returns —
// Models()/Efforts() must never read empty just because the background
// fetch/disk-cache half hasn't run yet.
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
	<-started // drain, avoid a leaked goroutine complaint
}

// TestAttemptManifestRefresh_AFailedFetchKeepsThePreviousCatalogue proves the
// background half's own failure never blanks what the synchronous embedded
// seed (or an earlier successful refresh) already stored.
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

// TestCache_DefaultModel_EmptyForManifestSource pins that nothing infers a
// default for a manifest-derived catalogue — ModelManifestSpec has no
// default_when-equivalent field at all, so this is structural, not just
// untested.
func TestCache_DefaultModel_EmptyForManifestSource(t *testing.T) {
	c := NewCache(context.Background())
	embedded := []byte(
		`{"updatedAt":"2026-01-01T00:00:00Z","providers":{"test":{"models":[` +
			`{"id":"m1","label":"M1"},{"id":"m2","label":"M2"}]}}}`,
	)

	c.RefreshManifest(manifestDescriptor(), t.TempDir(), embedded, false)

	assert.Empty(t, c.DefaultModel("probe"))
}

// TestCache_Efforts_UnionKeyForManifestSource pins effortsOf's "" entry: the
// union of every model's own levels, so a picker shown before a model is
// confirmed still offers something — the contract provider.resolveEfforts
// depends on for claude's own "" key (agent-api.ts:329-338).
func TestCache_Efforts_UnionKeyForManifestSource(t *testing.T) {
	c := NewCache(context.Background())
	c.Store("probe", []Model{
		{ID: "m1", Efforts: []string{"low", "high"}},
		{ID: "m2", Efforts: []string{"high", "max"}},
	})

	assert.ElementsMatch(t, []string{"low", "high", "max"}, c.Efforts("probe", ""))
}

// TestRegression_CloseJoinsAManifestRefreshStillOwingItsDiskWrite pins the
// t.TempDir()-cleanup flake this cache caused across internal/api/v0 and
// internal/app/usecases/chat: the boot warm-up forked a manifest refresh, the
// test body returned, and the fetch's writeManifestCache landed in the home
// AFTERWARDS — recreating the directory RemoveAll had just emptied, which
// testing reports as "TempDir RemoveAll cleanup: ... directory not empty".
//
// Its cancellation-based sibling above cannot cover this. The discover path's
// write sits in attemptRefresh, AFTER a ctx.Err() gate that can refuse it; the
// manifest path's write sits inside ProbeManifest itself, upstream of every gate
// attemptManifestRefresh has. Cancelling can only shorten that window — the join
// is what closes it, so the write is on disk by the time Close returns.
func TestRegression_CloseJoinsAManifestRefreshStillOwingItsDiskWrite(t *testing.T) {
	home := t.TempDir()
	d := manifestDescriptor()
	c := NewCache(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	// The write is INSIDE the probe, where ProbeManifest really puts it, so no
	// post-probe ctx check could refuse it.
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

// TestRegression_RefreshAfterCloseForksNothing proves Close is a latch, not just
// a drain: a refresh kicked afterwards (a request still in flight when shutdown
// began) must not fork at all, or the join it already completed would be
// meaningless and the WaitGroup could be Added to after its Wait returned.
func TestRegression_RefreshAfterCloseForksNothing(t *testing.T) {
	home := t.TempDir()
	c := NewCache(context.Background())
	var probed atomic.Bool
	c.manifest = func(context.Context, *spec.ModelManifestSpec, []byte, string, bool) ([]Model, error) {
		probed.Store(true)
		return nil, nil
	}

	c.Close()
	c.Close() // idempotent
	c.RefreshManifest(manifestDescriptor(), home, nil, true)
	c.Close() // joins nothing; returns rather than hanging

	assert.False(t, probed.Load(), "a refresh kicked after Close must never fork")
}
