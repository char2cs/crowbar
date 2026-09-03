package catalog_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/catalog"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return data
}

type scriptRunner struct {
	mu      sync.Mutex
	replies map[string][]byte
	errs    map[string]error
	calls   []string
}

func (r *scriptRunner) Run(_ context.Context, argv []string) ([]byte, error) {
	key := strings.Join(argv, " ")
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, key)
	if err, ok := r.errs[key]; ok {
		return nil, err
	}
	if out, ok := r.replies[key]; ok {
		return out, nil
	}
	return nil, exec.ErrCommandFailed
}

func inventoryDescriptor(t *testing.T) *spec.Descriptor {
	t.Helper()
	d := &spec.Descriptor{ID: "claude"}
	d.Spawn.Cmd = "claude"
	d.Spawn.InteractiveRequired = true
	d.Runtime.Hooks.Format = "json"
	d.Presentation.SlashCatalog = &spec.SlashCatalogSpec{
		Completeness: spec.CatalogCompletenessPluginOnly,
		Pipeline: spec.CatalogPipelineSpec{
			Adapter:            spec.CatalogAdapterJSONInventoryDetails,
			Command:            []string{"plugin", "list", "--json"},
			RowsPath:           "[]",
			EnabledField:       "enabled",
			IDField:            "id",
			SourcePattern:      `^(?P<source>[^@]+)`,
			DetailCommand:      []string{"plugin", "details", "{id}"},
			DetailPattern:      `(?m)^[^\S\r\n]*Skills[^\S\r\n]+\(\d+\):?[^\S\r\n]+(?P<items>[^\r\n]+?)[^\S\r\n]*$`,
			DetailEmptyPattern: `(?m)^[^\S\r\n]*Skills[^\S\r\n]+\(0\)[^\S\r\n]*:?[^\S\r\n]*$`,
			DetailItemsGroup:   "items",
			DetailSeparator:    ",",
			Item: spec.CatalogItemMapping{
				Label: "{source}:{name}", InsertText: "/{source}:{name} ", Source: "{source}",
			},
		},
	}
	return d
}

func textSectionDescriptor(t *testing.T) *spec.Descriptor {
	t.Helper()
	d := &spec.Descriptor{ID: "codex"}
	d.Spawn.Cmd = "codex"
	d.Spawn.InteractiveRequired = true
	d.Runtime.Hooks.Format = "json"
	d.Presentation.SlashCatalog = &spec.SlashCatalogSpec{
		Completeness: spec.CatalogCompletenessModelVisible,
		Pipeline: spec.CatalogPipelineSpec{
			Adapter:     spec.CatalogAdapterJSONTextSection,
			Command:     []string{"debug", "prompt-input"},
			TextPath:    "[].content[].text",
			StartMarker: "<skills_instructions>",
			EndMarker:   "</skills_instructions>",
			ItemPattern: `(?m)^- (?P<name>[^\r\n]+?):\s+(?P<description>.*?)(?:\s+\((?:file|orchestrator resource|environment resource|custom resource):[^\r\n]*\))?\r?$`,
			Item: spec.CatalogItemMapping{
				Label: "{name}", Description: "{description}", InsertText: "${name} ", Source: "model-visible",
			},
		},
	}
	return d
}

func TestProbe_InventoryFanOutProducesNormalisedItems(t *testing.T) {
	d := inventoryDescriptor(t)
	runner := &scriptRunner{replies: map[string][]byte{
		"plugin list --json": fixture(t, "claude_plugin_list.json"),
		"plugin details superpowers@claude-plugins-official": fixture(t, "claude_plugin_details_superpowers.txt"),
		"plugin details empty-kit@claude-plugins-official":   fixture(t, "claude_plugin_details_empty.txt"),
	}}

	got, err := catalog.ProbeWith(context.Background(), d, runner)

	require.NoError(t, err)
	labels := labelsOf(got)
	assert.Equal(t, []string{
		"superpowers:brainstorming",
		"superpowers:systematic-debugging",
		"superpowers:writing-plans",
	}, labels)
	assert.Equal(t, "claude", got.ProviderID)
	assert.Equal(t, string(spec.CatalogCompletenessPluginOnly), got.Completeness)
	assert.NotEmpty(t, got.Warnings, "a plugin-only inventory must say so")
	runner.mu.Lock()
	calls := append([]string(nil), runner.calls...)
	runner.mu.Unlock()
	assert.NotContains(t, calls, "plugin details disabled-kit@claude-plugins-official",
		"a disabled plugin must not be inspected")
}

