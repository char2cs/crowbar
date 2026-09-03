package template_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/template"
)

func TestExpand_ReplacesEveryDeclaredPlaceholder(t *testing.T) {
	ctx := models.TemplateCtx{
		Tmp: "/tmp/seg", ID: "sess-1", Message: "hi", Context: "doc",
		ContextPointer: "see file", Cwd: "/work", CrowbarHook: "/bin/crowbar",
		Segid: "seg-1", RunnerToken: "tok", Provider: "claude",
		ProjectID: "p", RepoID: "r", WorkspaceID: "w", Socket: "/tmp/s.sock",
	}

	got := template.Expand(
		"{tmp}|{id}|{message}|{context}|{context_pointer}|{cwd}|{crowbar_hook}|"+
			"{crowbar}|{segid}|{runner_token}|{provider}|{project_id}|{repo_id}|{workspace_id}|{socket}",
		ctx,
	)

	assert.Equal(t,
		"/tmp/seg|sess-1|hi|doc|see file|/work|/bin/crowbar|"+
			"/bin/crowbar|seg-1|tok|claude|p|r|w|/tmp/s.sock", got)
}

func TestExpand_SocketPathIsShortEnoughForAUnixSocket(t *testing.T) {
	ctx := models.TemplateCtx{Socket: "/tmp/crowbar-71d4b349e29928c3.sock"}

	rendered := template.Expand("unix://{socket}", ctx)

	// macOS's sun_path is a hard 104 bytes — see [[project_dev_home_isolation]].
	// This asserts the CONVENTION (a short temp-dir socket, never a worktree
	// path) stays short once rendered, not that {socket} itself enforces a limit.
	assert.LessOrEqual(t, len(rendered)-len("unix://"), 104)
}

func TestExpand_LeavesUnknownPlaceholdersAlone(t *testing.T) {
	got := template.Expand("{not_a_token}", models.TemplateCtx{})

	assert.Equal(t, "{not_a_token}", got,
		"blanking a typo'd token would hide the mistake behind an empty argument")
}

func TestExpand_DoesNotRescanReplacementText(t *testing.T) {
	ctx := models.TemplateCtx{Message: "{cwd}", Cwd: "/secret"}

	got := template.Expand("{message}", ctx)

	assert.Equal(t, "{cwd}", got,
		"a user's message must stay literal data, never become argv structure")
}

func TestScopeFlags_OmitsRepoWhenTheWorkspaceHasNone(t *testing.T) {
	testCases := []struct {
		name string
		ctx  models.TemplateCtx
		want string
	}{
		{
			name: "repo-backed workspace",
			ctx:  models.TemplateCtx{ProjectID: "p", WorkspaceID: "w", RepoID: "r"},
			want: "--project=p --workspace=w --repo=r",
		},
		{
			name: "project-home workspace has no repo id",
			ctx:  models.TemplateCtx{ProjectID: "p", WorkspaceID: "w"},
			want: "--project=p --workspace=w",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.ctx.ScopeFlags())
		})
	}
}

func TestScopeFlags_AlwaysUsesTheEqualsFormSoAnEmptyValueCannotEatTheNextToken(t *testing.T) {
	got := models.TemplateCtx{ProjectID: "", WorkspaceID: "w"}.ScopeFlags()

	assert.Equal(t, "--project= --workspace=w", got)
	assert.NotContains(t, got, "--project --", "a space-separated empty value swallows its neighbour")
}
