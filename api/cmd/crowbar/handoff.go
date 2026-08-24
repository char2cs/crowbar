package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

func newHandoffCmd() *cobra.Command {
	handoff := &cobra.Command{
		Use:   "handoff",
		Short: "Inspect an agentic chat's handed-off context",
	}
	handoff.AddCommand(newHandoffDumpCmd())
	return handoff
}

func newHandoffDumpCmd() *cobra.Command {
	var project, repo, workspace string
	cmd := &cobra.Command{
		Use:   "dump <chatId>",
		Short: "Print a chat's assembled handoff to stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runHandoffDump(args[0], project, repo, workspace, "unix://", os.Stdout)
		},
	}
	bindScopeFlags(cmd, &project, &repo, &workspace)
	return cmd
}

// runHandoffDump fetches chatID's assembled handoff from the Crowbar daemon
// over its IPC socket and writes it to out. It is factored out of the cobra
// RunE so tests can exercise it against a stub daemon without going through
// cobra or the real socket. The target URL is scoped to project/repo/workspace
// (Task 3 nested the agent routes under the workspace group; this is a
// manual/debug command, so a human invoking it passes the scope flags
// explicitly).
func runHandoffDump(chatID, project, repo, workspace, host string, out io.Writer) error {
	client, err := ipc.NewClient(host)
	if err != nil {
		return err
	}

	status, body, err := client.Get(context.Background(),
		scopedAgentPath(project, repo, workspace, "/"+chatID+"/handoff"))
	if err != nil {
		return err
	}

	var envelope struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
		Data    struct {
			Handoff string `json:"handoff"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("handoff: decode response (status %d): %w", status, err)
	}
	if !envelope.Success {
		return fmt.Errorf("handoff: %s", envelope.Error)
	}

	_, err = fmt.Fprint(out, envelope.Data.Handoff)
	return err
}
