package migrations

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

const oldOverride = "# my notes\nid: acme\nsession:\n  resume: { arg: \"--resume {id}\" } # keep\n" +
	"  locate: { root: \"~/x\" }\n"

// An override written for an older schema is rewritten in place, stamped, its
// comments kept and its original beside it; the next boot has nothing to do.
func TestDescriptorSchema_RewritesAnOldOverrideKeepingTheOriginal(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "descriptors/acme.yaml", oldOverride)

	finish(t, boot(t, h))

	root := h.Root()
	assert.Equal(t, oldOverride, readHome(t, root, "descriptors/acme.yaml.schema1.bak"))
	now := readHome(t, root, "descriptors/acme.yaml")
	assert.Contains(t, now, "# my notes")
	assert.Contains(t, now, "# keep")
	assert.Contains(t, now, "schema_version: 3")
	assert.Contains(t, now, "persists: turn_start")
	assert.NotContains(t, now, "locate")
	up, err := descriptorcheck.Upgrade([]byte(now))
	require.NoError(t, err)
	assert.Empty(t, up.Changes)
	info, err := os.Stat(filepath.Join(root, "descriptors/acme.yaml"))
	require.NoError(t, err)
	assert.Zero(t, info.Mode().Perm()&0o077)
	assert.Equal(t, []migrate.Counts{{"restored": 0, "rewritten": 1, "changes": 2}},
		migratetest.Applied(t, h)[StepDescriptorSchema])

	finish(t, boot(t, h))
	assert.Len(t, migratetest.Applied(t, h)[StepDescriptorSchema], 1, "nothing left to do at the next boot")
	assert.NoFileExists(t, filepath.Join(root, "descriptors/acme.yaml.schema1.bak.2"))
}

// A file the upgrade cannot complete, short of a fact a later schema requires,
// is stamped with the schema it meets: at every later start it still runs.
func TestDescriptorSchema_StampsTheSchemaTheFileMeets(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "descriptors/acme.yaml", oldOverride+
		"terminal_notices:\n  - kind: usage_limit\n    needle: \"limit reached\"\n")

	finish(t, boot(t, h))

	now := readHome(t, h.Root(), "descriptors/acme.yaml")
	assert.Contains(t, now, "schema_version: 2")
	assert.NotContains(t, now, "locate")
	f := findingFor(t, descriptorcheck.Validate([]byte(now)), "terminal_notices.anchor_missing")
	assert.Equal(t, descriptorcheck.SeverityWarning, f.Severity)
	assert.Contains(t, f.Hint, "schema_version: 3")

	finish(t, boot(t, h))
	assert.Len(t, migratetest.Applied(t, h)[StepDescriptorSchema], 1, "nothing left to do at the next boot")
}

func findingFor(t *testing.T, rep descriptorcheck.Report, rule string) descriptorcheck.Finding {
	t.Helper()
	for _, f := range rep.Findings {
		if f.Rule == rule {
			return f
		}
	}
	t.Fatalf("no %s finding in %+v", rule, rep.Findings)
	return descriptorcheck.Finding{}
}

// An earlier build set refused overrides aside as <name>.refused; this step puts
// each back where the loader reads it, and brings it up to date.
func TestDescriptorSchema_PutsBackAnOverrideSetAside(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "descriptors/acme.yaml.refused", oldOverride)
	testutil.WriteFile(t, h, "descriptors/kept.yaml", "id: kept\nschema_version: 3\n")
	testutil.WriteFile(t, h, "descriptors/kept.yaml.refused", "# an older copy\n")

	finish(t, boot(t, h))

	root := h.Root()
	assert.NoFileExists(t, filepath.Join(root, "descriptors/acme.yaml.refused"))
	assert.Contains(t, readHome(t, root, "descriptors/acme.yaml"), "schema_version: 3")
	assert.Equal(t, oldOverride, readHome(t, root, "descriptors/acme.yaml.schema1.bak"))
	assert.Equal(t, "id: kept\nschema_version: 3\n", readHome(t, root, "descriptors/kept.yaml"),
		"a file the user has beside the copy wins")
	assert.Equal(t, "# an older copy\n", readHome(t, root, "descriptors/kept.yaml.refused"))
	assert.Equal(t, []migrate.Counts{{"restored": 1, "rewritten": 1, "changes": 2}},
		migratetest.Applied(t, h)[StepDescriptorSchema])
}

// A document this build cannot read, a newer one or a broken one, stays where it
// is, unchanged: validation reports it, nothing is lost.
func TestDescriptorSchema_LeavesWhatItCannotUpgradeAlone(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "descriptors/newer.yaml", "id: newer\nschema_version: 9\n")
	testutil.WriteFile(t, h, "descriptors/broken.yaml", "id: broken\nnot: [yaml\n")

	finish(t, boot(t, h))

	root := h.Root()
	assert.Equal(t, "id: newer\nschema_version: 9\n", readHome(t, root, "descriptors/newer.yaml"))
	assert.Equal(t, "id: broken\nnot: [yaml\n", readHome(t, root, "descriptors/broken.yaml"))
	matches, err := filepath.Glob(filepath.Join(root, "descriptors", "*.bak"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

// A restored home runs the step again over a file whose earlier backup is still
// there: the new backup is set beside it, never over it.
func TestDescriptorSchema_KeepsAnEarlierBackup(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "descriptors/acme.yaml", oldOverride)
	testutil.WriteFile(t, h, "descriptors/acme.yaml.schema1.bak", "# first\n")

	finish(t, boot(t, h))

	root := h.Root()
	assert.Equal(t, "# first\n", readHome(t, root, "descriptors/acme.yaml.schema1.bak"))
	assert.Equal(t, oldOverride, readHome(t, root, "descriptors/acme.yaml.schema1.bak.2"))
}
