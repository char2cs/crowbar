package modeldiscovery_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRegression_ModelDiscoveryGoSourceCarriesNoProviderWireVocabulary is the
// grep-style backstop for this feature's own hard law (mirrors
// descriptor_test.go's TestRegression_NoShippedDescriptorContainsThe
// AlternationGlyph): model.discover's whole point is that Go implements a
// GENERIC "run a command, map its JSON" primitive, and every one of these
// words — codex's own name, and codex's OWN wire field names for its model
// list — belongs only in codex.yaml as DATA (a template placeholder or a
// field path), never as a Go identifier or string literal. Scoped to the
// files this feature actually touches: "priority", "visibility" etc. are
// ordinary English words that legitimately appear elsewhere in this
// codebase (e.g. domain.AgentProviderPreference.Priority) for reasons that
// have nothing to do with codex's model wire shape, so scanning the whole
// tree would false-positive on code this feature never touched.
//
// The file list below is every file this feature's GENERIC primitive lives
// in — the ones whose entire content this feature wrote. It deliberately
// excludes files this feature only ADDED a field or a call to (agents.go,
// domain/agent_provider.go, provider.go, dto/agent.go, and the rest): those
// carry substantial pre-existing prose from unrelated work that legitimately
// uses these ordinary English words (e.g. "codex" as a worked example in a
// comment written for a different feature entirely, or
// AgentProviderPreference.Priority, a field this task never touched) — a
// whole-file scan of those would be a standing false-positive trap, not a
// meaningful check on this feature's own code.
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

// apiModuleRoot walks up from this test file's own directory to the api
// module's go.mod, so the file list above resolves regardless of `go test`'s
// working directory.
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