func TestProbe_APluginWithNoSkillsContributesNothingAndNoWarning(t *testing.T) {
	d := inventoryDescriptor(t)
	runner := &scriptRunner{replies: map[string][]byte{
		"plugin list --json":         []byte(`[{"id":"empty-kit@x","enabled":true}]`),
		"plugin details empty-kit@x": fixture(t, "claude_plugin_details_empty.txt"),
	}}

	got, err := catalog.ProbeWith(context.Background(), d, runner)

	require.NoError(t, err)
	assert.Empty(t, got.Items)
	for _, w := range got.Warnings {
		assert.NotContains(t, w, "unrecognized")
	}
}

func TestProbe_TextSectionReadsAMarkedBlock(t *testing.T) {
	d := textSectionDescriptor(t)
	runner := &scriptRunner{replies: map[string][]byte{
		"debug prompt-input": fixture(t, "codex_prompt_input.json"),
	}}

	got, err := catalog.ProbeWith(context.Background(), d, runner)

	require.NoError(t, err)
	require.NotEmpty(t, got.Items)
	assert.Contains(t, labelsOf(got), "imagegen")
	for _, item := range got.Items {
		assert.NotContains(t, item.Description, "/Users/",
			"a filesystem location must never reach a published catalogue")
	}
}

func TestProbe_TextSectionWithNoSectionIsMalformed(t *testing.T) {
	d := textSectionDescriptor(t)
	runner := &scriptRunner{replies: map[string][]byte{
		"debug prompt-input": []byte(`[{"content":[{"text":"nothing here"}]}]`),
	}}

	_, err := catalog.ProbeWith(context.Background(), d, runner)

	assert.ErrorIs(t, err, catalog.ErrMalformedOutput)
}

func TestProbe_NonJSONOutputIsMalformed(t *testing.T) {
	for _, tc := range []struct {
		name string
		d    *spec.Descriptor
		key  string
	}{
		{"text section", textSectionDescriptor(t), "debug prompt-input"},
		{"inventory", inventoryDescriptor(t), "plugin list --json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &scriptRunner{replies: map[string][]byte{tc.key: []byte("not json")}}

			_, err := catalog.ProbeWith(context.Background(), tc.d, runner)

			assert.ErrorIs(t, err, catalog.ErrMalformedOutput)
		})
	}
}

func TestProbe_InventoryRowsThatAreNotObjectsAreMalformed(t *testing.T) {
	d := inventoryDescriptor(t)
	runner := &scriptRunner{replies: map[string][]byte{
		"plugin list --json": []byte(`["just","strings"]`),
	}}

	_, err := catalog.ProbeWith(context.Background(), d, runner)

	assert.ErrorIs(t, err, catalog.ErrMalformedOutput)
}

func TestProbe_AnEmptyInventoryIsAnEmptyMenu(t *testing.T) {
	d := inventoryDescriptor(t)
	runner := &scriptRunner{replies: map[string][]byte{"plugin list --json": []byte(`[]`)}}

	got, err := catalog.ProbeWith(context.Background(), d, runner)

	require.NoError(t, err)
	assert.Empty(t, got.Items)
}

func TestProbe_MalformedRowsAreOmittedWithAWarning(t *testing.T) {
	d := inventoryDescriptor(t)
	runner := &scriptRunner{replies: map[string][]byte{
		"plugin list --json": []byte(`[{"id":"-dashy@x","enabled":true},{"id":"","enabled":true},"nope"]`),
	}}

	got, err := catalog.ProbeWith(context.Background(), d, runner)

	require.NoError(t, err)
	assert.Empty(t, got.Items)
	assert.Contains(t, strings.Join(got.Warnings, "|"), "malformed provider inventory rows")
}

