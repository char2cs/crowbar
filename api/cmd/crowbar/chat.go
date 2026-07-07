package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

func newChatCmd() *cobra.Command {
	chat := &cobra.Command{
		Use:    "chat",
		Short:  "Manage Crowbar agent chats",
		Hidden: true,
	}
	rename := &cobra.Command{
		Use:   "rename <chatid> <title>",
		Short: "Set an agent chat's title",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			// Run by the agent — must never break its turn: swallow to exit-0,
			// stderr only (never stdout).
			if err := runChatRename(args[0], args[1], "unix://"); err != nil {
				fmt.Fprintf(os.Stderr, "crowbar chat rename %s: %v\n", args[0], err)
			}
			return nil
		},
	}
	chat.AddCommand(rename)
	return chat
}

// runChatRename posts a title to the daemon with source=agent (agent precedence:
// upgrade a derived title, never clobber a user-locked one).
func runChatRename(chatID, title, host string) error {
	client, err := ipc.NewClient(host)
	if err != nil {
		return err
	}
	_, _, err = client.PostJSON(context.Background(),
		"/v0/agent/chats/"+chatID+"/rename?source=agent",
		map[string]any{"title": title})
	return err
}
