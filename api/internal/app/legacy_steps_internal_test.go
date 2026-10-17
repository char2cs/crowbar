package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

type fakeHomeCreator struct {
	created []string
	err     error
}

func (f *fakeHomeCreator) CreateHome(_ context.Context, p domain.Project) (domain.Workspace, error) {
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

// Only a live project with no live home gets one, each checkpointed as it lands.
func TestReconcileProjectHomes_CreatesOnlyTheMissingHomes(t *testing.T) {
	c, cp := &fakeHomeCreator{}, fakeCheckpoints{}
	err := reconcileProjectHomes(context.Background(),
		[]domain.Project{{ID: "homed"}, {ID: "bare"}, {ID: "going", Deleting: true}, {ID: "tombstoned"}},
		[]domain.Workspace{
			{ID: "h1", ProjectID: "homed", Kind: domain.WorkspaceKindHome},
			{ID: "h2", ProjectID: "tombstoned", Kind: domain.WorkspaceKindHome, Status: domain.WorkspaceStatusDeleted},
			{ID: "w", ProjectID: "bare", Kind: domain.WorkspaceKindGit},
		}, c, cp)

	require.NoError(t, err)
	assert.Equal(t, []string{"bare", "tombstoned"}, c.created)
	assert.Equal(t, fakeCheckpoints{"bare": {"homes_created": 1}, "tombstoned": {"homes_created": 1}}, cp)
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

// Each step checks its own postcondition: a row it could have replayed and
// did not upgrade halts the boot, naming the step and the row.
func TestLegacyImpls_ARowLeftBehindFailsVerification(t *testing.T) {
	leftBehind := func(context.Context) ([]string, error) { return []string{"row-1"}, nil }
	for name, spoil := range map[string]func(*legacyFuncs){
		"project-homes":               func(f *legacyFuncs) { f.missingHomes = leftBehind },
		"chat-types":                  func(f *legacyFuncs) { f.untypedChats = leftBehind },
		"chat-providers-from-history": func(f *legacyFuncs) { f.staleProviders = leftBehind },
	} {
		t.Run(name, func(t *testing.T) {
			f := upToDate()
			spoil(&f)

			_, err := runLegacy(t, f)

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
	sess, err := migrations.Begin(context.Background(), h, migrate.Options{Version: "v-test"})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sess.Close()) })
	require.NoError(t, sess.RunPreOpen(context.Background(), migrations.PreOpenImpls()))
	return h, sess.RunPostOpen(context.Background(), legacyImpls(f))
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
		removeHookJournals: func(context.Context, checkpoints) (int, error) { return 0, nil },
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
		removeHookJournals: func(context.Context, checkpoints) (int, error) { return 0, called("removeHookJournals") },
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
