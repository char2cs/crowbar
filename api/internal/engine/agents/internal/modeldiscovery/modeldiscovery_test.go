package modeldiscovery_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/modeldiscovery"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// codexShapedDescriptor mirrors codex.yaml's own model.discover block, over
// `cat` printing the trimmed real capture instead of a live CLI — the point
// of the fixture is testing THIS mapping against real shape, not a live
// process.
func codexShapedDescriptor(fixture string) *spec.Descriptor {
	d := &spec.Descriptor{ID: "codex-shaped"}
	d.Spawn.Cmd = "cat"
	d.Model = &spec.ModelSpec{
		Discover: &spec.ModelDiscoverSpec{
			Command:   []string{fixture},
			Adapter:   spec.ModelDiscoverAdapterJSON,
			ItemsPath: "models[]",
			KeepWhen:  &spec.ModelFieldMatch{Field: "visibility", Equals: "list"},
			OrderBy:   "priority",
			Item: spec.ModelItemMapping{
				ID:            "{slug}",
				Label:         "{display_name}",
				Efforts:       "supported_reasoning_levels[].effort",
				DefaultEffort: "{default_reasoning_level}",
			},
		},
	}
	return d
}

func TestProbe_FiltersOrdersAndMapsARealCapture(t *testing.T) {
	fixture, err := filepath.Abs("testdata/codex_debug_models.json")
	require.NoError(t, err)
	d := codexShapedDescriptor(fixture)

	got, err := modeldiscovery.Probe(context.Background(), d, models.ProbeOptions{Cwd: t.TempDir()}, nil)

	require.NoError(t, err)
	ids := make([]string, len(got))
	for i, m := range got {
		ids[i] = m.ID
	}
	assert.Equal(t, []string{"gpt-6-astra", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5"}, ids,
		"visibility:hide models dropped, the rest ordered by ascending priority")
}

func TestProbe_DropsHiddenModels(t *testing.T) {
	fixture, err := filepath.Abs("testdata/codex_debug_models.json")
	require.NoError(t, err)
	d := codexShapedDescriptor(fixture)

	got, err := modeldiscovery.Probe(context.Background(), d, models.ProbeOptions{Cwd: t.TempDir()}, nil)

	require.NoError(t, err)
	for _, m := range got {
		assert.NotEqual(t, "gpt-reserve", m.ID)
		assert.NotEqual(t, "codex-auto-review", m.ID)
	}
}

func TestProbe_MapsEffortsAndDefaultEffortPerModel(t *testing.T) {
	fixture, err := filepath.Abs("testdata/codex_debug_models.json")
	require.NoError(t, err)
	d := codexShapedDescriptor(fixture)

	got, err := modeldiscovery.Probe(context.Background(), d, models.ProbeOptions{Cwd: t.TempDir()}, nil)
	require.NoError(t, err)

	byID := map[string]modeldiscovery.Model{}
	for _, m := range got {
		byID[m.ID] = m
	}
	astra := byID["gpt-6-astra"]
	assert.Equal(t, "GPT-6-Astra", astra.Label)
	assert.Equal(t, "low", astra.DefaultEffort)
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max", "ultra"}, astra.Efforts)

	luna := byID["gpt-5.6-luna"]
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, luna.Efforts,
		"gpt-5.6-luna has no ultra tier, unlike gpt-6-astra")
}

// TestProbe_RealCaptureHasNoStatedDefault pins the measured fact that
// `codex debug models` carries no per-model "this is the default" field —
// only default_reasoning_level, an EFFORT default, not a model one — so
// with no default_when: declared (codexShapedDescriptor doesn't), every
// row's Default is false and gpt-6-astra ordering first must NEVER be read
// as it being the default.
func TestProbe_RealCaptureHasNoStatedDefault(t *testing.T) {
	fixture, err := filepath.Abs("testdata/codex_debug_models.json")
	require.NoError(t, err)
	d := codexShapedDescriptor(fixture)

	got, err := modeldiscovery.Probe(context.Background(), d, models.ProbeOptions{Cwd: t.TempDir()}, nil)

	require.NoError(t, err)
	for _, m := range got {
		assert.False(t, m.Default, "model %q: no field in this capture states a default", m.ID)
	}
}

