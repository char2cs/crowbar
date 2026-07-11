// Package worktreepath derives deterministic filesystem paths for git
// worktrees and per-entity directories, all rooted under ~/.crowbar.
//
// It exposes two path families. The per-entity helpers (StorageDir,
// ThreadsStorageDir, RepoDir, ...) key metadata directories by opaque UUIDs
// (projectID/repoID/workspaceID). The human-readable family (Derive,
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
// Path: <home>/projects/<project>/<slug>/<branch>/worktree, where slug is the
// repo's full remote identity host/owner/repo (or a single leaf name for a
// repo with no remote). Slug and branch separators map to nested directories;
// branch names are not sanitized because git check-ref-format already forbids
// unsafe components (spec §3.9). The trailing "worktree" leaf makes the
// <slug>/<branch> directory a WORKSPACE ROOT: it holds the git worktree and
// the sibling "chats" tree (see WorkspaceRoot/ChatsDir) side by side, so
// agentic chat state never lands inside the git worktree (never shows up in
// git status) and the whole workspace's on-disk footprint is removed by a
// single rm -rf of the root.
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
	return filepath.Join(home, "projects", project, slug, branch, "worktree"), nil
}

// WorkspaceRoot returns the workspace directory that holds the worktree and
// the chats tree as siblings, given the worktree path.
func WorkspaceRoot(worktreePath string) string { return filepath.Dir(worktreePath) }

// ChatsDir returns the per-workspace agentic chats directory for a MANAGED
// worktree: the sibling of the worktree, NOT inside it (so agent state never
// appears in git status). It is valid ONLY when the worktree is itself under
// crowbar home; an adopted checkout (repo-home/project-home) whose worktree is
// the user's real dir OUTSIDE home must NOT use this (its chats would land on the
// user's filesystem) — it reroots under home via HomeDefaultChatsDir instead. The
// kind-aware choice between the two lives in the agent WorkspaceReader seam.
func ChatsDir(worktreePath string) string {
	return filepath.Join(WorkspaceRoot(worktreePath), "chats")
}

// HomeDefaultChatsDir returns the agentic chats directory for an adopted checkout
// (repo-home/project-home), rooted strictly under crowbar home so no plaintext
// conversation ledger is ever written beside the user's real repository (Task 7).
//
// Path: <home>/projects/<project>/<slug>/default/chats, where slug is the repo's
// on-disk identity (worktreepath.RemoteSlug). A blank slug (the project-level home
// has no repo) collapses to <home>/projects/<project>/default/chats — still under
// home, never the user's real dir. The worktree/Cwd is unaffected: only the chats
// tree moves, the adopted worktree stays exactly where the user imported it.
func HomeDefaultChatsDir(
	home string,
	project string,
	slug string,
) string {
	return filepath.Join(home, "projects", project, slug, "default", "chats")
}

// AgentChatDir returns a chat's own directory ({chat_dir}) under an already
// resolved chats directory. It holds everything that must outlive a single
// segment: the handoff ledger, and any per-provider state a CLI keeps between
// spawns (codex's CODEX_HOME, whose rollouts are what `codex resume` reads —
// putting that under the per-segment SegmentDir destroyed codex's own session
// the moment it was switched away from). Removed only when the chat is deleted
// (PurgeChat / the workspace-delete cascade).
func AgentChatDir(chatsDir, chatID string) string {
	return filepath.Join(chatsDir, chatID)
}

// AgentLedgerDir returns the per-chat handoff-ledger directory under an already
// resolved chats directory.
func AgentLedgerDir(chatsDir, chatID string) string {
	return filepath.Join(chatsDir, chatID, "ledger")
}

// SegmentDir returns a segment's per-spawn config/transcript directory ({tmp})
// under an already resolved chats directory.
func SegmentDir(chatsDir, chatID, segmentID, provider string) string {
	return filepath.Join(chatsDir, chatID, segmentID+"-"+provider)
}

// UnderHome reports whether path is strictly nested under crowbarHome — i.e. home
// is a proper directory-boundary prefix of path. A blank path or home, home
// itself, or a mere string-prefix sibling (…/.crowbar-other) is never under home.
//
// It is the shared predicate the agent chats-dir seam uses to distinguish a
// Crowbar-managed worktree (chats stay beside it, ChatsDir) from an adopted
// checkout at the user's real dir (chats reroot under home, HomeDefaultChatsDir),
// mirroring the repository-layer managedWorktreePath rm guard so both converge on
// exactly one notion of "under home".
func UnderHome(
	path string,
	home string,
) bool {
	if path == "" || home == "" {
		return false
	}
	return strings.HasPrefix(path, strings.TrimRight(home, "/")+"/")
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
