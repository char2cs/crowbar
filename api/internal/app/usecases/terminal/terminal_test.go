package terminal_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newTerminalUsecase(
	t *testing.T,
) (
	*mocks.TerminalEngine,
	*mocks.TerminalProfileStore,
	*mocks.WorkspaceSyncer,
	terminal.Usecase,
) {
	t.Helper()
	eng := mocks.NewTerminalEngine()
	profiles := mocks.NewTerminalProfileStore()
	syncer := mocks.NewWorkspaceSyncer()
	syncer.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, WorktreePath: "/repo/x"}, nil
	}
	uc := terminal.New(eng, profiles, syncer)
	return eng, profiles, syncer, uc
}

func TestTerminalUsecase_CreateSession_ResolvesWorkspaceDir(t *testing.T) {
	eng, _, _, uc := newTerminalUsecase(t)
	ctx := context.Background()

	var gotDir string
	eng.CreateFn = func(_ context.Context, _, dir string, _ *domain.TerminalProfile) (string, error) {
		gotDir = dir
		return "sess1", nil
	}

	id, err := uc.CreateSession(ctx, "w1", nil)
	require.NoError(t, err)
	assert.Equal(t, "sess1", id)
	assert.Equal(t, "/repo/x", gotDir)
}

func TestTerminalUsecase_CreateSession_WorkspaceError(t *testing.T) {
	eng, _, syncer, uc := newTerminalUsecase(t)
	ctx := context.Background()
	syncer.GetFn = func(_ context.Context, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}
	eng.CreateFn = func(_ context.Context, _, _ string, _ *domain.TerminalProfile) (string, error) {
		return "", nil
	}

	_, err := uc.CreateSession(ctx, "w1", nil)
	assert.Error(t, err)
}

func TestTerminalUsecase_CreateSession_EngineError(t *testing.T) {
	eng, _, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	eng.CreateFn = func(_ context.Context, _, _ string, _ *domain.TerminalProfile) (string, error) {
		return "", errors.New("boom")
	}

	_, err := uc.CreateSession(ctx, "w1", nil)
	assert.Error(t, err)
}

func TestTerminalUsecase_KillSession(t *testing.T) {
	eng, _, _, uc := newTerminalUsecase(t)
	ctx := context.Background()

	var killed string
	eng.KillFn = func(_ context.Context, id string) error {
		killed = id
		return nil
	}

	require.NoError(t, uc.KillSession(ctx, "sess1"))
	assert.Equal(t, "sess1", killed)
}

func TestTerminalUsecase_KillSession_Error(t *testing.T) {
	eng, _, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	eng.KillFn = func(_ context.Context, _ string) error { return errors.New("boom") }

	assert.Error(t, uc.KillSession(ctx, "sess1"))
}

func TestTerminalUsecase_ListProfiles(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	_ = profiles.Save(ctx, domain.TerminalProfile{ID: "p1", Name: "a"})

	list, err := uc.ListProfiles(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestTerminalUsecase_ListProfiles_Error(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	profiles.FindAllErr = errors.New("boom")

	_, err := uc.ListProfiles(ctx)
	assert.Error(t, err)
}

func TestTerminalUsecase_CreateProfile_MintsID(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()

	got, err := uc.CreateProfile(ctx, domain.TerminalProfile{Name: "a"})
	require.NoError(t, err)
	assert.NotEmpty(t, got.ID)
	assert.Len(t, profiles.Saved, 1)
}

func TestTerminalUsecase_CreateProfile_Error(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	profiles.SaveErr = errors.New("boom")

	_, err := uc.CreateProfile(ctx, domain.TerminalProfile{Name: "a"})
	assert.Error(t, err)
}

func TestTerminalUsecase_UpdateProfile(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	_ = profiles.Save(ctx, domain.TerminalProfile{ID: "p1", Name: "a"})

	got, err := uc.UpdateProfile(ctx, domain.TerminalProfile{ID: "p1", Name: "b"})
	require.NoError(t, err)
	assert.Equal(t, "b", got.Name)
}

func TestTerminalUsecase_UpdateProfile_MissingID(t *testing.T) {
	_, _, _, uc := newTerminalUsecase(t)
	ctx := context.Background()

	_, err := uc.UpdateProfile(ctx, domain.TerminalProfile{Name: "b"})
	assert.Error(t, err)
}

func TestTerminalUsecase_UpdateProfile_SaveError(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	profiles.SaveErr = errors.New("boom")

	_, err := uc.UpdateProfile(ctx, domain.TerminalProfile{ID: "p1", Name: "b"})
	assert.Error(t, err)
}

func TestTerminalUsecase_DeleteProfile(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()

	require.NoError(t, uc.DeleteProfile(ctx, "p1"))
	assert.Equal(t, []string{"p1"}, profiles.Deleted)
}

func TestTerminalUsecase_DeleteProfile_Error(t *testing.T) {
	_, profiles, _, uc := newTerminalUsecase(t)
	ctx := context.Background()
	profiles.DeleteErr = errors.New("boom")

	assert.Error(t, uc.DeleteProfile(ctx, "p1"))
}
