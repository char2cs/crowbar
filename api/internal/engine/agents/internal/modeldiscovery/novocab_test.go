package modeldiscovery_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegression_ModelDiscoveryGoSourceCarriesNoProviderWireVocabulary(t *testing.T) {
	root := apiModuleRoot(t)

	files := []string{
		"internal/engine/agents/internal/spec/model.go",
		"internal/engine/agents/internal/pathselect/pathselect.go",
		"internal/engine/agents/internal/modeldiscovery/modeldiscovery.go",
		"internal/engine/agents/internal/modeldiscovery/cache.go",
		"internal/engine/agents/internal/modeldiscovery/manifest.go",
		"internal/engine/agents/internal/protocol/internal/descriptor/internal/rules/model_catalog.go",
		"internal/engine/agents/internal/protocol/internal/descriptor/internal/rules/model_discover.go",
		"internal/engine/agents/internal/protocol/internal/descriptor/internal/rules/model_manifest.go",
		"internal/engine/agents/internal/protocol/internal/descriptor/internal/rules/effort_catalog.go",
		"internal/app/usecases/chat/internal/runner/promptswitch.go",
		"internal/app/usecases/chat/internal/conversation/selection.go",
	}
	forbidden := []string{
		"codex", "debug models", "slug", "display_name",
		"supported_reasoning_levels", "visibility", "priority", "default_reasoning_level",
		"claude", "anthropic", "fable", "opus", "sonnet", "haiku", "opusplan", "--effort",
	}

	for _, rel := range files {
		t.Run(rel, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, rel))
			require.NoError(t, err)
			lower := strings.ToLower(string(data))
			for _, word := range forbidden {
				if strings.Contains(lower, word) {
					t.Errorf("%s: contains provider wire-vocabulary word %q — "+
						"that vocabulary belongs only in codex.yaml as data", rel, word)
				}
			}
		})
	}
}

func TestRegression_NoGoSourceNamesAProvider(t *testing.T) {
	root := apiModuleRoot(t)
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lower := strings.ToLower(string(data))
		for _, word := range []string{"claude", "codex", "anthropic", "openai"} {
			if strings.Contains(lower, word) {
				t.Errorf("%s names the provider %q: that belongs in its descriptor", path, word)
			}
		}
		return nil
	})
	require.NoError(t, err)
}

func apiModuleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "api module go.mod not found above %s", file)
		dir = parent
	}
}
