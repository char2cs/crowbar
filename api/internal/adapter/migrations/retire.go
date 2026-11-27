package migrations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/char2cs/crowbar/api/internal/adapter/migrations/chatledger"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

var retiredLogs = []string{
	"agent_activity", "agent_activity_snapshots", "agent_runner", "agent_runner_snapshots", "node", "node_snapshots",
}

const keptNote = "kept "

const retireSubject = "legacy files the chat ledger and the project trees replaced"

type legacyFile struct {
	rel    string
	verify bool
	stamp  string
}

func legacyFiles(
	h home.Home,
) ([]legacyFile, error) {
	var paths []string
	for _, dir := range []string{h.Events(), h.Store()} {
		for _, name := range retiredLogs {
			for _, suffix := range []string{"", "-wal", "-shm"} {
				paths = append(paths, filepath.Join(dir, name+".db"+suffix))
			}
		}
	}
	trees, err := filepath.Glob(filepath.Join(h.Root(), "chats", "*", chatledger.PromptsDir))
	if err != nil {
		return nil, fmt.Errorf("retire-legacy: %w", err)
	}
	trees = append(trees, filepath.Join(h.State(), markersDir))
	files, err := filesUnder(trees)
	if err != nil {
		return nil, err
	}
	paths = append(paths, files...)
	paths = append(paths, filepath.Join(h.Root(), "bin", "crowbar"))
	return describe(h, paths)
}

func filesUnder(
	trees []string,
) ([]string, error) {
	var out []string
	for _, tree := range trees {
		err := filepath.WalkDir(tree, func(path string, entry fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) && path == tree {
				return filepath.SkipDir
			}
			if err == nil && entry.Type().IsRegular() {
				out = append(out, path)
			}
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("retire-legacy: %w", err)
		}
	}
	return out, nil
}

func describe(
	h home.Home,
	paths []string,
) ([]legacyFile, error) {
	var out []legacyFile
	for _, path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("retire-legacy: %w", err)
		}
		rel, err := filepath.Rel(h.Root(), path)
		if err != nil {
			return nil, fmt.Errorf("retire-legacy: %w", err)
		}
		rel = filepath.ToSlash(rel)
		spare := strings.HasSuffix(rel, "-shm") || strings.HasSuffix(rel, "-wal") && info.Size() == 0 || rel == "bin/crowbar"
		out = append(out, legacyFile{
			rel: rel, verify: !spare,
			stamp: keptNote + strconv.FormatInt(info.Size(), 10) + " " + strconv.FormatInt(info.ModTime().UnixNano(), 10),
		})
	}
	return out, nil
}

func retirable(
	env migrate.Env,
	noted func(key string) (migrate.Note, bool),
) ([]legacyFile, error) {
	ledgerStep := migrate.ID{Number: StepChatLedger, Name: "chat-ledger"}
	if !env.History.CompletedAt(1) || !chatledger.Settled(env.History.Marks(ledgerStep), env.History.Notes(ledgerStep)) {
		return nil, nil
	}
	files, err := legacyFiles(env.Home)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(files, func(f legacyFile) bool {
		note, ok := noted(f.rel)
		return ok && note.Detail == f.stamp
	}), nil
}

func planRetire(
	_ context.Context,
	env migrate.Env,
) (migrate.Plan, error) {
	files, err := retirable(env, func(key string) (migrate.Note, bool) { n, ok := env.Notes[key]; return n, ok })
	if err != nil || len(files) == 0 {
		return migrate.Plan{}, err
	}
	return migrate.Plan{Actions: []migrate.Action{{Kind: migrate.Delete, Subject: retireSubject, Count: int64(len(files))}}}, nil
}

func RetireImpl() migrate.Impl {
	return migrate.Impl{Apply: applyRetire}
}

func applyRetire(
	ctx context.Context,
	env migrate.Env,
	cp *migrate.Checkpoints,
) (migrate.Counts, error) {
	files, err := retirable(env, cp.Noted)
	if err != nil || len(files) == 0 {
		return nil, err
	}
	same, err := backedUp(env, files)
	if err != nil {
		return nil, err
	}
	counts := migrate.Counts{}
	for _, f := range files {
		if f.verify && !same[f.rel] {
			env.Log.WarnContext(ctx, "retire-legacy: kept a legacy file the backup does not hold as it is", "file", f.rel)
			if err := cp.Note(f.rel, migrate.Note{Detail: f.stamp}); err != nil {
				return counts, err
			}
			counts["kept_files"]++
			continue
		}
		size, err := remove(env.Home, f.rel)
		if err != nil {
			return counts, err
		}
		counts["deleted_files"]++
		counts["deleted_bytes"] += size
	}
	env.Log.InfoContext(ctx, "retire-legacy: deleted the legacy files", "counts", counts)
	return counts, pruneEmptyDirs(env.Home)
}

func backedUp(
	env migrate.Env,
	files []legacyFile,
) (map[string]bool, error) {
	backup, err := env.History.Backup()
	if err != nil || backup == "" {
		return map[string]bool{}, err
	}
	var rels []string
	for _, f := range files {
		if f.verify {
			rels = append(rels, f.rel)
		}
	}
	return migrate.SameInBackup(backup, env.Home.Root(), rels)
}

func remove(
	h home.Home,
	rel string,
) (int64, error) {
	path := filepath.Join(h.Root(), filepath.FromSlash(rel))
	info, err := os.Lstat(path)
	if err != nil {
		return 0, fmt.Errorf("retire-legacy: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return 0, fmt.Errorf("retire-legacy: %w", err)
	}
	return info.Size(), nil
}

func pruneEmptyDirs(
	h home.Home,
) error {
	journals, err := filepath.Glob(filepath.Join(h.Root(), "chats", "*", chatledger.PromptsDir))
	if err != nil {
		return fmt.Errorf("retire-legacy: %w", err)
	}
	var dirs []string
	for _, journal := range journals {
		dirs = append(dirs, filepath.Dir(journal))
	}
	dirs = append(dirs, filepath.Join(h.Root(), "chats"), filepath.Join(h.State(), markersDir), filepath.Join(h.Root(), "bin"))
	for _, dir := range dirs {
		if err := removeEmptyTree(dir); err != nil {
			return err
		}
	}
	return nil
}

func removeEmptyTree(
	dir string,
) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("retire-legacy: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			if err := removeEmptyTree(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}
		}
	}
	if left, err := os.ReadDir(dir); err != nil || len(left) > 0 {
		return err
	}
	if err := os.Remove(dir); err != nil {
		return fmt.Errorf("retire-legacy: %w", err)
	}
	return nil
}
