package descriptorschema_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptorschema"
)

const (
	shippedDir = "../protocol/internal/descriptor/descriptors-v3"
	historyDir = "../../descriptorcheck/testdata/history"
)

func upgrade(t *testing.T, raw string) descriptorschema.Result {
	t.Helper()
	up, err := descriptorschema.Upgrade([]byte(raw))
	require.NoError(t, err)
	return up
}

func decode(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &out))
	return out
}

// A shipped descriptor is in today's shape: with its stamp taken away it is
// read as the oldest schema, and still no transition finds anything to do.
func TestUpgrade_ATodayShapedDocumentIsLeftByteForByte(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(shippedDir, "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		require.NoError(t, err)
		up := upgrade(t, string(raw))
		assert.Equal(t, descriptorschema.Current, up.From, "%s states the current schema", f)
		assert.Empty(t, up.Changes, f)

		unstamped := strings.Replace(string(raw), "\nschema_version: 3\n", "\n", 1)
		require.NotEqual(t, string(raw), unstamped, "%s is stamped", f)
		up = upgrade(t, unstamped)
		assert.Empty(t, up.Changes, f)
		assert.Equal(t, unstamped, string(up.Raw))
	}
}

func TestUpgrade_ADocumentFromANewerSchemaIsRefusedWithBoth(t *testing.T) {
	_, err := descriptorschema.Upgrade([]byte("id: x\nschema_version: 9\n"))
	require.ErrorIs(t, err, descriptorschema.ErrNewer)
	assert.Contains(t, err.Error(), "9")
	assert.Contains(t, err.Error(), "3")
}

func TestUpgrade_StampsTheCurrentSchemaAfterTheID(t *testing.T) {
	up := upgrade(t, "# mine\nid: x\nsession:\n  locate: { root: \"~/x\" }\n")
	assert.Equal(t, "# mine\nid: x\nschema_version: 3\nsession: {}\n", string(up.Raw))
	again := upgrade(t, string(up.Raw))
	assert.Empty(t, again.Changes, "a second upgrade finds nothing")
}

// An old field map keeps every branch it had, in order, under the field's
// current name: the meaning of each mapping survives the upgrade.
func TestUpgrade_EveryHistoricFieldMapKeepsItsBranches(t *testing.T) {
	renamed := map[string][]string{"subagent_id": {"scope_id"}, "nested_session_id": {"spawns_scope", "addresses_scope"}}
	files, err := filepath.Glob(filepath.Join(historyDir, "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		require.NoError(t, err)
		up := upgrade(t, string(raw))
		before, after := fieldBranches(decode(t, raw)), fieldBranches(decode(t, up.Raw))
		for key, branches := range before {
			event, field := key[0], key[1]
			names := renamed[field]
			if names == nil {
				names = []string{field}
			}
			for _, name := range names {
				if field == "async_work" {
					continue
				}
				assert.Equal(t, branches, after[[2]string{event, name}], "%s: %s.%s", filepath.Base(f), event, name)
			}
		}
	}
}

// The user's comments are theirs: every one in every historic document is
// still in it once upgraded.
func TestUpgrade_EveryCommentSurvives(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(historyDir, "*.yaml"))
	require.NoError(t, err)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		require.NoError(t, err)
		after := commentLines(string(upgrade(t, string(raw)).Raw))
		for line := range commentLines(string(raw)) {
			assert.True(t, after[line], "%s lost %q", filepath.Base(f), line)
		}
	}
}

func commentLines(doc string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(doc, "\n") {
		if _, c, ok := strings.Cut(line, "#"); ok && strings.HasPrefix(strings.TrimSpace(line), "#") {
			out[strings.TrimSpace(c)] = true
		}
	}
	return out
}

// fieldBranches is, per event map and field, the source paths it tries in order.
func fieldBranches(doc map[string]any) map[[2]string][]string {
	out := map[[2]string][]string{}
	events, _ := doc["events"].(map[string]any)
	for name, raw := range events {
		event, _ := raw.(map[string]any)
		for scope, block := range map[string]any{name: event, name + ".api": event["api"], name + ".hooks": event["hooks"]} {
			b, _ := block.(map[string]any)
			fields, _ := b["map"].(map[string]any)
			for field, v := range fields {
				out[[2]string{scope, field}] = branches(v)
			}
		}
	}
	return out
}

func branches(v any) []string {
	switch x := v.(type) {
	case string:
		parts := strings.Split(x, "||")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	case map[string]any:
		if list, ok := x["first_present"].([]any); ok {
			var out []string
			for _, item := range list {
				out = append(out, branches(item)...)
			}
			return out
		}
	}
	return []string{yamlString(v)}
}

func yamlString(v any) string {
	b, _ := yaml.Marshal(v)
	return strings.TrimSpace(string(b))
}

// The upgrade is schema-driven: no provider a descriptor names appears in its
// code, so every descriptor, shipped or a user's own, upgrades the same way.
func TestUpgrade_ItsCodeNamesNoProvider(t *testing.T) {
	names := providerNames(t)
	require.NotEmpty(t, names)
	sources := []string{"../../../../adapter/migrations/descriptor_schema.go"}
	own, err := filepath.Glob("*.go")
	require.NoError(t, err)
	for _, f := range own {
		if !strings.HasSuffix(f, "_test.go") {
			sources = append(sources, f)
		}
	}
	for _, f := range sources {
		raw, err := os.ReadFile(f)
		require.NoError(t, err)
		for _, name := range names {
			re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(name) + `\b`)
			assert.False(t, re.Match(raw), "%s names the provider %q", f, name)
		}
	}
}

// providerNames are the id, command and display name of every descriptor the
// repo ships or has shipped.
func providerNames(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(shippedDir, "*.yaml"))
	require.NoError(t, err)
	history, err := filepath.Glob(filepath.Join(historyDir, "*.yaml"))
	require.NoError(t, err)
	seen := map[string]bool{}
	for _, f := range append(files, history...) {
		raw, err := os.ReadFile(f)
		require.NoError(t, err)
		var d struct {
			ID      string `yaml:"id"`
			Display string `yaml:"display_name"`
			Spawn   struct {
				Cmd string `yaml:"cmd"`
			} `yaml:"spawn"`
		}
		require.NoError(t, yaml.Unmarshal(raw, &d))
		for _, n := range []string{d.ID, d.Display, d.Spawn.Cmd} {
			if n != "" {
				seen[n] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	return out
}
