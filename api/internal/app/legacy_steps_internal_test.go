package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

// fakeHomeCreator refuses a home whose id is taken, as the workspace create
// command does for any row under that id, tombstones included.
type fakeHomeCreator struct {
	taken   map[string]bool
	created []string
	err     error
}

func (f *fakeHomeCreator) CreateHome(_ context.Context, p domain.Project) (domain.Workspace, error) {
	if f.taken[workspace.ProjectHomeID(p.ID)] {
		return domain.Workspace{}, errors.New("workspace already exists")
	}
	f.created = append(f.created, p.ID)
	return domain.Workspace{}, f.err
}

// fakeCheckpoints records what a step marked, by key.
type fakeCheckpoints map[string]migrate.Counts

func (f fakeCheckpoints) Done(key string) bool {
	_, ok := f[key]
	return ok
}

func (f fakeCheckpoints) Mark(key string, n migrate.Counts) error {
	f[key] = n
	return nil
}

// Only a live project with no live home gets one, each checkpointed as it
// lands. A home tombstone still holds the home's id until its purge runs, so
// that project's home is not missing and cannot be created again.
// migrate --plan previews a boot with Implemented: a step the catalog holds and
// it does not name would preview as a halt the boot never gives.
func TestImplemented_NamesEveryStepTheBootRuns(t *testing.T) {
	cat, err := migrations.Catalog()
	require.NoError(t, err)

	report, err := migrate.Inspect(t.Context(), testutil.FixtureHome(t), cat, Implemented, migrate.Options{})

	require.NoError(t, err)
	assert.Len(t, report.Steps, 5, "a fresh home has every step pending")
	assert.False(t, Implemented(migrations.StepHookDeliveries+1), "no step past the catalog")
}

func TestReconcileProjectHomes_CreatesOnlyTheMissingHomes(t *testing.T) {
	tombstone := workspace.ProjectHomeID("tombstoned")
	c, cp := &fakeHomeCreator{taken: map[string]bool{tombstone: true}}, fakeCheckpoints{}
	err := reconcileProjectHomes(context.Background(),
		[]domain.Project{{ID: "homed"}, {ID: "bare"}, {ID: "going", Deleting: true}, {ID: "tombstoned"}},
		[]domain.Workspace{
			{ID: "h1", ProjectID: "homed", Kind: domain.WorkspaceKindHome},
			{ID: tombstone, ProjectID: "tombstoned", Kind: domain.WorkspaceKindHome, Status: domain.WorkspaceStatusDeleted},
			{ID: "w", ProjectID: "bare", Kind: domain.WorkspaceKindGit},
		}, c, cp)

	require.NoError(t, err)
	assert.Equal(t, []string{"bare"}, c.created)
	assert.Equal(t, fakeCheckpoints{"bare": {"homes_created": 1}}, cp)
}

func TestReconcileProjectHomes_AHomeThatCannotBeCreatedFailsTheStep(t *testing.T) {
	cp := fakeCheckpoints{}
	err := reconcileProjectHomes(context.Background(), []domain.Project{{ID: "bare"}}, nil,
		&fakeHomeCreator{err: errors.New("boom")}, cp)

	require.ErrorContains(t, err, "bare")
	assert.Empty(t, cp)
}

// An older daemon finished every step and left its markers: each step is
// adopted, and none of the work behind it runs.
func TestLegacyImpls_AdoptedStepRunsNothing(t *testing.T) {
	h, err := runLegacy(t, untouchable(),
		"project-homes", "workspace-paths-table", "chat-types", "chat-providers-from-history", "hook-deliveries")

	require.NoError(t, err)
	adopted := []migrate.Counts{{"adopted": 1}}
	applied := migratetest.Applied(t, h)
	for _, step := range []int{
		migrations.StepProjectHomes, migrations.StepChatTypes, migrations.StepChatProviders, migrations.StepHookDeliveries,
	} {
		assert.Equal(t, adopted, applied[step], "step %d", step)
	}
	assert.NoDirExists(t, filepath.Join(h.State(), "upgrades"))
}

