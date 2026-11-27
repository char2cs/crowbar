//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireHomeRESTWorking(
	t *testing.T,
	h *harness,
	projectID string,
	want bool,
) {
	t.Helper()
	var home workspaceDTO
	h.get("/v0/projects/"+projectID+"/home", &home)
	require.NotEmpty(t, home.ID, "GET /home must resolve the project-home workspace")
	require.Equalf(t, want, home.Working, "REST GET /home working for project %s", projectID)
}

func TestRegression_HomeWorkspaceWorkingReflectsAgentTurn(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	projectID := createProjectForHome(t, h)
	homeBase := "/v0/projects/" + projectID + "/home"

	feed := recordSidebar(t, h)

	var created struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	feed.awaitListed(created.ID)
	startChat(t, h, homeBase, created.ID)

	detail := getAgentChat(t, h, homeBase, created.ID)
	require.NotEmpty(t, detail.LiveRunnerID, "the freshly spawned chat must have a runner placed on it")
	segID := detail.LiveRunnerID

	requireHomeRESTWorking(t, h, projectID, false)

	postHook(t, h, homeBase, "livestub", segID, "user_prompt", `{"prompt":"hi"}`)
	feed.awaitTurnStart(created.ID)
	h.Quiesce()
	requireHomeRESTWorking(t, h, projectID, true)

	postHook(t, h, homeBase, "livestub", segID, "turn_stop", `{"last_assistant_message":"done"}`)
	feed.awaitTurnEnd(created.ID)
	h.Quiesce()
	requireHomeRESTWorking(t, h, projectID, false)
}
