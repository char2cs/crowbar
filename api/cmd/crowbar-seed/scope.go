package main

import "fmt"

// scope is the project/repo/workspace triple review threads are nested under,
// the one workspace-scoped route shape this tool still calls.
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

// chatsPath is the repo-scoped chat list/create route. GET is already scoped
// to the repo server-side, so there is no repo id for a caller to filter by.
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
