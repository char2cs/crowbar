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
	return &cobra.Command{
		Use:   "dump <chatId>",
		Short: "Print a chat's assembled handoff to stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runHandoffDump(args[0], "unix://", os.Stdout)
		},
	}
}

// runHandoffDump fetches chatID's assembled handoff from the Crowbar daemon
// over its IPC socket and writes it to out. It is factored out of the cobra
// RunE so tests can exercise it against a stub daemon without going through
// cobra or the real socket.
func runHandoffDump(chatID, host string, out io.Writer) error {
	client, err := ipc.NewClient(host)
	if err != nil {
		return err
	}

	status, body, err := client.Get(context.Background(), "/v0/agent/chats/"+chatID+"/handoff")
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
