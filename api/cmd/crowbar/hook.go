package main

import (
	"context"
	"encoding/json"
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
				err = runHook(hookRun{
					Event: args[0], Segment: segment, Provider: provider,
					Project: project, Repo: repo, Workspace: workspace,
					Payload: payload, Host: "unix://", Out: os.Stdout,
				})
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
		defer func() { _ = f.Close() }()
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

// hookRun is one invocation of the relay. It is a struct rather than eight
// positional parameters because the last two exist for the tests — the daemon
// host, and where a decision is printed — and a caller that must count arguments
// to find them will eventually mis-count.
type hookRun struct {
	Event     string
	Segment   string
	Provider  string
	Project   string
	Repo      string
	Workspace string
	Payload   []byte
	Host      string
	Out       io.Writer
}

// runHook forwards a raw hook payload verbatim to the daemon, which holds the
// descriptor and parses it per the provider's declared format. The target URL
// is scoped to project/repo/workspace (Task 3 nested the agent routes under
// the workspace group; the vendor CLI's hook command now passes these ids as
// explicit flags so the callback can rebuild the nested path).
//
// It then does the one thing that makes a prompt answerable from Crowbar's chat:
// if the daemon says this hook opened a question a human can decide HERE, the
// process STAYS ALIVE. A vendor CLI holds its permission gate open for exactly as
// long as its hook runs, so exiting is what lets its own dialog through.
//
// A DAEMON THAT CANNOT BE REACHED NEVER BLOCKS. The delivery error returns
// immediately, nothing is printed, and the exit is 0 — so the CLI's dialog
// reaches the human in milliseconds. That is strictly better than waiting out a
// budget on a daemon that will never answer, and its worst case is exactly the
// behaviour of a machine with no Crowbar on it.
func runHook(run hookRun) error {
	envelope := hookEnvelope{
		DeliveryID: uuid.NewString(),
		SegmentID:  run.Segment,
		Provider:   run.Provider,
		Event:      run.Event,
		PayloadRaw: string(run.Payload),
		Project:    run.Project,
		Repo:       run.Repo,
		Workspace:  run.Workspace,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	if _, err := persistHookEnvelope(envelope); err != nil {
		return err
	}
	// A failed or non-2xx delivery leaves the fsynced envelope in the spool.
	// The daemon's loop and every later hook retry the same delivery id in FIFO
	// order; nothing is discarded merely because this short-lived callback exits.
	ack, err := drainHookSpoolFor(context.Background(), run.Host, envelope.DeliveryID)
	if err != nil {
		return err
	}
	wait, waiting := awaitDirective(ack)
	if !waiting {
		return nil
	}
	return awaitHookAnswer(envelope, run.Host, wait, run.Out)
}

// awaitDirective reads the stay-alive instruction off the daemon's
// acknowledgement, if there is one.
//
// A body that cannot be decoded is NOT an instruction to wait. An older daemon,
// a proxy, an empty 202 — every one of them lands on "exit now", which is the
// behaviour every hook had before this channel existed.
func awaitDirective(body []byte) (waitMS int64, waiting bool) {
	if len(body) == 0 {
		return 0, false
	}
	var ack hookAck
	if err := json.Unmarshal(body, &ack); err != nil {
		return 0, false
	}
	if ack.Data.Await == nil || ack.Data.Await.WaitMS <= 0 {
		return 0, false
	}
	return ack.Data.Await.WaitMS, true
}
