package agent_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

// mustDescriptor loads validMinimalDescriptor (from descriptor_error_test.go)
// with an extra injectYAML snippet appended, failing the test on any parse
// error so callers can go straight to exercising BuildSpawnPlan.
func mustDescriptor(t *testing.T, injectYAML string) *agent.Descriptor {
	t.Helper()
	d, err := agent.LoadDescriptor([]byte(validMinimalDescriptor + injectYAML))
	require.NoError(t, err)
	return d
}

func TestBuildSpawnPlan_UnknownInjectVerb_ReturnsError(t *testing.T) {
	d := mustDescriptor(t, "\nconfig_injection:\n  - bogus_verb: {}\n")
	ctx := agent.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarHook: "/bin/crowbar"}

	_, err := agent.BuildSpawnPlan(d, ctx, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown inject verb")
}

func TestWriteFileStep_MissingFromSource_WritesEmptyDestination(t *testing.T) {
	d := mustDescriptor(t, "\nconfig_injection:\n  - write_file: { path: \"{tmp}/dest.txt\", from: \"{tmp}/does-not-exist.txt\" }\n")
	ctx := agent.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarHook: "/bin/crowbar"}

	plan, err := agent.BuildSpawnPlan(d, ctx, nil, nil)
	require.NoError(t, err)
	defer plan.Cleanup()

	got, err := os.ReadFile(filepath.Join(ctx.Tmp, "dest.txt"))
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestWriteFileStep_ExistingFromSource_CopiesContent(t *testing.T) {
	ctx := agent.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarHook: "/bin/crowbar"}
	require.NoError(t, os.WriteFile(filepath.Join(ctx.Tmp, "source.txt"), []byte("copy-me"), 0o644))

	d := mustDescriptor(t, "\nconfig_injection:\n  - write_file: { path: \"{tmp}/dest.txt\", from: \"{tmp}/source.txt\" }\n")

	plan, err := agent.BuildSpawnPlan(d, ctx, nil, nil)
	require.NoError(t, err)
	defer plan.Cleanup()

	got, err := os.ReadFile(filepath.Join(ctx.Tmp, "dest.txt"))
	require.NoError(t, err)
	require.Equal(t, "copy-me", string(got))
}

// TestWriteFileStep_CopyFailsWhenDestDirReadOnly drives the write_file "from"
// copy path (not the tolerant-missing-source path) into a real copyFile
// failure by pre-creating the destination directory read-only.
func TestWriteFileStep_CopyFailsWhenDestDirReadOnly(t *testing.T) {
	ctx := agent.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarHook: "/bin/crowbar"}
	require.NoError(t, os.WriteFile(filepath.Join(ctx.Tmp, "source.txt"), []byte("copy-me"), 0o644))

	roDir := filepath.Join(ctx.Tmp, "ro")
	require.NoError(t, os.Mkdir(roDir, 0o555)) // r-x: MkdirAll no-ops on it, OpenFile create fails
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })

	d := mustDescriptor(t, "\nconfig_injection:\n  - write_file: { path: \""+roDir+"/dest.txt\", from: \"{tmp}/source.txt\" }\n")

	_, err := agent.BuildSpawnPlan(d, ctx, nil, nil)
	require.Error(t, err)
}

func TestRenderHooksStep_MkdirFailsWhenIntoParentIsAFile(t *testing.T) {
	ctx := agent.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarHook: "/bin/crowbar"}
	blocker := filepath.Join(ctx.Tmp, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))

	d := mustDescriptor(t, "\nconfig_injection:\n  - render_hooks: { into: \""+blocker+"/nested/settings.json\" }\n")

	_, err := agent.BuildSpawnPlan(d, ctx, nil, nil)
	require.Error(t, err)
}

func TestWriteFileStep_MkdirFailsWhenPathParentIsAFile(t *testing.T) {
	ctx := agent.TemplateCtx{Tmp: t.TempDir(), Cwd: t.TempDir(), CrowbarHook: "/bin/crowbar"}
	blocker := filepath.Join(ctx.Tmp, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))

	d := mustDescriptor(t, "\nconfig_injection:\n  - write_file: { path: \""+blocker+"/nested/dest.txt\", content: \"hi\" }\n")

	_, err := agent.BuildSpawnPlan(d, ctx, nil, nil)
	require.Error(t, err)
}
