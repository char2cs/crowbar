package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func runPaths(t *testing.T) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"paths"})
	err := root.Execute()
	return out.String(), err
}

func TestPathsCmd_PrintsHomeAndSocket(t *testing.T) {
	h := testutil.FixtureHome(t)
	t.Setenv(home.EnvVar, h.Root())

	out, err := runPaths(t)

	require.NoError(t, err)
	dec := json.NewDecoder(strings.NewReader(out))
	var got map[string]string
	require.NoError(t, dec.Decode(&got))
	assert.Equal(t, map[string]string{"home": h.Root(), "socket": h.Socket()}, got)
	require.ErrorIs(t, dec.Decode(new(any)), io.EOF)
}

func TestPathsCmd_FailsOnARelativeHome(t *testing.T) {
	t.Setenv(home.EnvVar, "rel")

	out, err := runPaths(t)

	require.ErrorContains(t, err, "absolute")
	assert.Empty(t, out)
}
