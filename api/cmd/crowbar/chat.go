package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

func newChatCmd() *cobra.Command {
	var project, repo, workspace, segment string
	chat := &cobra.Command{
		Use:    "chat",
		Short:  "Manage Crowbar agent chats",
		Hidden: true,
	}
	rename := &cobra.Command{
		Use:   "rename <title>",
		Short: "Set the title of the chat the calling runner is on right now",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			// Run by the agent — must never break its turn: swallow to exit-0,
			// stderr only (never stdout).
			if err := runChatRename(segment, args[0], project, repo, workspace, "unix://"); err != nil {
				fmt.Fprintf(os.Stderr, "crowbar chat rename --segment %s: %v\n", segment, err)
			}
			return nil
		},
	}
	rename.Flags().StringVar(&segment, "segment", "", "Crowbar segment id")
	bindScopeFlags(rename, &project, &repo, &workspace)
	chat.AddCommand(rename)
	return chat
}

// runChatRename posts a title to the daemon with source=agent (agent precedence:
// upgrade a derived title, never clobber a user-locked one), against the RUNNER
// named by segment — never a chat id. The chat id used to be baked into this
// command at spawn time, so an agent that titled itself after moving to a
// different chat (a /clear or /resume issued inside it) renamed the chat it
// used to be on. The daemon resolves segment → runner → its CURRENT chat at
// call time (see (*agent.Usecase).RenameByRunner), so nothing here can go
// stale. The target URL is scoped to project/repo/workspace (Task 3 nested the
// agent routes under the workspace group; the vendor CLI's rename instruction
// passes these ids as explicit flags so the callback can rebuild the nested
// path).
func runChatRename(segment, title, project, repo, workspace, host string) error {
	client, err := ipc.NewClient(host)
	if err != nil {
		return err
	}
	_, _, err = client.PostJSON(context.Background(),
		scopedAgentPath(project, repo, workspace, "/runners/"+segment+"/rename?source=agent"),
		map[string]any{"title": title})
	return err
}
