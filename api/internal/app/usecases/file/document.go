package file

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
)

// foldCase matches worktree prefixes case-insensitively where the default
// filesystem is, so one file never gets two identities.
const foldCase = runtime.GOOS == "darwin"

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
// a worktree is that workspace's document; anywhere else it is a host document
// in its canonical spelling, served only over the local transport.
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

// refuseDaemonState refuses a path under the daemon's state directory: a save
// there would corrupt the live databases, and a read of them has no use.
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
	if _, inside := relUnder(safepath.Canonical(h.State()), canon); inside {
		return fmt.Errorf("file: document %s: %w", canon, apperr.ErrDaemonStateDocument)
	}
	return nil
}

// owningWorkspace picks the row whose worktree holds canon most closely. A
// project imported in place has its home and its repo at one path: the repo
// row wins that tie.
func owningWorkspace(
	rows []domain.Workspace,
	canon string,
) (domain.Workspace, string, bool) {
	var best domain.Workspace
	var bestRoot, bestRel string
	found := false
	for _, row := range rows {
		if row.WorktreePath == "" {
			continue
		}
		root := safepath.Canonical(row.WorktreePath)
		rel, inside := relUnder(root, canon)
		if !inside || (found && !beats(row, root, best, bestRoot)) {
			continue
		}
		best, bestRoot, bestRel, found = row, root, rel, true
	}
	return best, bestRel, found
}

func beats(
	row domain.Workspace,
	root string,
	best domain.Workspace,
	bestRoot string,
) bool {
	if len(root) != len(bestRoot) {
		return len(root) > len(bestRoot)
	}
	return row.Kind != domain.WorkspaceKindHome && best.Kind == domain.WorkspaceKindHome
}

// relUnder reports whether path is root or below it, on a separator boundary,
// and returns the rest relative to root.
func relUnder(
	root string,
	path string,
) (string, bool) {
	if root == "" || len(path) < len(root) || !samePrefix(path[:len(root)], root) {
		return "", false
	}
	rest := path[len(root):]
	if rest == "" || strings.HasSuffix(root, string(filepath.Separator)) {
		return rest, true
	}
	if rest[0] != filepath.Separator {
		return "", false
	}
	return rest[1:], true
}

func samePrefix(
	a string,
	b string,
) bool {
	if foldCase {
		return strings.EqualFold(a, b)
	}
	return a == b
}
