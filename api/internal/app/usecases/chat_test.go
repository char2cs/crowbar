package usecases_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newChatUsecase(
	t *testing.T,
) (
	*mocks.ChatRepo,
	*mocks.ChatWorkspaceRepo,
	*mocks.ProjectRollup,
	usecases.ChatUsecase,
) {
	t.Helper()
	chat := mocks.NewChatRepo()
	ws := mocks.NewChatWorkspaceRepo()
	roll := mocks.NewProjectRollup()
	uc := usecases.NewChatUsecase(chat, ws, roll)
	return chat, ws, roll, uc
}

func TestChatUsecase_CreateChat_TouchesActivityAndRollsUp(t *testing.T) {
	chat, ws, roll, uc := newChatUsecase(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)

	ws.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, RepoID: "r1"}, nil
	}

	got, err := uc.CreateChat(ctx, "w1", "My Chat", now)
	require.NoError(t, err)
	assert.NotEmpty(t, got.ID)
	assert.Equal(t, "My Chat", chat.Created[0].Title)
	assert.Equal(t, "w1", chat.Created[0].WsID)
	assert.Equal(t, "w1", ws.TouchedID)
	assert.Equal(t, "r1", roll.TouchedRepoID)
}

func TestChatUsecase_CreateChat_CreateError(t *testing.T) {
	chat, _, _, uc := newChatUsecase(t)
	ctx := context.Background()

	chat.CreateErr = errors.New("boom")
	_, err := uc.CreateChat(ctx, "w1", "t", time.Now())
	assert.Error(t, err)
}

func TestChatUsecase_CreateChat_RollupSkippedWhenWorkspaceMissing(t *testing.T) {
	chat, ws, roll, uc := newChatUsecase(t)
	ctx := context.Background()

	ws.GetFn = func(_ context.Context, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("missing")
	}

	_, err := uc.CreateChat(ctx, "w1", "t", time.Now())
	require.NoError(t, err)
	assert.NotEmpty(t, chat.Created)
	assert.False(t, roll.Touched)
}

func TestChatUsecase_ForkChat_LoadsParentTitle(t *testing.T) {
	chat, ws, _, uc := newChatUsecase(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)

	chat.GetFn = func(_ context.Context, id string) (domain.Chat, error) {
		return domain.Chat{ID: id, WsID: "w1", Title: "Parent"}, nil
	}
	ws.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		return domain.Workspace{ID: id, RepoID: "r1"}, nil
	}

	got, err := uc.ForkChat(ctx, "parent1", now)
	require.NoError(t, err)
	assert.NotEmpty(t, got.ID)
	assert.Equal(t, "parent1", chat.Forked[0].ParentID)
	assert.Equal(t, "w1", chat.Forked[0].WsID)
	assert.Equal(t, "Parent", chat.Forked[0].Title)
}

func TestChatUsecase_ForkChat_ParentNotFound(t *testing.T) {
	chat, _, _, uc := newChatUsecase(t)
	ctx := context.Background()

	chat.GetFn = func(_ context.Context, _ string) (domain.Chat, error) {
		return domain.Chat{}, errors.New("boom")
	}

	_, err := uc.ForkChat(ctx, "parent1", time.Now())
	assert.Error(t, err)
}

func TestChatUsecase_ForkChat_ForkError(t *testing.T) {
	chat, _, _, uc := newChatUsecase(t)
	ctx := context.Background()

	chat.GetFn = func(_ context.Context, id string) (domain.Chat, error) {
		return domain.Chat{ID: id, WsID: "w1", Title: "p"}, nil
	}
	chat.ForkErr = errors.New("boom")

	_, err := uc.ForkChat(ctx, "parent1", time.Now())
	assert.Error(t, err)
}

func TestChatUsecase_RenameChat(t *testing.T) {
	_, _, _, uc := newChatUsecase(t)
	ctx := context.Background()

	got, err := uc.RenameChat(ctx, "c1", "New")
	require.NoError(t, err)
	assert.Equal(t, "New", got.Title)
}

func TestChatUsecase_RenameChat_Error(t *testing.T) {
	chat, _, _, uc := newChatUsecase(t)
	ctx := context.Background()

	chat.RenameErr = errors.New("boom")
	_, err := uc.RenameChat(ctx, "c1", "New")
	assert.Error(t, err)
}

func TestChatUsecase_DeleteChat_CascadesToChildren(t *testing.T) {
	chat, _, _, uc := newChatUsecase(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)

	chat.GetFn = func(_ context.Context, id string) (domain.Chat, error) {
		return domain.Chat{ID: id, WsID: "w1"}, nil
	}
	chat.ListByWorkspaceFn = func(_ context.Context, _ string) ([]domain.Chat, error) {
		return []domain.Chat{
			{ID: "c1", WsID: "w1"},
			{ID: "child1", WsID: "w1", ParentID: "c1"},
			{ID: "child2", WsID: "w1", ParentID: "c1"},
			{ID: "grand1", WsID: "w1", ParentID: "child1"},
			{ID: "other", WsID: "w1", ParentID: "z"},
		}, nil
	}

	err := uc.DeleteChat(ctx, "c1", now)
	require.NoError(t, err)
	assert.ElementsMatch(t,
		[]string{"grand1", "child1", "child2", "c1"},
		chat.Deleted,
	)
}

func TestChatUsecase_DeleteChat_NoChildren(t *testing.T) {
	chat, _, _, uc := newChatUsecase(t)
	ctx := context.Background()

	chat.GetFn = func(_ context.Context, id string) (domain.Chat, error) {
		return domain.Chat{ID: id, WsID: "w1"}, nil
	}
	chat.ListByWorkspaceFn = func(_ context.Context, _ string) ([]domain.Chat, error) {
		return []domain.Chat{{ID: "c1", WsID: "w1"}}, nil
	}

	err := uc.DeleteChat(ctx, "c1", time.Now())
	require.NoError(t, err)
	assert.Equal(t, []string{"c1"}, chat.Deleted)
}

func TestChatUsecase_DeleteChat_GetError(t *testing.T) {
	chat, _, _, uc := newChatUsecase(t)
	ctx := context.Background()

	chat.GetFn = func(_ context.Context, _ string) (domain.Chat, error) {
		return domain.Chat{}, errors.New("boom")
	}

	err := uc.DeleteChat(ctx, "c1", time.Now())
	assert.Error(t, err)
}

func TestChatUsecase_DeleteChat_ListError(t *testing.T) {
	chat, _, _, uc := newChatUsecase(t)
	ctx := context.Background()

	chat.GetFn = func(_ context.Context, id string) (domain.Chat, error) {
		return domain.Chat{ID: id, WsID: "w1"}, nil
	}
	chat.ListByWorkspaceFn = func(_ context.Context, _ string) ([]domain.Chat, error) {
		return nil, errors.New("boom")
	}

	err := uc.DeleteChat(ctx, "c1", time.Now())
	assert.Error(t, err)
}

func TestChatUsecase_DeleteChat_DeleteError(t *testing.T) {
	chat, _, _, uc := newChatUsecase(t)
	ctx := context.Background()

	chat.GetFn = func(_ context.Context, id string) (domain.Chat, error) {
		return domain.Chat{ID: id, WsID: "w1"}, nil
	}
	chat.ListByWorkspaceFn = func(_ context.Context, _ string) ([]domain.Chat, error) {
		return []domain.Chat{{ID: "c1", WsID: "w1"}}, nil
	}
	chat.DeleteErr = errors.New("boom")

	err := uc.DeleteChat(ctx, "c1", time.Now())
	assert.Error(t, err)
}
