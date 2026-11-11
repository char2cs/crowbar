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

const shippedDir = "../protocol/internal/descriptor/descriptors-v3"

func TestVersion_AnUnstampedDocumentIsCurrent(t *testing.T) {
	for _, doc := range []string{"id: x\n", "id: [x\n"} {
		v, _, err := descriptorschema.Version([]byte(doc))
		require.NoError(t, err)
		assert.Equal(t, descriptorschema.Current, v, doc)
	}
	v, line, err := descriptorschema.Version([]byte("id: x\nschema_version: 3\n"))
	require.NoError(t, err)
	assert.Equal(t, [2]int{3, 2}, [2]int{v, line})
	_, _, err = descriptorschema.Version([]byte("id: x\nschema_version: next\n"))
	require.ErrorIs(t, err, descriptorschema.ErrUnsupported)
}

// The descriptor checks are generic: no provider a descriptor names appears in
// their code, so a user's own descriptor is held to exactly the same rules.
func TestVersion_TheDescriptorChecksNameNoProvider(t *testing.T) {
	names := providerNames(t)
	require.NotEmpty(t, names)
	var sources []string
	for _, pattern := range []string{"*.go", "../../descriptorcheck/*.go", "../protocol/internal/descriptor/version.go"} {
		files, err := filepath.Glob(pattern)
		require.NoError(t, err)
		for _, f := range files {
			if !strings.HasSuffix(f, "_test.go") {
				sources = append(sources, f)
			}
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

// providerNames are the id, command and display name of every shipped descriptor.
func providerNames(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(shippedDir, "*.yaml"))
	require.NoError(t, err)
	var out []string
	for _, f := range files {
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
				out = append(out, n)
			}
		}
	}
	return out
}
