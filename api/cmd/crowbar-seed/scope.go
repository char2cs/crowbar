package main

import "fmt"

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

func chatsPath(
	projectID string,
	repoID string,
) string {
	return repoScopePath(projectID, repoID, "/chats")
}

func chatDetailPath(
	projectID string,
	repoID string,
	chatID string,
) string {
	return repoScopePath(projectID, repoID, "/chats/"+chatID)
}

func flatChatPath(
	chatID string,
	suffix string,
) string {
	return "/v0/chats/" + chatID + suffix
}
