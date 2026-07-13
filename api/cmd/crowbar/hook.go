package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

func newHookCmd() *cobra.Command {
	var segment, provider, payloadFile, payloadInline, project, repo, workspace string
	cmd := &cobra.Command{
		Use:    "hook <event>",
		Short:  "Forward a vendor-CLI hook payload to the Crowbar daemon",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(_ *cobra.Command, args []string) error {
			// A hook must never break the vendor CLI: swallow every error into
			// an exit-0 RunE, surfaced on stderr only (never stdout).
			payload, err := resolvePayload(payloadInline, payloadFile, os.Stdin)
			if err == nil {
				err = runHook(args[0], segment, provider, project, repo, workspace, payload, "unix://")
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "crowbar hook %s: %v\n", args[0], err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&segment, "segment", "", "Crowbar segment id")
	cmd.Flags().StringVar(&provider, "provider", "", "provider id")
	cmd.Flags().StringVar(&payloadFile, "payload-file", "", "read the payload from this file instead of stdin")
	cmd.Flags().StringVar(&payloadInline, "payload", "", "inline payload instead of stdin")
	bindScopeFlags(cmd, &project, &repo, &workspace)
	return cmd
}

// resolvePayload selects the payload source: inline > file > stdin.
func resolvePayload(inline, file string, stdin io.Reader) ([]byte, error) {
	switch {
	case inline != "":
		return []byte(inline), nil
	case file != "":
		return os.ReadFile(file) //nolint:gosec // path is authored in the descriptor's own hook command
	default:
		return io.ReadAll(io.LimitReader(stdin, 8<<20))
	}
}

// runHook forwards a raw hook payload verbatim to the daemon, which holds the
// descriptor and parses it per the provider's declared format. The target URL
// is scoped to project/repo/workspace (Task 3 nested the agent routes under
// the workspace group; the vendor CLI's hook command now passes these ids as
// explicit flags so the callback can rebuild the nested path).
func runHook(
	event, segment, provider, project, repo, workspace string,
	payload []byte, host string,
) error {
	client, err := ipc.NewClient(host)
	if err != nil {
		return err
	}
	body := map[string]any{
		"segment_id":  segment,
		"provider":    provider,
		"event":       event,
		"payload_raw": string(payload),
	}
	_, _, err = client.PostJSON(context.Background(), scopedAgentPath(project, repo, workspace, "/hooks"), body)
	return err
}
