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

func newHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "hook <event>",
		Short:  "Forward a vendor-CLI hook payload to the Crowbar daemon",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(_ *cobra.Command, args []string) error {
			// A hook must never break the vendor CLI: swallow errors, exit 0.
			_ = runHook(args[0], os.Stdin, "unix://")
			return nil
		},
	}
}

func runHook(event string, stdin io.Reader, host string) error {
	raw, err := io.ReadAll(io.LimitReader(stdin, 8<<20))
	if err != nil {
		return fmt.Errorf("hook: read stdin: %w", err)
	}
	var payload any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			payload = map[string]any{"_raw": string(raw)} // tolerate non-JSON (argv-delivered variants)
		}
	}
	client, err := ipc.NewClient(host)
	if err != nil {
		return err
	}
	body := map[string]any{
		"segment_id": os.Getenv("CROWBAR_SEGMENT_ID"),
		"event":      event,
		"payload":    payload,
	}
	_, _, err = client.PostJSON(context.Background(), "/v0/agent/hooks", body)
	return err
}
