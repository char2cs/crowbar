// Package worktreepath derives deterministic paths under ~/.crowbar: per-entity metadata dirs keyed by
// UUIDs, and human-readable worktree dirs keyed by remote identity so navigable paths carry no UUIDs.
// DetectClash rejects case-only collisions on case-insensitive filesystems.
package worktreepath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrPathClash reports that a candidate worktree path collides
// case-insensitively with an already-existing sibling worktree, which is
// unrepresentable on a case-insensitive filesystem (macOS APFS, Windows).
var ErrPathClash = errors.New("worktreepath: case-insensitive path clash")

// Derive returns <home>/projects/<project>/<slug>/<branch>/worktree for a workspace. The "worktree"
// leaf makes <slug>/<branch> a workspace root holding the worktree beside its "chats" tree, so agent
// state never shows in git status and one rm -rf of the root removes the whole footprint.
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
	// filepath.Join cleans traversal components (a remote-less repo's name slug can be "../../tmp"),
	// and a workspace outside <home>/projects is unreclaimable by every removal guard, so refuse it
	// here where the path is built rather than trusting callers.
	root := filepath.Join(home, "projects")
	path := filepath.Join(root, project, slug, branch, "worktree")
	if !UnderHome(path, root) {
		return "", fmt.Errorf(
			"worktreepath: derive escapes the crowbar home: %q is not under %q", path, root,
		)
	}
	return path, nil
}

// WorkspaceRoot returns the workspace directory that holds the worktree and
// the chats tree as siblings, given the worktree path.
func WorkspaceRoot(worktreePath string) string { return filepath.Dir(worktreePath) }

// ChatsDir returns a managed worktree's chats directory, beside the worktree so agent state never
// appears in git status. Only valid for worktrees under crowbar home: an adopted checkout must use
// HomeDefaultChatsDir so its chats never land on the user's filesystem.
func ChatsDir(worktreePath string) string {
	return filepath.Join(WorkspaceRoot(worktreePath), "chats")
}

// worktreeLeaf is the last component of every managed worktree path (Derive).
const worktreeLeaf = "worktree"

// OwnRoot returns the root holding a managed worktree path's "worktree" leaf, and false otherwise.
// Pre-leaf <slug>/<branch> rows are refused (their parent is shared by siblings), as are unclean
// paths and roots not strictly inside <home>/projects/<project>.
func OwnRoot(
	worktreePath string,
	home string,
) (string, bool) {
	if worktreePath == "" || home == "" || !filepath.IsAbs(worktreePath) ||
		filepath.Clean(worktreePath) != worktreePath || filepath.Base(worktreePath) != worktreeLeaf {
		return "", false
	}
	root := filepath.Dir(worktreePath)
	rel, err := filepath.Rel(filepath.Join(home, "projects"), root)
	if err != nil || !filepath.IsLocal(rel) || len(strings.Split(rel, string(filepath.Separator))) < 2 {
		return "", false
	}
	return root, true
}

// ManagedChatsDir resolves a home worktree's chats dir: beside a leaf-shaped worktree, or the legacy
// <slug>/chats of a pre-leaf row so its attachments stay readable — unless that dir is a checkout or
// outside the project, when it gets its own dir keyed by workspace id.
func ManagedChatsDir(
	home string,
	projectID string,
	workspaceID string,
	worktreePath string,
) string {
	if _, ok := OwnRoot(worktreePath, home); ok {
		return ChatsDir(worktreePath)
	}
	legacy := ChatsDir(filepath.Clean(worktreePath))
	project := ProjectDir(home, projectID)
	if projectID != "" && UnderHome(filepath.Dir(legacy), project) && !IsLiveCheckout(legacy) {
		return legacy
	}
	return filepath.Join(project, ".workspace-chats", workspaceID)
}

// HoldsAnother reports whether checkout is, or contains, another workspace's
// checkout or the chats tree beside it — as a pre-leaf branch named "chats"
// holds every sibling's. A forced removal of it would delete their files.
func HoldsAnother(
	checkout string,
	others []string,
) bool {
	checkout = filepath.Clean(checkout)
	for _, other := range others {
		if other == "" {
			continue
		}
		other = filepath.Clean(other)
		for _, claim := range []string{other, ChatsDir(other)} {
			if claim == checkout || UnderHome(claim, checkout) {
				return true
			}
		}
	}
	return false
}

// HomeDefaultChatsDir returns <home>/projects/<project>/<slug>/default/chats (blank slug collapses
// the <slug> level) for an adopted checkout, so no conversation ledger is ever written beside the
// user's real repository; the adopted worktree itself stays where it was imported.
func HomeDefaultChatsDir(
	home string,
	project string,
	slug string,
) string {
	return filepath.Join(home, "projects", project, slug, "default", "chats")
}