// TestProbe_DefaultWhenFlagsTheStatedRow proves default_when works for a
// source that DOES state a default (a manifest's isDefault: true, e.g.
// model/list's own shape) — implemented even though debug models never
// exercises it, per the task's own requirement.
func TestProbe_DefaultWhenFlagsTheStatedRow(t *testing.T) {
	d := &spec.Descriptor{ID: "manifest-shaped"}
	d.Spawn.Cmd = "echo"
	d.Model = &spec.ModelSpec{Discover: &spec.ModelDiscoverSpec{
		Command: []string{
			`{"models":[{"slug":"a","display_name":"A","isDefault":false},` +
				`{"slug":"b","display_name":"B","isDefault":true}]}`,
		},
		Adapter:     spec.ModelDiscoverAdapterJSON,
		ItemsPath:   "models[]",
		DefaultWhen: &spec.ModelFieldMatch{Field: "isDefault", Equals: true},
		Item:        spec.ModelItemMapping{ID: "{slug}", Label: "{display_name}"},
	}}

	got, err := modeldiscovery.Probe(context.Background(), d, models.ProbeOptions{Cwd: t.TempDir()}, nil)

	require.NoError(t, err)
	require.Len(t, got, 2)
	byID := map[string]modeldiscovery.Model{}
	for _, m := range got {
		byID[m.ID] = m
	}
	assert.False(t, byID["a"].Default)
	assert.True(t, byID["b"].Default)
}

func TestProbe_UnsupportedWhenNoDiscoverBlockDeclared(t *testing.T) {
	d := &spec.Descriptor{ID: "no-discovery"}

	_, err := modeldiscovery.Probe(context.Background(), d, models.ProbeOptions{Cwd: t.TempDir()}, nil)

	assert.ErrorIs(t, err, modeldiscovery.ErrUnsupported)
}

func TestProbe_MalformedOutputIsReported(t *testing.T) {
	d := &spec.Descriptor{ID: "bad-output"}
	d.Spawn.Cmd = "echo"
	d.Model = &spec.ModelSpec{Discover: &spec.ModelDiscoverSpec{
		Command:   []string{"not json"},
		Adapter:   spec.ModelDiscoverAdapterJSON,
		ItemsPath: "models[]",
		Item:      spec.ModelItemMapping{ID: "{slug}", Label: "{slug}"},
	}}

	_, err := modeldiscovery.Probe(context.Background(), d, models.ProbeOptions{Cwd: t.TempDir()}, nil)

	assert.ErrorIs(t, err, modeldiscovery.ErrMalformedOutput)
}

func TestProbe_RowsMissingIDOrLabelAreDropped(t *testing.T) {
	d := &spec.Descriptor{ID: "sparse"}
	d.Spawn.Cmd = "echo"
	d.Model = &spec.ModelSpec{Discover: &spec.ModelDiscoverSpec{
		Command:   []string{`{"models":[{"slug":"a","display_name":"A"},{"display_name":"no id"}]}`},
		Adapter:   spec.ModelDiscoverAdapterJSON,
		ItemsPath: "models[]",
		Item:      spec.ModelItemMapping{ID: "{slug}", Label: "{display_name}"},
	}}

	got, err := modeldiscovery.Probe(context.Background(), d, models.ProbeOptions{Cwd: t.TempDir()}, nil)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].ID)
}

