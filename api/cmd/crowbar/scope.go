package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// scopedAgentPath builds the agent API path for project/repo, appending suffix
// after the .../chats segment; workspace is ignored. A project home has no repo
// id and is mounted under /v0/projects/:projectId/home/chats instead.
func scopedAgentPath(
	project, repo, workspace, suffix string,
) string {
	if repo == "" {
		return "/v0/projects/" + project + "/home/chats" + suffix
	}
	return "/v0/projects/" + project + "/repos/" + repo + "/chats" + suffix
}

// bindScopeFlags registers the scope flags shared by every in-PTY CLI callback.
// --workspace is ignored but still accepted since {scope_flags} renders it; a project
// home omits --repo, because the shell drops a bare `--repo ` and pflag eats the next token.
func bindScopeFlags(
	cmd *cobra.Command,
	project, repo, workspace, homeFlag, host *string,
) {
	cmd.Flags().StringVar(project, "project", "", "project id")
	cmd.Flags().StringVar(repo, "repo", "", "repo id")
	cmd.Flags().StringVar(workspace, "workspace", "", "workspace id")
	cmd.Flags().StringVar(homeFlag, "home", "", "crowbar home this callback must operate against")
	cmd.Flags().StringVar(host, "host", "unix://", "daemon address to dial; the spawn hands over the socket the daemon bound")
}

// applyHomeOverride sets CROWBAR_HOME for this short-lived callback process, so
// the home baked into the spawn command line wins over whatever the vendor CLI
// forwards; without it a dev instance's callbacks could fall back to ~/.crowbar.
func applyHomeOverride(override string) error {
	if override == "" {
		return nil
	}
	if err := os.Setenv(home.EnvVar, override); err != nil {
		return fmt.Errorf("set %s: %w", home.EnvVar, err)
	}
	return nil
}
