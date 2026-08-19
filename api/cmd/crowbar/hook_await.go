package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

// The collector: the relay's half of the rewake delivery channel.
//
// A vendor CLI can register a hook that runs in the BACKGROUND and, when it exits
// with a particular status, wakes the model and feeds it whatever that hook
// printed. This command is that hook. It blocks against the daemon until the chat
// its runner is on has a message, prints the message, and exits with the wake
// status — which is how a prompt reaches a session that is already running,
// instead of a fresh CLI being spawned to carry it in its argv.
//
// EVERY EXIT PATH PRINTS EITHER ONE COMPLETE MESSAGE OR NOTHING AT ALL, and
// nothing is always safe. A collector that prints nothing and exits 0 simply
// leaves the session as it was; the daemon, which never handed a message to a
// collector that did not take it, delivers by restarting the CLI instead. That
// fallback is the reason none of the failures below need to be clever.
const (
	// awaitPollBudget is how long ONE request asks the daemon to hold for. It is
	// not the collector's lifetime: this command loops, so a poll ending empty is
	// answered by asking again, and the process stays armed until its provider
	// kills it or the daemon says its runner is gone.
	//
	// It is short enough for the daemon to re-read which chat this runner is on —
	// a CLI moves between conversations on its own — and long enough that the loop
	// costs one local request a minute.
	awaitPollBudget = 60 * time.Second

	// awaitSlack is added when bounding the request's own transport. The daemon
	// returns at its own deadline; this only stops a wedged socket from outliving
	// it.
	awaitSlack = 15 * time.Second
)

func newAwaitPromptCmd() *cobra.Command {
	var segment, provider, token, project, repo, workspace string
	var wakeStatus int
	cmd := &cobra.Command{
		Use:    "await-prompt",
		Short:  "Block until the Crowbar daemon has a chat message for this runner",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			// Errors are swallowed exactly as the sibling hook command swallows them:
			// a callback must never break the CLI that fired it. Reported on stderr,
			// never on stdout — stdout is the message channel, and a diagnostic
			// printed there would be read as something the user said.
			prompt, err := collectPrompt(awaitRun{
				Segment: segment, Token: token,
				Project: project, Repo: repo, Workspace: workspace,
				Host: "unix://", Budget: awaitPollBudget,
			}, awaitSignals())
			if err != nil {
				fmt.Fprintf(os.Stderr, "crowbar hook await-prompt: %v\n", err)
				return nil
			}
			if prompt == "" {
				return nil
			}
			// The wake status is the PROVIDER'S protocol and arrives from its own
			// descriptor; this command knows only that it has to leave with it. A
			// status of zero means the provider has no such mechanism, and then
			// nothing is printed at all: text on the stdout of a hook nobody acts on
			// is a message thrown away, which is worse than one never delivered —
			// the daemon has already recorded the handover.
			if wakeStatus <= 0 {
				fmt.Fprintln(os.Stderr,
					"crowbar hook await-prompt: no wake status declared; refusing to print a message that cannot be delivered")
				return nil
			}
			// One write, one buffer, and the exit AFTER it. A provider reading a
			// truncated message would deliver a truncated message; there is no path
			// here that can produce one.
			if _, err := os.Stdout.WriteString(prompt); err != nil {
				fmt.Fprintf(os.Stderr, "crowbar hook await-prompt: write: %v\n", err)
				return nil
			}
			os.Exit(wakeStatus)
			return nil
		},
	}
	cmd.Flags().StringVar(&segment, "segment", "", "Crowbar segment id")
	cmd.Flags().StringVar(&provider, "provider", "", "provider id")
	cmd.Flags().StringVar(&token, "token", "", "runner token minted at spawn")
	cmd.Flags().IntVar(&wakeStatus, "wake-status", 0,
		"exit status this provider takes a collected message on (descriptor-declared)")
	bindScopeFlags(cmd, &project, &repo, &workspace)
	return cmd
}

// awaitRun is one collector's configuration.
type awaitRun struct {
	Segment   string
	Token     string
	Project   string
	Repo      string
	Workspace string
	Host      string
	Budget    time.Duration
}

// awaitSignals traps the ways a collector is told to stop.
//
// SIGTERM is the important one and it is not an error case: it is how a provider
// ENDS a background hook that has outlived its declared timeout. Measured against
// claude 2.1.235 on 2026-08-18, that is exactly what arrives, and answering it by
// exiting silently is what keeps a routine end-of-life from looking like a
// delivery.
func awaitSignals() <-chan os.Signal {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	return signals
}

// collectPrompt polls the daemon until it has a message, and returns "" for every
// way of having none.
//
// It LOOPS on an empty poll rather than exiting, and that is what keeps a session
// reachable between turns: the provider arms this hook once, when a turn ends, and
// a collector that exited on its first empty answer would leave the chat with no
// channel until the next turn — which cannot happen until a message is delivered.
func collectPrompt(run awaitRun, signals <-chan os.Signal) (string, error) {
	client, err := ipc.NewClientWithTimeout(run.Host, run.Budget+awaitSlack)
	if err != nil {
		return "", err
	}
	path := scopedAgentPath(run.Project, run.Repo, run.Workspace, "/runners/"+run.Segment+"/prompt-await")

	for {
		ctx, cancel := context.WithCancel(context.Background())
		type result struct {
			prompt string
			retry  bool
			err    error
		}
		done := make(chan result, 1)
		go func() {
			prompt, retry, pollErr := pollForPrompt(ctx, client, path, run.Token, run.Budget)
			done <- result{prompt: prompt, retry: retry, err: pollErr}
		}()

		select {
		case <-signals:
			// Being stopped. Release the poll so the daemon frees this collector's
			// slot immediately — a slot left registered is a slot a message could be
			// handed to, and nobody is here to carry it.
			cancel()
			return "", nil
		case r := <-done:
			cancel()
			if r.err != nil || !r.retry {
				return r.prompt, r.err
			}
		}
	}
}

// pollForPrompt makes ONE request. retry reports whether asking again is the right
// answer to what came back.
//
// Only an empty 2xx is worth asking again for. A transport failure means no daemon
// — the machine looks like one Crowbar was never installed on, and this process
// has nothing to wait for. A non-2xx means the daemon has answered and the answer
// is no: a rejected credential, or a runner it no longer knows. Retrying either
// would be a detached process polling a socket forever, which is precisely the
// orphan this must not become.
func pollForPrompt(
	ctx context.Context,
	client *ipc.Client,
	path, token string,
	budget time.Duration,
) (prompt string, retry bool, err error) {
	status, body, err := client.PostJSON(ctx, path, map[string]any{
		"token":  token,
		"waitMs": budget.Milliseconds(),
	})
	if err != nil {
		if ctx.Err() != nil {
			return "", false, nil
		}
		return "", false, err
	}
	if status == http.StatusNoContent {
		return "", true, nil
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "", false, fmt.Errorf("daemon returned HTTP %d", status)
	}
	var answer struct {
		Data struct {
			Prompt string `json:"prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		// A body that cannot be read is not a message. Never print a guess into a
		// conversation.
		return "", false, fmt.Errorf("decode: %w", err)
	}
	// A 2xx carrying no prompt is the same fact as a 204 — a daemon that returned
	// without writing, which gin sends as an empty 200.
	return answer.Data.Prompt, answer.Data.Prompt == "", nil
}
