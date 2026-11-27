package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

func scopedAgentPath(
	project, repo, workspace, suffix string,
) string {
	if repo == "" {
		return "/v0/projects/" + project + "/home/chats" + suffix
	}
	return "/v0/projects/" + project + "/repos/" + repo + "/chats" + suffix
}

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

func applyHomeOverride(override string) error {
	if override == "" {
		return nil
	}
	if err := os.Setenv(home.EnvVar, override); err != nil {
		return fmt.Errorf("set %s: %w", home.EnvVar, err)
	}
	return nil
}
