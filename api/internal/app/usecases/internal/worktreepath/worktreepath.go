// Package worktreepath derives deterministic filesystem paths for git
// worktrees and per-entity directories, all rooted under ~/.crowbar. Paths are
// keyed by opaque UUIDs (projectID/repoID/workspaceID), never by remote URL.
package worktreepath

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

// For returns the git worktree directory for a workspace.
//
// Path: <crowbarHome>/projects/<projectID>/<repoID>/workspaces/<workspaceID>/worktree
func For(
	crowbarHome string,
	projectID string,
	repoID string,
	workspaceID string,
) string {
	return filepath.Join(
		workspaceDir(crowbarHome, projectID, repoID, workspaceID),
		"worktree",
	)
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

// AgentLedgerDir returns the per-chat agentic ledger directory under the
// workspace's Crowbar-managed storage (NOT inside the git worktree).
//
// Path: .../workspaces/<workspaceID>/agent-ledger/<chatID>
func AgentLedgerDir(
	crowbarHome string,
	projectID string,
	repoID string,
	workspaceID string,
	chatID string,
) string {
	return filepath.Join(
		workspaceDir(crowbarHome, projectID, repoID, workspaceID),
		"agent-ledger",
		chatID,
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
