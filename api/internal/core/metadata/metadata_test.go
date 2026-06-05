package metadata

import (
	"runtime"
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

// --- New tests for coverage ---

func TestGetEventsPath_NonEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(resetForTesting)
	resetForTesting()
	got := GetEventsPath()
	assert.NotEmpty(t, got)
}

func TestGetStorePath_NonEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(resetForTesting)
	resetForTesting()
	got := GetStorePath()
	assert.NotEmpty(t, got)
}

func TestGetRunsPath_NonEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(resetForTesting)
	resetForTesting()
	got := GetRunsPath()
	assert.NotEmpty(t, got)
}

func TestGetLogsPath_NonEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(resetForTesting)
	resetForTesting()
	got := GetLogsPath()
	assert.NotEmpty(t, got)
}

func TestDefaultMetadata_Name(t *testing.T) {
	m := defaultMetadata()
	assert.Equal(t, "Crowbar", m.Metadata.Name)
	assert.NotEmpty(t, m.Paths.Events)
	assert.NotEmpty(t, m.Paths.Store)
	assert.NotEmpty(t, m.Paths.Runs)
	assert.NotEmpty(t, m.Paths.Logs)
}

func TestOsValue_Resolve_OSHit(t *testing.T) {
	v := OsValue[string]{Default: "default-val", OS: map[string]string{runtime.GOOS: "os-val"}}
	assert.Equal(t, "os-val", v.Resolve())
}

func TestOsValue_Resolve_NilMap(t *testing.T) {
	v := OsValue[string]{Default: "only-default"}
	assert.Equal(t, "only-default", v.Resolve())
}

func TestResolveHome_TildeExpansion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Cleanup(resetForTesting)
	resetForTesting()
	// Default home path starts with "~/.crowbar"; resolveHome should expand it.
	got := resolveHome()
	assert.True(t, strings.HasPrefix(got, tmp), "expected %q to start with %q", got, tmp)
}

func TestResolveHome_NonTilde(t *testing.T) {
	t.Cleanup(resetForTesting)
	resetForTesting()
	// Force the singleton to a non-tilde home so the non-~ branch executes.
	m := defaultMetadata()
	m.Paths.Home = OsValue[string]{Default: "/absolute/path"}
	metadata = m
	once.Do(func() {}) // mark once as done so Get() returns our metadata
	got := resolveHome()
	assert.Equal(t, "/absolute/path", got)
}