// RunnerDir returns a runner's per-spawn config dir, <chatsDir>/runners/<runnerID>-<provider>. It is
// keyed by the runner, not the chat, because a runner's chat pointer is erasable (Displace) while
// runner id and provider are not, so boot reconciliation can always find and reap it.
func RunnerDir(chatsDir, runnerID, provider string) string {
	return filepath.Join(chatsDir, "runners", runnerID+"-"+provider)
}

// LedgerChatsDir returns <crowbar home>/chats, under which each chat's directory is joined by id. It
// is never derived from WorkspaceID, which is optional and whose directory can move, so a ledger
// never moves out from under a chat that is writing to it.
func LedgerChatsDir(crowbarHome string) string {
	return filepath.Join(crowbarHome, "chats")
}

// UnderHome reports whether path is strictly nested under crowbarHome at a directory boundary; blank
// inputs, home itself and string-prefix siblings are not. It is the one notion of "under home" the
// chats-dir seam and the repository rm guard share.
func UnderHome(
	path string,
	home string,
) bool {
	if path == "" || home == "" {
		return false
	}
	return strings.HasPrefix(path, strings.TrimRight(home, "/")+"/")
}

// ResolvePath resolves symlinks in p, falling back to a lexical clean when p
// cannot be resolved (e.g. it no longer exists on disk).
func ResolvePath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
}

// SamePath reports whether two paths name the same location after resolving symlinks, since git
// worktree list emits resolved paths while a repo's Path is as imported.
func SamePath(a, b string) bool {
	return ResolvePath(a) == ResolvePath(b)
}

// IsLiveCheckout reports whether dir is a git checkout: it holds a `.git`
// entry (a file, for a linked worktree). A checkout git still has registered is
// git's to remove — never a delete's rm -rf.
func IsLiveCheckout(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

// SiblingRoots lists the existing workspace roots under a repo's slug directory so a create can
// reject a case-insensitive clash; a missing slug directory has none.
func SiblingRoots(
	crowbarHome string,
	projectID string,
	slug string,
) ([]string, error) {
	parent := filepath.Join(crowbarHome, "projects", projectID, slug)
	entries, err := os.ReadDir(parent)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(parent, entry.Name()))
	}
	return paths, nil
}

// DetectClash returns ErrPathClash when candidate is case-insensitively equal to any existing path:
// on a case-insensitive filesystem the two would share a directory, so creation is refused.
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

// maxPathBranchAttempts bounds the disambiguation search. It is a wedge ceiling,
// not a budget: reaching it means a hundred workspaces have frozen variants of
// one name, which is a home in trouble for other reasons.
const maxPathBranchAttempts = 100

// FreePathBranch returns branch, or branch-2, branch-3, … when taken: a workspace's directory is fixed
// at creation and never follows a rename, so an abandoned name must not block a new branch. The
// sibling scan catches case-only clashes; the directory test catches nested branches.
func FreePathBranch(
	home string,
	project string,
	slug string,
	branch string,
	siblings []string,
) (string, error) {
	for n := 1; n <= maxPathBranchAttempts; n++ {
		candidate := branch
		if n > 1 {
			candidate = fmt.Sprintf("%s-%d", branch, n)
		}
		path, err := Derive(home, project, slug, candidate)
		if err != nil {
			return "", err
		}
		root := WorkspaceRoot(path)
		if DetectClash(siblings, root) != nil {
			continue
		}
		if _, statErr := os.Lstat(root); statErr == nil {
			continue
		}
		return candidate, nil
	}
	return "", fmt.Errorf(
		"%w: no free directory for branch %q after %d attempts",
		ErrPathClash, branch, maxPathBranchAttempts)
}

// RemoveUnderHome deletes target only when strictly under crowbar home, and says why when it stays.
// It stops a poisoned or misconfigured chats dir from reaching the user's real repository.
func RemoveUnderHome(
	home string,
	target string,
) error {
	if !UnderHome(target, home) {
		return fmt.Errorf("worktreepath: refusing to remove %s: not under the crowbar home %s", target, home)
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("worktreepath: remove %s: %w", target, err)
	}
	return nil
}

// AttachmentsDir returns <chatsDir>/<chatID>/attachments, a chat's durable attachment store. Keyed by
// chat id (never erased), so deleting the chat's directory already removes it.
func AttachmentsDir(chatsDir, chatID string) string {
	return filepath.Join(chatsDir, chatID, "attachments")
}

// RestoreDurableAttachmentRefs rewrites durable-store absolute attachment paths in a hook's prompt
// back to logical chats/<chatID>/attachments/<file> refs, since a prompt typed into the CLI has no
// other copy of the original text. Text without such paths comes back unchanged.
func RestoreDurableAttachmentRefs(text, chatsDir, chatID string) string {
	absolutePrefix := filepath.ToSlash(AttachmentsDir(chatsDir, chatID))
	pattern := regexp.MustCompile(regexp.QuoteMeta(absolutePrefix) + `/([^/)\s]+)`)
	logicalPrefix := "chats/" + chatID + "/attachments"
	return pattern.ReplaceAllString(text, logicalPrefix+"/$1")
}
