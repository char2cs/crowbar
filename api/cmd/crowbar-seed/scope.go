package main

import "fmt"

// scope is the project/repo/workspace triple review threads are still nested
// under. Every other workspace-scoped route this tool once called was deleted
// when the API moved to a chat-scoped surface (spec §8 step 6) — threads
// alone never moved (repo-level review commentary, spec §4.4) — so this is
// the one shape that survives unchanged.
type scope struct {
	projectID   string
	repoID      string
	workspaceID string
}

func (s scope) path(
	suffix string,
) string {
	return fmt.Sprintf(
		"/v0/projects/%s/repos/%s/workspaces/%s%s",
		s.projectID, s.repoID, s.workspaceID, suffix,
	)
}

func repoScopePath(
	projectID string,
	repoID string,
	suffix string,
) string {
	return fmt.Sprintf("/v0/projects/%s/repos/%s%s", projectID, repoID, suffix)
}

// chatsPath is the repo-scoped chat list/create route. GET lists every chat
// the repo owns — already scoped server-side (ListChatsInRepo's cwd walk), so
// unlike the deleted workspace list there is no repo id left for a caller to
// filter by — and POST mints one.
func chatsPath(
	projectID string,
	repoID string,
) string {
	return repoScopePath(projectID, repoID, "/chats")
}

// chatDetailPath addresses one chat and its lifecycle verbs, e.g.
// chatDetailPath(...)+"/branch" or +"/stop".
func chatDetailPath(
	projectID string,
	repoID string,
	chatID string,
) string {
	return repoScopePath(projectID, repoID, "/chats/"+chatID)
}

// flatChatPath addresses the shared-bucket surface spec §4.2 moved off the
// workspace group entirely (git, files, review, search, identity, editor/LSP):
// no project/repo prefix, just the chat that owns the worktree.
func flatChatPath(
	chatID string,
	suffix string,
) string {
	return "/v0/chats/" + chatID + suffix
}