// The old closures' false meant "some rows could not be replayed": those rows
// are counted and the step completes, as the pre-engine boot did.
func TestLegacyImpls_SkippedRowsCompleteTheStep(t *testing.T) {
	f := upToDate()
	f.backfillChatTypes = func(context.Context) (int, error) { return 2, nil }
	f.restateProviders = func(context.Context) (int, error) { return 1, nil }

	h, err := runLegacy(t, f)

	require.NoError(t, err)
	applied := migratetest.Applied(t, h)
	assert.Equal(t, []migrate.Counts{{"skipped_rows": 2}}, applied[migrations.StepChatTypes])
	assert.Equal(t, []migrate.Counts{{"skipped_rows": 1}}, applied[migrations.StepChatProviders])
}

// The other half of the old false, a list that could not be read, is no row
// to skip: the step fails and halts the boot.
func TestLegacyImpls_AFailedListFailsTheStep(t *testing.T) {
	f := upToDate()
	f.backfillChatTypes = func(context.Context) (int, error) { return 0, errors.New("list chats: disk gone") }

	_, err := runLegacy(t, f)

	halt := requireHalt(t, err, migrate.HaltStepFailed)
	assert.Equal(t, "chat-types", halt.Step.Name)
	assert.Contains(t, halt.Message, "disk gone")
}

// A retired hook journal that cannot be deleted is logged and counted, not
// fatal: the step completes, and its check excuses the journal it kept.
func TestLegacyImpls_AnUndeletableHookJournalIsSkippedNotFatal(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	h := testutil.FixtureHome(t)
	chats := filepath.Join(h.Root(), "projects", "p", "chats")
	stuck := hookJournal{workspaceID: "stuck", path: filepath.Join(chats, hookDeliveriesDir)}
	gone := hookJournal{workspaceID: "gone", path: filepath.Join(h.Root(), "projects", "q", "chats", hookDeliveriesDir)}
	testutil.WriteFile(t, h, filepath.Join("projects", "p", "chats", hookDeliveriesDir, "seen"), "")
	testutil.WriteFile(t, h, filepath.Join("projects", "q", "chats", hookDeliveriesDir, "seen"), "")
	require.NoError(t, os.Chmod(chats, 0o500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(chats, 0o700)) })
	f := upToDate()
	f.hookJournals = journalsAt(stuck, gone)

	require.NoError(t, runLegacyIn(t, h, f))

	assert.DirExists(t, stuck.path)
	assert.NoDirExists(t, gone.path)
	applied := migratetest.Applied(t, h)
	assert.Equal(t, []migrate.Counts{{"skipped_rows": 1, "dirs_removed": 1}}, applied[migrations.StepHookDeliveries])
}

// A halt names the first rows a check found, not all of them: the message is
// kept in the ledger and served to the user.
func TestLegacyImpls_AHaltNamesTheFirstTenRowsLeft(t *testing.T) {
	f := upToDate()
	f.missingHomes = func(context.Context) ([]string, error) {
		ids := make([]string, 12)
		for i := range ids {
			ids[i] = fmt.Sprintf("row-%02d", i+1)
		}
		return ids, nil
	}

	_, err := runLegacy(t, f)

	halt := requireHalt(t, err, migrate.HaltVerifyFailed)
	assert.Contains(t, halt.Message, "12 projects without a home remain: row-01, ")
	assert.Contains(t, halt.Message, "row-10 and 2 more")
	assert.NotContains(t, halt.Message, "row-11")
}

