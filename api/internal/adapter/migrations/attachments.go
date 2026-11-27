package migrations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
)

const StepChatAttachments = 8

const attachmentsDir = "attachments"

func planChatAttachments(_ context.Context, env migrate.Env) (migrate.Plan, error) {
	found, err := workspaceAttachments(env.Home)
	if err != nil || len(found) == 0 {
		return migrate.Plan{}, err
	}
	return migrate.Plan{Actions: []migrate.Action{{
		Kind: migrate.Convert, Subject: "chat attachments → chats/<id>/attachments", Count: int64(len(found)),
	}}}, nil
}

func moveChatAttachments(_ context.Context, env migrate.Env, cp *migrate.Checkpoints) (migrate.Counts, error) {
	found, err := workspaceAttachments(env.Home)
	if err != nil {
		return nil, err
	}
	for _, src := range found {
		rel, err := filepath.Rel(env.Home.Root(), src)
		if err != nil {
			return nil, fmt.Errorf("chat-attachments: %w", err)
		}
		if cp.Done(rel) {
			continue
		}
		chatID := filepath.Base(filepath.Dir(src))
		moved, kept, err := moveFiles(src, worktreepath.AttachmentsDir(env.Home.Root(), chatID))
		if err != nil {
			return nil, err
		}
		if len(kept) > 0 {
			detail := "kept, the name is taken in the chat's directory: " + strings.Join(kept, ", ")
			if err := cp.Note(rel, migrate.Note{Detail: detail}); err != nil {
				return nil, err
			}
		}
		if err := cp.Mark(rel, migrate.Counts{"moved_files": moved}); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func moveFiles(src, dst string) (int64, []string, error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return 0, nil, fmt.Errorf("chat-attachments: list %s: %w", src, err)
	}
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return 0, nil, fmt.Errorf("chat-attachments: %w", err)
	}
	var moved int64
	var kept []string
	for _, e := range entries {
		target := filepath.Join(dst, e.Name())
		if _, err := os.Lstat(target); err == nil {
			kept = append(kept, e.Name())
			continue
		}
		if err := os.Rename(filepath.Join(src, e.Name()), target); err != nil {
			return 0, nil, fmt.Errorf("chat-attachments: move %s: %w", e.Name(), err)
		}
		moved++
	}
	if len(kept) == 0 {
		removeEmpty(src, filepath.Dir(src))
	}
	return moved, kept, nil
}

func removeEmpty(dirs ...string) {
	for _, d := range dirs {
		if os.Remove(d) != nil {
			return
		}
	}
}

func workspaceAttachments(h home.Home) ([]string, error) {
	root := filepath.Join(h.Root(), "projects")
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		switch {
		case errors.Is(err, fs.ErrNotExist) && path == root:
			return fs.SkipAll
		case err != nil:
			return err
		case !d.IsDir():
			return nil
		case d.Name() == "worktree" || d.Name() == ".git":
			return fs.SkipDir
		case d.Name() == attachmentsDir && filepath.Base(filepath.Dir(filepath.Dir(path))) == "chats":
			found = append(found, path)
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("chat-attachments: scan %s: %w", root, err)
	}
	return found, nil
}
