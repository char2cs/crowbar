package migratetree

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"slices"
	"strings"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type LegacySources struct {
	ChatLog    asynxModels.Store
	NodeLog    string
	View       *gormdb.DB
	Projects   func(ctx context.Context) ([]domain.Project, error)
	Workspaces func(ctx context.Context) ([]domain.Workspace, error)
	Repos      func(ctx context.Context) ([]domain.Repository, error)
}

type LegacyChat struct {
	ID, WorkspaceID string
	Placement       tree.Placement
}

type LegacyHome struct {
	Nodes       []tree.Node
	Folders     []tree.Folder
	Chats       []LegacyChat
	UnreadChats []string
	Workspaces  []domain.Workspace
	Repos       []domain.Repository
	Projects    []domain.Project
}

type legacyChat struct {
	domain.Chat
	ParentID string `json:"parentId,omitempty"`
	Order    int    `json:"order"`
}

type legacyNode struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	ParentID string `json:"parentId,omitempty"`
	Order    int    `json:"order"`
}

func ReadLegacy(ctx context.Context, src LegacySources) (LegacyHome, error) {
	var h LegacyHome
	var err error
	if h.Projects, err = src.Projects(ctx); err != nil {
		return LegacyHome{}, fmt.Errorf("migratetree: projects: %w", err)
	}
	if h.Workspaces, err = src.Workspaces(ctx); err != nil {
		return LegacyHome{}, fmt.Errorf("migratetree: workspaces: %w", err)
	}
	if h.Repos, err = src.Repos(ctx); err != nil {
		return LegacyHome{}, fmt.Errorf("migratetree: repos: %w", err)
	}
	if h.Chats, h.UnreadChats, err = readChats(ctx, src.ChatLog); err != nil {
		return LegacyHome{}, err
	}
	if h.Nodes, err = readNodes(ctx, src.NodeLog); err != nil {
		return LegacyHome{}, err
	}
	if h.Folders, err = readFolders(ctx, src.View); err != nil {
		return LegacyHome{}, err
	}
	return h, nil
}

func readChats(ctx context.Context, log asynxModels.Store) ([]LegacyChat, []string, error) {
	b := asynx.New[legacyChat]().WithEventStore(log).WithSnapshotStore(asynxstore.NewSnapshots())
	agentchat.KeepPlacement(b)
	ax, err := b.Build()
	if err != nil {
		return nil, nil, fmt.Errorf("migratetree: chat reader: %w", err)
	}
	defer shutdown(ctx, ax)
	var out []LegacyChat
	var unread []string
	err = eachAggregate(ctx, log, func(id string) error {
		c, err := ax.Get(ctx, id)
		if errors.Is(err, asynxModels.ErrNotFound) {
			return nil
		}
		if err != nil {
			slog.WarnContext(ctx, "migratetree: a chat's history does not replay", "chat_id", id, "err", err)
			unread = append(unread, id)
			return nil
		}
		out = append(out, LegacyChat{
			ID: id, WorkspaceID: c.WorkspaceID,
			Placement: tree.Placement{ParentID: c.ParentID, Order: c.Order},
		})
		return nil
	})
	return out, unread, err
}

func readNodes(ctx context.Context, path string) ([]tree.Node, error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	log, err := eventsqlite.OpenReadOnly(path)
	if err != nil {
		return nil, fmt.Errorf("migratetree: open %s: %w", path, err)
	}
	defer closeLog(ctx, log)
	ax, err := asynx.New[legacyNode]().WithEventStore(log).WithSnapshotStore(asynxstore.NewSnapshots()).Build()
	if err != nil {
		return nil, fmt.Errorf("migratetree: node reader: %w", err)
	}
	defer shutdown(ctx, ax)
	var out []tree.Node
	err = eachAggregate(ctx, log, func(id string) error {
		n, err := ax.Get(ctx, id)
		if errors.Is(err, asynxModels.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("migratetree: node %s: %w", id, err)
		}
		kind, err := tree.KindOf(n.Kind)
		if err != nil {
			return fmt.Errorf("migratetree: node %s: %w", id, err)
		}
		out = append(out, tree.Node{ID: n.ID, Kind: kind, ParentID: n.ParentID, Order: n.Order})
		return nil
	})
	return out, err
}

func readFolders(ctx context.Context, db *gormdb.DB) ([]tree.Folder, error) {
	var cols []string
	if err := db.WithContext(ctx).Raw("SELECT name FROM pragma_table_info('folders')").Scan(&cols).Error; err != nil {
		return nil, fmt.Errorf("migratetree: look for the folders table: %w", err)
	}
	if len(cols) == 0 {
		return nil, nil
	}
	home := "''"
	if slices.Contains(cols, "home_id") {
		home = "home_id"
	}
	var rows []struct{ ID, Name, RepoID, HomeID string }
	if err := db.WithContext(ctx).Raw(
		"SELECT id, name, repo_id, " + home + " AS home_id FROM folders ORDER BY id").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("migratetree: read folders: %w", err)
	}
	out := make([]tree.Folder, len(rows))
	for i, r := range rows {
		out[i] = tree.Folder{ID: r.ID, Name: r.Name, RepoID: r.RepoID, HomeID: r.HomeID}
	}
	return out, nil
}

const eventKeyPrefix = "events:"

type aggregateLister interface {
	AggregateIDs(ctx context.Context) ([]string, error)
}

func eachAggregate(ctx context.Context, log asynxModels.Store, each func(id string) error) error {
	lister, ok := log.(aggregateLister)
	if !ok {
		return errors.New("migratetree: the event log cannot list its aggregates")
	}
	keys, err := lister.AggregateIDs(ctx)
	if err != nil {
		return fmt.Errorf("migratetree: list aggregates: %w", err)
	}
	for _, key := range keys {
		if id, ok := strings.CutPrefix(key, eventKeyPrefix); ok {
			if err := each(id); err != nil {
				return err
			}
		}
	}
	return nil
}

func shutdown[T any](ctx context.Context, ax asynx.Asynx[T]) {
	if err := ax.Shutdown(ctx); err != nil {
		slog.WarnContext(ctx, "migratetree: stop a legacy reader", "err", err)
	}
}

func closeLog(ctx context.Context, log asynxModels.Store) {
	cl, ok := log.(io.Closer)
	if !ok {
		return
	}
	if err := cl.Close(); err != nil {
		slog.WarnContext(ctx, "migratetree: close the legacy node log", "err", err)
	}
}
