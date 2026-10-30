// Package migratetree is the placement-tree migration step: it copies the
// retired placement records (the position aggregate's rows, the folders table,
// each chat's own placement) into the per-project Trees, verbatim.
package migratetree

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
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

// LegacySources are where the retired records and the facts that place them live.
type LegacySources struct {
	// ChatLog is the chat event log; its schema-1 events carry each placement.
	ChatLog asynxModels.Store
	// NodeLog is the path of the retired position aggregate's event log.
	NodeLog string
	// View holds the retired folders table.
	View       *gormdb.DB
	Projects   func(ctx context.Context) ([]domain.Project, error)
	Workspaces func(ctx context.Context) ([]domain.Workspace, error)
	Repos      func(ctx context.Context) ([]domain.Repository, error)
}

// LegacyChat is the part of a chat the conversion reads.
type LegacyChat struct {
	ID, WorkspaceID string
	Placement       tree.Placement
}

// LegacyHome is every retired placement record with the facts that place it.
type LegacyHome struct {
	Nodes   []tree.Node
	Folders []tree.Folder
	Chats   []LegacyChat
	// UnreadChats are chats whose history does not replay: they still exist,
	// listed from their row, but carry no placement of their own to copy.
	UnreadChats []string
	Workspaces  []domain.Workspace
	Repos       []domain.Repository
	Projects    []domain.Project
}

// legacyChat is the chat document as schema 1 wrote it, placement included.
type legacyChat struct {
	domain.Chat
	ParentID string `json:"parentId,omitempty"`
	Order    int    `json:"order"`
}

// legacyNode is the retired position aggregate's document.
type legacyNode struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	ParentID string `json:"parentId,omitempty"`
	Order    int    `json:"order"`
}

// ReadLegacy reads every retired placement record. It writes nothing: its
// asynx instances keep their snapshots in memory and are shut down after.
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
	ax, err := asynx.New[legacyChat]().WithEventStore(log).WithSnapshotStore(asynxstore.NewSnapshots()).
		WithSchemaVersion(agentchat.SchemaVersion).WithUpcaster(1, keepAll).Build()
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
	log, err := eventsqlite.NewEventStore(path)
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
	var tables int64
	if err := db.WithContext(ctx).Raw(
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'folders'").Scan(&tables).Error; err != nil {
		return nil, fmt.Errorf("migratetree: look for the folders table: %w", err)
	}
	if tables == 0 {
		return nil, nil
	}
	var rows []struct{ ID, Name, RepoID, HomeID string }
	if err := db.WithContext(ctx).Raw(
		"SELECT id, name, repo_id, home_id FROM folders ORDER BY id").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("migratetree: read folders: %w", err)
	}
	out := make([]tree.Folder, len(rows))
	for i, r := range rows {
		out[i] = tree.Folder{ID: r.ID, Name: r.Name, RepoID: r.RepoID, HomeID: r.HomeID}
	}
	return out, nil
}

// eventKeyPrefix marks an aggregate's event stream among a log's keys.
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

// keepAll reads a schema-1 event as written: the legacy document still has
// the fields the current chat dropped.
func keepAll(_ context.Context, _ string, patches []byte) ([]byte, error) {
	return patches, nil
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
