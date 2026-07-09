// Package worktreepath derives deterministic filesystem paths for git
// worktrees and per-entity directories, all rooted under ~/.crowbar.
//
// It exposes two path families. The per-entity helpers (StorageDir,
// ThreadsStorageDir, RepoDir, ...) key metadata directories by opaque UUIDs
// (projectID/repoID/workspaceID). The human-readable family (Derive, HomeLeaf,
// RemoteSlug) keys the git worktree by its natural identity —
// <home>/projects/<project>/<host>/<owner>/<repo>/<branch>/ — so navigable
// paths carry no UUIDs (spec §3.9). DetectClash rejects case-only collisions on
// case-insensitive filesystems and Move relocates a worktree while keeping the
// id↔path map consistent.
package worktreepath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

// ErrPathClash reports that a candidate worktree path collides
// case-insensitively with an already-existing sibling worktree, which is
// unrepresentable on a case-insensitive filesystem (macOS APFS, Windows).
var ErrPathClash = errors.New("worktreepath: case-insensitive path clash")

// Derive returns the human-readable git worktree directory for a workspace.
//
// Path: <home>/projects/<project>/<slug>/<branch>, where slug is the repo's
// full remote identity host/owner/repo (or a single leaf name for a repo with
// no remote). Slug and branch separators map to nested directories; branch
// names are not sanitized because git check-ref-format already forbids unsafe
// components (spec §3.9).
func Derive(
	home string,
	project string,
	slug string,
	branch string,
) (string, error) {
	if home == "" || project == "" || slug == "" || branch == "" {
		return "", fmt.Errorf(
			"worktreepath: derive requires non-empty home, project, slug, and branch",
		)
	}
	return filepath.Join(home, "projects", project, slug, branch), nil
}

// HomeLeaf returns the .home sibling leaf for a net-new Crowbar-managed
// repo-home worktree.
//
// Path: <home>/projects/<project>/<slug>/.home. The leading-dot leaf can never
// collide with a branch leaf because git check-ref-format rejects refnames with
// a leading-dot component (spec §3.9).
func HomeLeaf(
	home string,
	project string,
	slug string,
) string {
	return filepath.Join(home, "projects", project, slug, ".home")
}

// DetectClash returns ErrPathClash when candidate is case-insensitively equal
// to any path in existingPaths.
//
// On a case-insensitive filesystem two git-distinct case-only identities would
// resolve to the same directory, so creation is rejected rather than
// disambiguated (spec §3.9, decision 13).
func DetectClash(
	existingPaths []string,
	candidate string,
) error {
	for _, existing := range existingPaths {
		if strings.EqualFold(existing, candidate) {
			return fmt.Errorf(
				"%w: %q collides with existing %q",
				ErrPathClash,
				candidate,
				existing,
			)
		}
	}
	return nil
}

// Move relocates a worktree from oldPath to newPath via the injected gitMove
// (a git worktree move) and then commits the id↔path map update via updateMap.
//
// If gitMove fails the map is left untouched, so the old map entry still
// resolves the worktree (spec §3.9). IO is injected so this helper stays pure
// and testable.
func Move(
	oldPath string,
	newPath string,
	gitMove func(from, to string) error,
	updateMap func() error,
) error {
	if err := gitMove(oldPath, newPath); err != nil {
		return fmt.Errorf("worktreepath: git worktree move: %w", err)
	}
	if err := updateMap(); err != nil {
		return fmt.Errorf("worktreepath: update path map: %w", err)
	}
	return nil
}

// StorageDir returns the per-workspace storage directory.
//
// Path: .../workspaces/<workspaceID>/storages
func StorageDir(
	crowbarHome string,
	projectID string,
	repoID string,
	workspaceID string,
) string {
	return filepath.Join(
		workspaceDir(crowbarHome, projectID, repoID, workspaceID),
		"storages",
	)
}

// ThreadsStorageDir returns the per-workspace thread storage directory.
//
// Path: .../workspaces/<workspaceID>/threads/storages
func ThreadsStorageDir(
	crowbarHome string,
	projectID string,
	repoID string,
	workspaceID string,
) string {
	return filepath.Join(
		workspaceDir(crowbarHome, projectID, repoID, workspaceID),
		"threads",
		"storages",
	)
}

// RepoDir returns the per-repo directory.
//
// Path: <crowbarHome>/projects/<projectID>/<repoID>
func RepoDir(
	crowbarHome string,
	projectID string,
	repoID string,
) string {
	return filepath.Join(ProjectDir(crowbarHome, projectID), repoID)
}

// RepoStorageDir returns the per-repo storage directory.
//
// Path: .../projects/<projectID>/<repoID>/storages
func RepoStorageDir(
	crowbarHome string,
	projectID string,
	repoID string,
) string {
	return filepath.Join(RepoDir(crowbarHome, projectID, repoID), "storages")
}

// RepoIconPath returns the per-repo icon file path.
//
// Path: .../projects/<projectID>/<repoID>/icon
func RepoIconPath(
	crowbarHome string,
	projectID string,
	repoID string,
) string {
	return filepath.Join(RepoDir(crowbarHome, projectID, repoID), "icon")
}

// ProjectDir returns the per-project directory.
//
// Path: <crowbarHome>/projects/<projectID>
func ProjectDir(
	crowbarHome string,
	projectID string,
) string {
	return filepath.Join(crowbarHome, "projects", projectID)
}

// ProjectStorageDir returns the per-project storage directory.
//
// Path: .../projects/<projectID>/storages
func ProjectStorageDir(
	crowbarHome string,
	projectID string,
) string {
	return filepath.Join(ProjectDir(crowbarHome, projectID), "storages")
}

// GlobalStateDir returns the global state directory.
//
// Path: <crowbarHome>/state
func GlobalStateDir(crowbarHome string) string {
	return filepath.Join(crowbarHome, "state")
}

// DefaultCrowbarHome returns the root for all Crowbar-managed state: the
// CROWBAR_HOME env override when set (dev instances point it inside the
// workspace being developed), otherwise ~/.crowbar.
func DefaultCrowbarHome() (string, error) {
	if override := os.Getenv(metadata.HomeEnvVar); override != "" {
		return override, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("crowbar home: %w", err)
	}
	return filepath.Join(h, ".crowbar"), nil
}

func workspaceDir(
	crowbarHome string,
	projectID string,
	repoID string,
	workspaceID string,
) string {
	return filepath.Join(
		RepoDir(crowbarHome, projectID, repoID),
		"workspaces",
		workspaceID,
	)
}