func TestProbe_ADetailFailureDegradesToAWarning(t *testing.T) {
	d := inventoryDescriptor(t)
	runner := &scriptRunner{
		replies: map[string][]byte{
			"plugin list --json": []byte(`[{"id":"a@x","enabled":true},{"id":"b@x","enabled":true}]`),
			"plugin details b@x": fixture(t, "claude_plugin_details_superpowers.txt"),
		},
		errs: map[string]error{"plugin details a@x": exec.ErrCommandFailed},
	}

	got, err := catalog.ProbeWith(context.Background(), d, runner)

	require.NoError(t, err)
	assert.NotEmpty(t, got.Items, "the readable plugin still contributes")
	assert.Contains(t, strings.Join(got.Warnings, "|"), "could not be inspected")
}

func TestProbe_AWholeProbeFailurePropagates(t *testing.T) {
	for _, sentinel := range []error{exec.ErrOutputLimit, exec.ErrCommandUnavailable} {
		d := inventoryDescriptor(t)
		runner := &scriptRunner{
			replies: map[string][]byte{"plugin list --json": []byte(`[{"id":"a@x","enabled":true}]`)},
			errs:    map[string]error{"plugin details a@x": sentinel},
		}

		_, err := catalog.ProbeWith(context.Background(), d, runner)

		assert.ErrorIs(t, err, sentinel)
	}
}

func TestProbe_AnUnrecognisedDetailShapeIsAWarningNotAFailure(t *testing.T) {
	d := inventoryDescriptor(t)
	runner := &scriptRunner{replies: map[string][]byte{
		"plugin list --json": []byte(`[{"id":"a@x","enabled":true}]`),
		"plugin details a@x": []byte("something else entirely"),
	}}

	got, err := catalog.ProbeWith(context.Background(), d, runner)

	require.NoError(t, err)
	assert.Contains(t, strings.Join(got.Warnings, "|"), "unrecognized component inventory")
}

func TestProbe_InventoryCommandFailurePropagates(t *testing.T) {
	d := inventoryDescriptor(t)
	runner := &scriptRunner{errs: map[string]error{"plugin list --json": exec.ErrCommandFailed}}

	_, err := catalog.ProbeWith(context.Background(), d, runner)

	assert.ErrorIs(t, err, exec.ErrCommandFailed)
}

func TestProbe_DeduplicatesOnSourceAndInsertText(t *testing.T) {
	d := inventoryDescriptor(t)
	runner := &scriptRunner{replies: map[string][]byte{
		"plugin list --json": []byte(`[{"id":"a@x","enabled":true}]`),
		"plugin details a@x": []byte("Component inventory\n  Skills (2)  dup, dup\n"),
	}}

	got, err := catalog.ProbeWith(context.Background(), d, runner)

	require.NoError(t, err)
	assert.Len(t, got.Items, 1)
}

func TestProbe_TruncatesToTheItemCeilingAndSaysSo(t *testing.T) {
	d := inventoryDescriptor(t)
	d.Presentation.SlashCatalog.MaxItems = 2
	runner := &scriptRunner{replies: map[string][]byte{
		"plugin list --json": []byte(`[{"id":"a@x","enabled":true}]`),
		"plugin details a@x": []byte("Component inventory\n  Skills (4)  one, two, three, four\n"),
	}}

	got, err := catalog.ProbeWith(context.Background(), d, runner)

	require.NoError(t, err)
	assert.Len(t, got.Items, 2)
	assert.Contains(t, strings.Join(got.Warnings, "|"), "truncated")
}

