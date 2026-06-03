package conversation

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func openTestDB(t *testing.T) *sqlite.Store {
	t.Helper()
	dir := t.TempDir()
	store, err := sqlite.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// openMigratedThenClosedRepo opens a store, runs New so the schema exists,
// then closes the store so subsequent DB calls fail.
func openMigratedThenClosedRepo(t *testing.T) *conversationRepo {
	t.Helper()
	dir := t.TempDir()
	store, err := sqlite.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	r, err := New(store.DB())
	require.NoError(t, err)
	require.NoError(t, store.Close())
	return r.(*conversationRepo)
}

func TestNew_AutoMigrate(t *testing.T) {
	store := openTestDB(t)
	_, err := New(store.DB())
	require.NoError(t, err)
}

func TestNew_MigrateFails(t *testing.T) {
	dir := t.TempDir()
	store, err := sqlite.New(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	require.NoError(t, store.Close()) // close before New so AutoMigrate fails
	_, err = New(store.DB())
	require.Error(t, err)
}

func TestCreate_DBError(t *testing.T) {
	r := openMigratedThenClosedRepo(t)
	ctx := context.Background()
	_, err := r.Create(ctx, "task-1", domain.ConversationMessageRoleUser, domain.ConversationMessageTypeText, "content")
	require.Error(t, err)
}

func TestListByTask_DBError(t *testing.T) {
	r := openMigratedThenClosedRepo(t)
	ctx := context.Background()
	_, err := r.ListByTask(ctx, "task-1")
	require.Error(t, err)
}

func TestCreate(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()
	msg, err := r.Create(ctx, "task-1", domain.ConversationMessageRoleUser, domain.ConversationMessageTypeText, "hello")
	require.NoError(t, err)
	require.NotEmpty(t, msg.ID)
	require.Equal(t, "task-1", msg.TaskID)
	require.Equal(t, domain.ConversationMessageRoleUser, msg.Role)
	require.Equal(t, domain.ConversationMessageTypeText, msg.Type)
	require.Equal(t, "hello", msg.Content)
	require.False(t, msg.CreatedAt.IsZero())
}

func TestCreate_AllRolesAndTypes(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()

	cases := []struct {
		role    domain.ConversationMessageRole
		msgType domain.ConversationMessageType
		content string
	}{
		{domain.ConversationMessageRoleUser, domain.ConversationMessageTypeText, "user text"},
		{domain.ConversationMessageRoleAgent, domain.ConversationMessageTypeToolCall, "tool call payload"},
		{domain.ConversationMessageRoleAgent, domain.ConversationMessageTypeToolResult, "tool result payload"},
	}

	for _, tc := range cases {
		msg, err := r.Create(ctx, "task-roles", tc.role, tc.msgType, tc.content)
		require.NoError(t, err)
		require.Equal(t, tc.role, msg.Role)
		require.Equal(t, tc.msgType, msg.Type)
		require.Equal(t, tc.content, msg.Content)
	}
}

func TestListByTask_Empty(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()
	msgs, err := r.ListByTask(ctx, "task-empty")
	require.NoError(t, err)
	require.Empty(t, msgs)
}

func TestListByTask_OrderedByCreatedAtASC(t *testing.T) {
	store := openTestDB(t)
	ri, err := New(store.DB())
	require.NoError(t, err)
	r := ri.(*conversationRepo)

	ctx := context.Background()
	taskID := "task-ordered"

	// Insert three messages with explicit timestamps to guarantee order
	msgs := []struct {
		role    domain.ConversationMessageRole
		msgType domain.ConversationMessageType
		content string
		offset  time.Duration
	}{
		{domain.ConversationMessageRoleUser, domain.ConversationMessageTypeText, "first", 0},
		{domain.ConversationMessageRoleAgent, domain.ConversationMessageTypeText, "second", time.Millisecond},
		{domain.ConversationMessageRoleUser, domain.ConversationMessageTypeText, "third", 2 * time.Millisecond},
	}

	var createdIDs []string
	base := time.Now().UTC()
	for _, m := range msgs {
		msg := domain.ConversationMessage{
			ID:        m.content + "-id",
			TaskID:    taskID,
			Role:      m.role,
			Type:      m.msgType,
			Content:   m.content,
			CreatedAt: base.Add(m.offset),
		}
		err := r.db.WithContext(ctx).Create(&msg).Error
		require.NoError(t, err)
		createdIDs = append(createdIDs, msg.ID)
	}

	result, err := r.ListByTask(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, result, 3)
	require.Equal(t, createdIDs[0], result[0].ID)
	require.Equal(t, createdIDs[1], result[1].ID)
	require.Equal(t, createdIDs[2], result[2].ID)
}

func TestListByTask_IsolatesByTaskID(t *testing.T) {
	store := openTestDB(t)
	r, err := New(store.DB())
	require.NoError(t, err)

	ctx := context.Background()

	_, err = r.Create(ctx, "task-X", domain.ConversationMessageRoleUser, domain.ConversationMessageTypeText, "msg for X")
	require.NoError(t, err)
	_, err = r.Create(ctx, "task-Y", domain.ConversationMessageRoleAgent, domain.ConversationMessageTypeText, "msg for Y")
	require.NoError(t, err)

	msgsX, err := r.ListByTask(ctx, "task-X")
	require.NoError(t, err)
	require.Len(t, msgsX, 1)
	require.Equal(t, "task-X", msgsX[0].TaskID)

	msgsY, err := r.ListByTask(ctx, "task-Y")
	require.NoError(t, err)
	require.Len(t, msgsY, 1)
	require.Equal(t, "task-Y", msgsY[0].TaskID)
}
