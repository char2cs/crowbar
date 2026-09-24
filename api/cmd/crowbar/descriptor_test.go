package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

func runDescriptor(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newDescriptorCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestDescriptorValidate_TheShippedDescriptorsPass(t *testing.T) {
	t.Setenv(metadata.HomeEnvVar, t.TempDir())

	out, err := runDescriptor(t, "validate")

	require.NoError(t, err)
	assert.Contains(t, out, "claude (shipped): ok")
	assert.Contains(t, out, "codex (shipped): ok")
}

func TestDescriptorValidate_ABrokenFileFailsWithItsFindings(t *testing.T) {
	file := filepath.Join(t.TempDir(), "broken.yaml")
	require.NoError(t, os.WriteFile(file, []byte("id: broken\nspawn:\n  cmd: broken\n"), 0o600))

	out, err := runDescriptor(t, "validate", file)

	require.ErrorIs(t, err, errDescriptorBlocked)
	assert.Contains(t, out, "broken ("+file+"): BLOCKED")
	assert.Contains(t, out, "error load.spawn_command at spawn (line 2)")
	assert.Contains(t, out, "hint:")
}

func TestDescriptorValidate_PrintsJSON(t *testing.T) {
	t.Setenv(metadata.HomeEnvVar, t.TempDir())

	out, err := runDescriptor(t, "validate", "--json")

	require.NoError(t, err)
	var reports []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &reports))
	assert.Len(t, reports, 2)
}

// Without --live, test validates the installed descriptor by provider id.
func TestDescriptorTest_WithoutLiveValidatesByProviderID(t *testing.T) {
	t.Setenv(metadata.HomeEnvVar, t.TempDir())

	out, err := runDescriptor(t, "test", "codex")

	require.NoError(t, err)
	assert.Contains(t, out, "codex (shipped): ok")
}

func TestDescriptorTest_AnUnknownProviderIsAnError(t *testing.T) {
	t.Setenv(metadata.HomeEnvVar, t.TempDir())

	_, err := runDescriptor(t, "test", "nope")

	require.ErrorContains(t, err, `no provider "nope"`)
}

// --live on a descriptor with a static error runs nothing live.
func TestDescriptorTest_LiveStopsAtAStaticError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "broken.yaml")
	require.NoError(t, os.WriteFile(file, []byte("id: broken\nspawn:\n  cmd: broken\n"), 0o600))

	out, err := runDescriptor(t, "test", file, "--live", "--json")

	require.ErrorIs(t, err, errDescriptorBlocked)
	var rep struct {
		Steps []struct {
			Name string `json:"name"`
		} `json:"steps"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &rep))
	require.Len(t, rep.Steps, 1)
	assert.Equal(t, "static", rep.Steps[0].Name)
}
