package metadata

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGet_LoadsEmbeddedMetadata(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	m := Get()
	assert.Equal(t, "Crowbar", m.Metadata.Name)
	assert.Equal(t, "0.1.0", m.Version.Number)
}

func TestGetEventsPathAt_RootsAtHomeDir(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	got := GetEventsPathAt("/tmp/crowtest")
	assert.True(t, strings.HasPrefix(got, "/tmp/crowtest"))
	assert.True(t, strings.HasSuffix(got, "state/events"))
}

func TestGetRunsPathAt_RootsAtHomeDir(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	got := GetRunsPathAt("/tmp/crowtest")
	assert.Equal(t, "/tmp/crowtest/runs", got)
}

func TestOsValue_Resolve_FallsBackToDefault(t *testing.T) {
	v := OsValue[string]{Default: "d", OS: map[string]string{"plan9": "x"}}
	assert.Equal(t, "d", v.Resolve())
}

func TestGetVersion_NonEmpty(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	assert.NotEmpty(t, GetVersion())
}
