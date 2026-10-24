package file

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

// ReadDocument reads a document. A host document is read only for a request
// that came over the local socket.
func (u *fileUsecase) ReadDocument(
	ctx context.Context,
	target domain.DocumentRef,
	transport domain.Transport,
) (domain.Document, error) {
	ref, err := u.resolveDocument(ctx, target, transport)
	if err != nil {
		return domain.Document{}, err
	}
	fc, err := u.readResolved(ctx, ref)
	if err != nil {
		return domain.Document{}, fmt.Errorf("file: read document: %w", err)
	}
	return domain.Document{DocumentRef: ref, FileContent: fc}, nil
}

func (u *fileUsecase) readResolved(
	ctx context.Context,
	ref domain.DocumentRef,
) (domain.FileContent, error) {
	if ref.Authority == domain.DocumentAuthorityHost {
		return u.fs.ReadHostContent(ref.Path)
	}
	repoPath, err := u.repoPath(ctx, ref.WorkspaceID)
	if err != nil {
		return domain.FileContent{}, err
	}
	return u.fs.ReadContent(repoPath, ref.Path)
}

// WriteDocument writes a document and returns the ref it resolved to. Only a
// workspace write resyncs: a host file belongs to no working tree.
func (u *fileUsecase) WriteDocument(
	ctx context.Context,
	target domain.DocumentRef,
	content string,
	encoding string,
	transport domain.Transport,
	now time.Time,
) (domain.DocumentRef, error) {
	ref, err := u.resolveDocument(ctx, target, transport)
	if err != nil {
		return domain.DocumentRef{}, err
	}
	if ref.Authority == domain.DocumentAuthorityHost {
		if err := u.fs.WriteHostContent(ref.Path, content, encoding); err != nil {
			return domain.DocumentRef{}, fmt.Errorf("file: write document: %w", err)
		}
		return ref, nil
	}
	repoPath, err := u.writePath(ctx, ref.WorkspaceID)
	if err != nil {
		return domain.DocumentRef{}, err
	}
	if err := u.fs.WriteContent(repoPath, ref.Path, content, encoding); err != nil {
		return domain.DocumentRef{}, fmt.Errorf("file: write document: %w", err)
	}
	if err := u.resync(ctx, ref.WorkspaceID, now); err != nil {
		return domain.DocumentRef{}, err
	}
	return ref, nil
}

// resolveDocument names the one authority for target. An absolute path inside
// a worktree, by file identity, is that workspace's document; anywhere else it
// is a host document in its canonical spelling, served only over the local
// transport.
func (u *fileUsecase) resolveDocument(
	ctx context.Context,
	target domain.DocumentRef,
	transport domain.Transport,
) (domain.DocumentRef, error) {
	if target.WorkspaceID != "" {
		return domain.DocumentRef{Authority: domain.DocumentAuthorityWorkspace, WorkspaceID: target.WorkspaceID, Path: target.Path}, nil
	}
	if !filepath.IsAbs(target.Path) {
		return domain.DocumentRef{}, fmt.Errorf("file: document %q: no workspace and not absolute: %w", target.Path, apperr.ErrInvalidArgument)
	}
	canon := safepath.Canonical(target.Path)
	if err := u.refuseDaemonState(canon); err != nil {
		return domain.DocumentRef{}, err
	}
	rows, err := u.syncer.List(ctx)
	if err != nil {
		return domain.DocumentRef{}, fmt.Errorf("file: document: list workspaces: %w", err)
	}
	if owner, rel, ok := owningWorkspace(rows, canon); ok {
		return domain.DocumentRef{Authority: domain.DocumentAuthorityWorkspace, WorkspaceID: owner.ID, Path: rel}, nil
	}
	if transport != domain.TransportLocal {
		return domain.DocumentRef{}, fmt.Errorf("file: document: %w", apperr.ErrHostDocumentNeedsLocalTransport)
	}
	return domain.DocumentRef{Authority: domain.DocumentAuthorityHost, Path: canon}, nil
}

// refuseDaemonState refuses a path inside the daemon's state directory, by file
// identity: a save there would corrupt the live databases, and a read of them
// has no use.
func (u *fileUsecase) refuseDaemonState(
	canon string,
) error {
	root, err := u.crowbarHome()
	if err != nil {
		return fmt.Errorf("file: document: crowbar home: %w", err)
	}
	h, err := home.At(root)
	if err != nil {
		return fmt.Errorf("file: document: crowbar home: %w", err)
	}
	if hits, _ := safepath.Locate(canon, []string{h.State()}); len(hits) > 0 {
		return fmt.Errorf("file: document %s: %w", canon, apperr.ErrDaemonStateDocument)
	}
	return nil
}

// owningWorkspace picks the row whose worktree is canon's nearest ancestor by
// file identity. A project imported in place has its home and its repo in one
// directory: the repo row wins that tie.
func owningWorkspace(
	rows []domain.Workspace,
	canon string,
) (domain.Workspace, string, bool) {
	if row, rel, ok := missingWorktreeOwner(rows, canon); ok {
		return row, rel, true
	}
	roots := make([]string, len(rows))
	for i, row := range rows {
		roots[i] = row.WorktreePath
	}
	hits, rel := safepath.Locate(canon, roots)
	if len(hits) == 0 {
		return domain.Workspace{}, "", false
	}
	for _, i := range hits {
		if rows[i].Kind != domain.WorkspaceKindHome {
			return rows[i], rel, true
		}
	}
	return rows[hits[0]], rel, true
}

// missingWorktreeOwner is the row whose worktree is gone from disk and whose
// canonical path is canon's longest prefix. A missing root has no identity to
// compare, and any existing root on canon's path lies above it.
func missingWorktreeOwner(
	rows []domain.Workspace,
	canon string,
) (domain.Workspace, string, bool) {
	var owner domain.Workspace
	var ownerRel string
	longest := 0
	for _, row := range rows {
		if row.WorktreePath == "" {
			continue
		}
		if _, err := os.Stat(row.WorktreePath); !errors.Is(err, os.ErrNotExist) {
			continue
		}
		root := safepath.Canonical(row.WorktreePath)
		if rel, ok := pathBelow(root, canon); ok && len(root) > longest {
			owner, ownerRel, longest = row, rel, len(root)
		}
	}
	return owner, ownerRel, longest > 0
}

// pathBelow is path relative to root when root is a whole-name prefix of it;
// darwin volumes fold case by default, so the prefix does too there.
func pathBelow(
	root string,
	path string,
) (string, bool) {
	prefix := root + string(filepath.Separator)
	if len(path) <= len(prefix) {
		return "", false
	}
	head := path[:len(prefix)]
	if head != prefix && (runtime.GOOS != "darwin" || !strings.EqualFold(head, prefix)) {
		return "", false
	}
	return filepath.ToSlash(path[len(prefix):]), true
}
