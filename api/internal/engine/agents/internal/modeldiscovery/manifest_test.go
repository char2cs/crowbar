package modeldiscovery_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/modeldiscovery"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func realEmbeddedManifest(t *testing.T) []byte {
	t.Helper()
	data := protocol.EmbeddedModelManifest()
	require.NotEmpty(t, data, "descriptor.go must embed descriptors-v3/*.json")
	return data
}

// realClaudeManifestSpec reads claude's OWN model.manifest: block off the
// shipped descriptor, so this test tracks claude.yaml rather than a second,
// hand-typed copy of its mapping.
func realClaudeManifestSpec(t *testing.T) *spec.ModelManifestSpec {
	t.Helper()
	d, err := protocol.Resolve(context.Background(), "", "claude", nil)
	require.NoError(t, err)
	require.NotNil(t, d.Model)
	require.NotNil(t, d.Model.Manifest, "claude.yaml must declare model.manifest:")
	return d.Model.Manifest
}

func TestProbeManifest_MapsAllFiveClaudeModelsFromTheRealBundle(t *testing.T) {
	man := realClaudeManifestSpec(t)
	embedded := realEmbeddedManifest(t)

	got, err := modeldiscovery.ProbeManifest(context.Background(), man, embedded, "", false)

	require.NoError(t, err)
	ids := make([]string, len(got))
	for i, m := range got {
		ids[i] = m.ID
	}
	assert.Equal(t, []string{"fable", "opus", "sonnet", "haiku", "opusplan"}, ids,
		"keep_when: status=current keeps every row in the bundled file today")
	for _, m := range got {
		assert.NotEmpty(t, m.Label, "model %q", m.ID)
		assert.NotEmpty(t, m.Efforts, "model %q must carry its own per-row efforts", m.ID)
		assert.False(t, m.Default, "the manifest states no default_when-equivalent")
	}
}

func TestProbeManifest_KeepWhenDropsNonCurrentRows(t *testing.T) {
	man := &spec.ModelManifestSpec{
		ItemsPath: "providers.test.models[]",
		KeepWhen:  &spec.ModelFieldMatch{Field: "status", Equals: "current"},
		Item:      spec.ModelItemMapping{ID: "{id}", Label: "{label}", Efforts: "efforts[]"},
	}
	embedded := manifestJSON(time.Now(), []manifestRow{
		{ID: "a", Label: "A", Status: "current", Efforts: []string{"low"}},
		{ID: "b", Label: "B", Status: "deprecated", Efforts: []string{"low"}},
	})

	got, err := modeldiscovery.ProbeManifest(context.Background(), man, embedded, "", false)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].ID)
}

func TestProbeManifest_UnsupportedWhenSpecIsNil(t *testing.T) {
	_, err := modeldiscovery.ProbeManifest(context.Background(), nil, []byte(`{}`), "", false)
	assert.ErrorIs(t, err, modeldiscovery.ErrUnsupported)
}

func TestProbeManifest_MalformedEmbeddedIsReported(t *testing.T) {
	man := &spec.ModelManifestSpec{
		ItemsPath: "providers.test.models[]",
		Item:      spec.ModelItemMapping{ID: "{id}", Label: "{label}"},
	}
	_, err := modeldiscovery.ProbeManifest(context.Background(), man, []byte("not json"), "", false)
	assert.ErrorIs(t, err, modeldiscovery.ErrMalformedOutput)
}

// --- freshness precedence -------------------------------------------------

type manifestRow struct {
	ID      string
	Label   string
	Status  string
	Efforts []string
}

func manifestJSON(updatedAt time.Time, rows []manifestRow) []byte {
	items := ""
	for i, r := range rows {
		if i > 0 {
			items += ","
		}
		efforts := ""
		for j, e := range r.Efforts {
			if j > 0 {
				efforts += ","
			}
			efforts += fmt.Sprintf("%q", e)
		}
		items += fmt.Sprintf(`{"id":%q,"label":%q,"status":%q,"efforts":[%s]}`, r.ID, r.Label, r.Status, efforts)
	}
	return []byte(fmt.Sprintf(
		`{"updatedAt":%q,"providers":{"test":{"models":[%s]}}}`,
		updatedAt.UTC().Format(time.RFC3339), items,
	))
}