// apiCatalogDescriptor is a websocket provider whose catalogue is one named call, over
// a fake endpoint that records every method it is asked and answers from replies.
func apiCatalogDescriptor(t *testing.T, replies map[string]string) (*spec.Descriptor, <-chan string) {
	t.Helper()
	ws := kit.NewFakeWS(t)
	endpoint := filepath.Join(t.TempDir(), "endpoint.json")
	body := `{"url":"` + ws.URL(ws.NewTicket()) + `"}`
	require.NoError(t, os.WriteFile(endpoint, []byte(body), 0o600))

	methods := make(chan string, 16)
	go func() {
		for frame := range ws.Frames {
			var req struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if json.Unmarshal(frame, &req) != nil {
				continue
			}
			methods <- req.Method
			result := replies[req.Method]
			if result == "" {
				result = "{}"
			}
			_ = ws.Send([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":` + result + `}`))
		}
	}()

	d := &spec.Descriptor{ID: "api-shaped"}
	d.Runtime.Transport = "api"
	d.Runtime.API = spec.APISpec{
		Delivery: spec.APIDeliveryWebsocket, Connect: []string{"cat", endpoint},
		ConnectMap: spec.ConnectMap{URL: "url"}, Handshake: spec.HandshakeSpec{Call: "hello"},
	}
	d.Spawn.Cmd = "cat"
	d.Model = &spec.ModelSpec{Discover: &spec.ModelDiscoverSpec{
		API:       &spec.ModelDiscoverAPI{Call: "catalog.list", Params: map[string]any{"fresh": true}},
		Adapter:   spec.ModelDiscoverAdapterJSON,
		ItemsPath: "groups[]",
		KeepWhen:  &spec.ModelFieldMatch{Field: "ready", Equals: true},
		Item:      spec.ModelItemMapping{Each: "items[]", ID: "{parent.key}:{value}", Label: "{value}"},
	}}
	return d, methods
}

func TestProbe_APICallMapsNestedRowsUnderTheirParent(t *testing.T) {
	d, methods := apiCatalogDescriptor(t, map[string]string{
		"catalog.list": `{"groups":[{"key":"alpha","ready":true,"items":["m1","m2"]},` +
			`{"key":"beta","ready":false,"items":["m3"]}]}`,
	})

	got, err := modeldiscovery.Probe(context.Background(), d, models.ProbeOptions{Cwd: t.TempDir()}, nil)

	require.NoError(t, err)
	ids := make([]string, len(got))
	for i, m := range got {
		ids[i] = m.ID
	}
	assert.Equal(t, []string{"alpha:m1", "alpha:m2"}, ids, "the not-ready group is dropped, ids carry the parent key")
	assert.Equal(t, "hello", <-methods, "the handshake runs first")
	assert.Equal(t, "catalog.list", <-methods)
	assert.Empty(t, methods, "the handshake and the one call are the only traffic: no session is opened")
}

func TestProbe_APICallFailureIsReported(t *testing.T) {
	d, _ := apiCatalogDescriptor(t, nil)
	d.Runtime.API.Connect = []string{"cat", filepath.Join(t.TempDir(), "missing.json")}

	_, err := modeldiscovery.Probe(context.Background(), d, models.ProbeOptions{Cwd: t.TempDir()}, nil)

	assert.Error(t, err)
}

func TestProbe_CurrentIDFlagsTheRepliesOwnSelection(t *testing.T) {
	reply := `{"cur":"alpha","sel":"m2","groups":[{"key":"alpha","ready":true,"items":["m1","m2"]}]}`
	cases := map[string]struct {
		reply string
		want  string
	}{
		"the composed id matching a row": {reply, "alpha:m2"},
		"a reply stating no selection":   {`{"cur":"","sel":"","groups":[{"key":"alpha","ready":true,"items":["m1"]}]}`, ""},
		"a selection no row offers":      {`{"cur":"alpha","sel":"gone","groups":[{"key":"alpha","ready":true,"items":["m1"]}]}`, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d, _ := apiCatalogDescriptor(t, map[string]string{"catalog.list": tc.reply})
			d.Model.Discover.CurrentID = "{cur}:{sel}"

			got, err := modeldiscovery.Probe(context.Background(), d, models.ProbeOptions{Cwd: t.TempDir()}, nil)

			require.NoError(t, err)
			current := ""
			for _, m := range got {
				if m.Default {
					current = m.ID
				}
			}
			assert.Equal(t, tc.want, current)
		})
	}
}