// Each step checks its own postcondition: a row it could have replayed and
// did not upgrade halts the boot, naming the step and the row.
func TestLegacyImpls_ARowLeftBehindFailsVerification(t *testing.T) {
	leftBehind := func(context.Context) ([]string, error) { return []string{"row-1"}, nil }
	for name, spoil := range map[string]func(*legacyFuncs, home.Home){
		"project-homes":               func(f *legacyFuncs, _ home.Home) { f.missingHomes = leftBehind },
		"chat-types":                  func(f *legacyFuncs, _ home.Home) { f.untypedChats = leftBehind },
		"chat-providers-from-history": func(f *legacyFuncs, _ home.Home) { f.staleProviders = leftBehind },
		// Still listed after its removal reported no failure: nothing excuses it.
		"hook-deliveries": func(f *legacyFuncs, h home.Home) {
			journal := hookJournal{workspaceID: "ws", path: filepath.Join(h.Root(), "row-1")}
			f.hookJournals = func(context.Context) ([]hookJournal, []error, error) {
				return []hookJournal{journal}, nil, nil
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := testutil.FixtureHome(t)
			f := upToDate()
			spoil(&f, h)

			err := runLegacyIn(t, h, f)

			halt := requireHalt(t, err, migrate.HaltVerifyFailed)
			assert.Equal(t, name, halt.Step.Name)
			assert.Contains(t, halt.Message, "row-1")
		})
	}
}

// runLegacy boots a fresh home holding markers through the production catalog
// and runs the legacy steps over f, returning what RunPostOpen returned.
func runLegacy(
	t *testing.T,
	f legacyFuncs,
	markers ...string,
) (home.Home, error) {
	t.Helper()
	h := testutil.FixtureHome(t)
	for _, name := range markers {
		testutil.WriteFile(t, h, filepath.Join("state", "upgrades", name), "")
	}
	return h, runLegacyIn(t, h, f)
}

func runLegacyIn(
	t *testing.T,
	h home.Home,
	f legacyFuncs,
) error {
	t.Helper()
	sess, err := migrations.Begin(t.Context(), h, migrate.Options{Version: "v-test"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sess.Close()) })
	require.NoError(t, sess.RunPreOpen(t.Context(), migrations.PreOpenImpls()))
	return sess.RunPostOpen(t.Context(), legacyImpls(f))
}

// journalsAt is a hook-journal listing that names each of journals while it is
// still on disk, as the production listing does.
func journalsAt(
	journals ...hookJournal,
) func(context.Context) ([]hookJournal, []error, error) {
	return func(context.Context) ([]hookJournal, []error, error) {
		var present []hookJournal
		for _, j := range journals {
			if _, err := os.Lstat(j.path); !errors.Is(err, fs.ErrNotExist) {
				present = append(present, j)
			}
		}
		return present, nil, nil
	}
}

// upToDate is a home with nothing left to upgrade.
func upToDate() legacyFuncs {
	none := func(context.Context) ([]string, error) { return nil, nil }
	return legacyFuncs{
		createMissingHomes: func(context.Context, checkpoints) error { return nil },
		missingHomes:       none,
		backfillChatTypes:  func(context.Context) (int, error) { return 0, nil },
		untypedChats:       none,
		restateProviders:   func(context.Context) (int, error) { return 0, nil },
		staleProviders:     none,
		hookJournals:       journalsAt(),
	}
}

// untouchable halts the run if any step's work runs.
func untouchable() legacyFuncs {
	called := func(what string) error {
		return errors.New(what + " ran on an adopted step")
	}
	return legacyFuncs{
		createMissingHomes: func(context.Context, checkpoints) error { return called("createMissingHomes") },
		missingHomes:       func(context.Context) ([]string, error) { return nil, called("missingHomes") },
		backfillChatTypes:  func(context.Context) (int, error) { return 0, called("backfillChatTypes") },
		untypedChats:       func(context.Context) ([]string, error) { return nil, called("untypedChats") },
		restateProviders:   func(context.Context) (int, error) { return 0, called("restateProviders") },
		staleProviders:     func(context.Context) ([]string, error) { return nil, called("staleProviders") },
		hookJournals: func(context.Context) ([]hookJournal, []error, error) {
			return nil, nil, called("hookJournals")
		},
	}
}

func requireHalt(
	t *testing.T,
	err error,
	reason migrate.HaltReason,
) *migrate.Halt {
	t.Helper()
	var halt *migrate.Halt
	require.ErrorAs(t, err, &halt)
	require.Equal(t, reason, halt.Reason, halt.Message)
	require.NotNil(t, halt.Step)
	return halt
}
