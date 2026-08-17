package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

const maxHookPayloadBytes = 64 << 20

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
		if len(inline) > maxHookPayloadBytes {
			return nil, fmt.Errorf("hook payload exceeds %d bytes", maxHookPayloadBytes)
		}
		return []byte(inline), nil
	case file != "":
		f, err := os.Open(file) //nolint:gosec // path is authored in the descriptor's own hook command
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return readBoundedHookPayload(f)
	default:
		return readBoundedHookPayload(stdin)
	}
}

func readBoundedHookPayload(r io.Reader) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(r, maxHookPayloadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > maxHookPayloadBytes {
		return nil, fmt.Errorf("hook payload exceeds %d bytes", maxHookPayloadBytes)
	}
	return payload, nil
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
	envelope := hookEnvelope{
		DeliveryID: uuid.NewString(),
		SegmentID:  segment,
		Provider:   provider,
		Event:      event,
		PayloadRaw: string(payload),
		Project:    project,
		Repo:       repo,
		Workspace:  workspace,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	if _, err := persistHookEnvelope(envelope); err != nil {
		return err
	}
	// A failed or non-2xx delivery leaves the fsynced envelope in the spool.
	// The daemon's loop and every later hook retry the same delivery id in FIFO
	// order; nothing is discarded merely because this short-lived callback exits.
	return drainHookSpool(context.Background(), host)
}
