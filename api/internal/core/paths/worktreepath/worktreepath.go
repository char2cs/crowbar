package worktreepath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var ErrPathClash = errors.New("worktreepath: case-insensitive path clash")

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
	root := filepath.Join(home, "projects")
	path := filepath.Join(root, project, slug, branch, "worktree")
	if !UnderHome(path, root) {
		return "", fmt.Errorf(
			"worktreepath: derive escapes the crowbar home: %q is not under %q", path, root,
		)
	}
	return path, nil
}

func WorkspaceRoot(worktreePath string) string { return filepath.Dir(worktreePath) }

func ChatsDir(worktreePath string) string {
	return filepath.Join(WorkspaceRoot(worktreePath), "chats")
}

const worktreeLeaf = "worktree"

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

func HomeDefaultChatsDir(
	home string,
	project string,
	slug string,
) string {
	return filepath.Join(home, "projects", project, slug, "default", "chats")
}

func RunnerDir(chatsDir, runnerID, provider string) string {
	return filepath.Join(chatsDir, "runners", runnerID+"-"+provider)
}

func LedgerChatsDir(crowbarHome string) string {
	return filepath.Join(crowbarHome, "chats")
}

func UnderHome(
	path string,
	home string,
) bool {
	if path == "" || home == "" {
		return false
	}
	return strings.HasPrefix(path, strings.TrimRight(home, "/")+"/")
}

func ResolvePath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
}

func SamePath(a, b string) bool {
	return ResolvePath(a) == ResolvePath(b)
}

func IsLiveCheckout(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

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

func RepoDir(
	crowbarHome string,
	projectID string,
	repoID string,
) string {
	return filepath.Join(ProjectDir(crowbarHome, projectID), repoID)
}

func RepoIconPath(
	crowbarHome string,
	projectID string,
	repoID string,
) string {
	return filepath.Join(RepoDir(crowbarHome, projectID, repoID), "icon")
}

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

const maxPathBranchAttempts = 100

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

func AttachmentsDir(crowbarHome, chatID string) string {
	return filepath.Join(LedgerChatsDir(crowbarHome), chatID, "attachments")
}

func RelayDir(crowbarHome, chatID string) string {
	return filepath.Join(LedgerChatsDir(crowbarHome), chatID, "relay")
}

func RestoreDurableAttachmentRefs(text, crowbarHome, chatID string) string {
	absolutePrefix := filepath.ToSlash(AttachmentsDir(crowbarHome, chatID))
	pattern := regexp.MustCompile(regexp.QuoteMeta(absolutePrefix) + `/([^/)\s]+)`)
	logicalPrefix := "chats/" + chatID + "/attachments"
	return pattern.ReplaceAllString(text, logicalPrefix+"/$1")
}