func TestProbe_EveryItemGetsAStableIdentity(t *testing.T) {
	d := inventoryDescriptor(t)
	runner := &scriptRunner{replies: map[string][]byte{
		"plugin list --json": []byte(`[{"id":"a@x","enabled":true}]`),
		"plugin details a@x": []byte("Component inventory\n  Skills (1)  only\n"),
	}}

	first, err := catalog.ProbeWith(context.Background(), d, runner)
	require.NoError(t, err)
	second, err := catalog.ProbeWith(context.Background(), d, runner)
	require.NoError(t, err)

	require.Len(t, first.Items, 1)
	assert.NotEmpty(t, first.Items[0].ID)
	assert.Equal(t, first.Items[0].ID, second.Items[0].ID,
		"the same item must keep its identity across probes")
	assert.Equal(t, models.CatalogItemKindSkill, first.Items[0].Kind)
}

func TestProbe_UnsupportedWhenNoCatalogueIsDeclared(t *testing.T) {
	_, err := catalog.ProbeWith(context.Background(), &spec.Descriptor{ID: "x"}, &scriptRunner{})
	assert.ErrorIs(t, err, catalog.ErrUnsupported)

	_, err = catalog.ProbeWith(context.Background(), nil, &scriptRunner{})
	assert.ErrorIs(t, err, catalog.ErrUnsupported)
}

func TestProbe_MalformedWhenTheAdapterIsUnknown(t *testing.T) {
	d := inventoryDescriptor(t)
	d.Presentation.SlashCatalog.Pipeline.Adapter = "telepathy"

	_, err := catalog.ProbeWith(context.Background(), d, &scriptRunner{})

	assert.ErrorIs(t, err, catalog.ErrMalformedOutput)
}

func TestProbe_RefusesAWorkdirThatIsNotAnAbsoluteExistingDirectory(t *testing.T) {
	d := inventoryDescriptor(t)
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	for _, cwd := range []string{"", "relative/path", filepath.Join(t.TempDir(), "missing"), file} {
		_, err := catalog.Probe(context.Background(), d, models.ProbeOptions{Cwd: cwd}, nil)
		assert.ErrorIs(t, err, catalog.ErrInvalidWorkdir, "cwd %q", cwd)
	}
}

func TestProbe_SeparatesItsOwnDeadlineFromCallerCancellation(t *testing.T) {
	d := inventoryDescriptor(t)
	d.Presentation.SlashCatalog.TimeoutMS = 20
	slow := runnerFunc(func(ctx context.Context, _ []string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	_, err := catalog.ProbeWith(context.Background(), d, slow)
	assert.ErrorIs(t, err, catalog.ErrTimeout)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = catalog.ProbeWith(cancelled, d, slow)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestProbe_RealRunnerReachesTheProviderCommand(t *testing.T) {
	d := inventoryDescriptor(t)
	d.Spawn.Cmd = "true"
	dir := t.TempDir()

	_, err := catalog.Probe(context.Background(), d,
		models.ProbeOptions{Cwd: dir, Env: []string{"PATH=" + os.Getenv("PATH")}}, nil)

	assert.ErrorIs(t, err, catalog.ErrMalformedOutput)
}

func TestProbe_AcquireIsHonouredByTheRealRunner(t *testing.T) {
	d := inventoryDescriptor(t)
	d.Spawn.Cmd = "true"
	held := 0
	acquire := func(context.Context) (func(), error) {
		held++
		return func() {}, nil
	}

	_, _ = catalog.Probe(context.Background(), d,
		models.ProbeOptions{Cwd: t.TempDir(), Env: []string{"PATH=" + os.Getenv("PATH")}}, acquire)

	assert.Positive(t, held, "every provider command must take the shared budget")
}

type runnerFunc func(context.Context, []string) ([]byte, error)

func (f runnerFunc) Run(ctx context.Context, argv []string) ([]byte, error) { return f(ctx, argv) }

func labelsOf(c models.SlashCatalog) []string {
	out := make([]string, 0, len(c.Items))
	for _, i := range c.Items {
		out = append(out, i.Label)
	}
	return out
}

var (
	_ = errors.Is
	_ = time.Second
)