func testManifestSpec(url string) *spec.ModelManifestSpec {
	return &spec.ModelManifestSpec{
		URL:       url,
		ItemsPath: "providers.test.models[]",
		Item:      spec.ModelItemMapping{ID: "{id}", Label: "{label}", Efforts: "efforts[]"},
	}
}

func TestProbeManifest_RemoteNewerThanBundleWins(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	newer := time.Now()
	embedded := manifestJSON(old, []manifestRow{{ID: "bundled", Label: "Bundled", Status: "current"}})
	remote := manifestJSON(newer, []manifestRow{{ID: "remote", Label: "Remote", Status: "current"}})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(remote)
	}))
	defer server.Close()
	man := testManifestSpec(server.URL)

	got, err := modeldiscovery.ProbeManifest(context.Background(), man, embedded, t.TempDir(), true)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "remote", got[0].ID)
}

func TestProbeManifest_RemoteOlderThanBundleLoses(t *testing.T) {
	newer := time.Now()
	old := time.Now().Add(-time.Hour)
	embedded := manifestJSON(newer, []manifestRow{{ID: "bundled", Label: "Bundled", Status: "current"}})
	remote := manifestJSON(old, []manifestRow{{ID: "remote", Label: "Remote", Status: "current"}})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(remote)
	}))
	defer server.Close()
	man := testManifestSpec(server.URL)

	got, err := modeldiscovery.ProbeManifest(context.Background(), man, embedded, t.TempDir(), true)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "bundled", got[0].ID, "an older fetch must never override a newer cache")
}

func TestProbeManifest_FailedFetchFallsBackAndNeverEmptiesTheCatalogue(t *testing.T) {
	embedded := manifestJSON(time.Now(), []manifestRow{{ID: "bundled", Label: "Bundled", Status: "current"}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	man := testManifestSpec(server.URL)

	got, err := modeldiscovery.ProbeManifest(context.Background(), man, embedded, t.TempDir(), true)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "bundled", got[0].ID)
}

func TestProbeManifest_FetchDisabledNeverContactsTheNetwork(t *testing.T) {
	embedded := manifestJSON(time.Now(), []manifestRow{{ID: "bundled", Label: "Bundled", Status: "current"}})
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write(manifestJSON(time.Now().Add(time.Hour), []manifestRow{{ID: "remote", Label: "R", Status: "current"}}))
	}))
	defer server.Close()
	man := testManifestSpec(server.URL)

	got, err := modeldiscovery.ProbeManifest(context.Background(), man, embedded, t.TempDir(), false)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "bundled", got[0].ID)
	assert.Equal(t, 0, calls, "fetchEnabled=false must never dial out")
}

// TestProbeManifest_DiskCacheSurvivesAFollowingFailedFetch proves a
// successful fetch is persisted to disk (the "last good on-disk cache") and
// a LATER call whose own fetch fails still reads that disk copy rather than
// regressing to the embedded bundle.
func TestProbeManifest_DiskCacheSurvivesAFollowingFailedFetch(t *testing.T) {
	home := t.TempDir()
	embedded := manifestJSON(time.Now().Add(-time.Hour), []manifestRow{{ID: "bundled", Label: "Bundled", Status: "current"}})
	cached := manifestJSON(time.Now(), []manifestRow{{ID: "cached", Label: "Cached", Status: "current"}})

	up := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !up {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(cached)
	}))
	defer server.Close()
	man := testManifestSpec(server.URL)

	first, err := modeldiscovery.ProbeManifest(context.Background(), man, embedded, home, true)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Equal(t, "cached", first[0].ID, "setup: the fetch must have won and been persisted")

	up = false
	second, err := modeldiscovery.ProbeManifest(context.Background(), man, embedded, home, true)
	require.NoError(t, err)
	require.Len(t, second, 1)
	assert.Equal(t, "cached", second[0].ID, "a failed fetch must fall back to disk, not the older embedded copy")
}
