package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

// The relay's half of the answer channel.
//
// A vendor CLI holds its own permission gate open for exactly as long as the hook
// process it fired keeps running, and reads that process's stdout as the
// decision. Measured against claude 2.1.234 on 2026-08-18: a hook that held the
// dialog for 45.5 seconds and then printed an allow got the tool RUN, with
// PostToolUse and Stop firing afterwards and no human touching the terminal. So
// answering a prompt from Crowbar's chat is not a new transport — it is this
// process staying alive.
//
// Every exit path here prints either ONE complete document or NOTHING, and exits
// 0. Nothing is the safe answer, always: a hook that prints nothing and exits 0
// falls through to the CLI's own dialog in milliseconds (measured), which is
// precisely the behaviour of a machine with no Crowbar installed.
const (
	// answerSlack is added to the daemon's declared budget when bounding the
	// long-poll's own transport. The daemon returns at its deadline under its own
	// control; this only stops a wedged socket from outliving it.
	answerSlack = 15 * time.Second

	// abandonBudget bounds the "somebody decided at the terminal" report.
	//
	// It is deliberately tiny. That report is made from a SIGNAL HANDLER, on a
	// process the provider has already decided to kill, and a hook that hangs while
	// being killed is the one thing this path may never do. Losing the report costs
	// a stale prompt in the chat until the turn ends; hanging costs the CLI.
	abandonBudget = 2 * time.Second
)

// hookAck is the daemon's reply to a delivered hook. Everything but the await
// directive is ignored: the relay carries bytes and decides nothing.
type hookAck struct {
	Data struct {
		Await *struct {
			ChoiceID string `json:"choiceId"`
			WaitMS   int64  `json:"waitMs"`
		} `json:"await"`
	} `json:"data"`
}

// hookAnswer is the verdict, rendered daemon-side from the provider's descriptor.
type hookAnswer struct {
	Data struct {
		Stdout string `json:"stdout"`
	} `json:"data"`
}

// awaitHookAnswer holds this process alive while a human decides in Crowbar, then
// prints whatever the daemon rendered.
//
// stdout is written exactly once, from one complete buffer. A provider reading a
// truncated document off a hook logs a parse failure and falls back to its dialog
// — so a partial write is strictly worse than no write, and there is no path here
// that can produce one.
func awaitHookAnswer(
	envelope hookEnvelope,
	host string,
	waitMS int64,
	out io.Writer,
) error {
	// SIGTERM is trapped because it is the ONLY notice Crowbar gets that a human
	// answered at the PTY instead. Measured against claude 2.1.234: saying YES
	// there fires PostToolUse (which resolves the prompt on its own), but saying NO
	// fires NOTHING AT ALL — not PostToolUse, not PostToolUseFailure, not
	// PermissionDenied, not Stop. The blocked hook is simply killed. Without this
	// trap a declined prompt would sit in the chat, unanswerable, until the turn
	// ended.
	//
	// It is also the signal the provider sends when its own hook timeout expires,
	// so trapping it is what keeps that case from looking like a hang either.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(signals)
	return awaitHookAnswerOn(envelope, host, waitMS, out, signals)
}

// awaitHookAnswerOn is awaitHookAnswer with the signal source supplied, so a test
// can drive the killed-relay path without signalling its own process.
func awaitHookAnswerOn(
	envelope hookEnvelope,
	host string,
	waitMS int64,
	out io.Writer,
	signals <-chan os.Signal,
) error {
	wait := time.Duration(waitMS) * time.Millisecond
	if wait <= 0 {
		// The daemon asked us to wait for no time at all, which is a daemon that has
		// changed its mind. Falling through is the honest reading.
		return nil
	}
	client, err := ipc.NewClientWithTimeout(host, wait+answerSlack)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type result struct {
		stdout string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		stdout, pollErr := pollHookAnswer(ctx, client, envelope)
		done <- result{stdout: stdout, err: pollErr}
	}()

	select {
	case <-signals:
		// Release the poll first so the daemon frees this prompt's slot, then report.
		// Both are best-effort and both are bounded: we are being killed.
		cancel()
		reportAbandoned(envelope, host)
		return nil
	case r := <-done:
		if r.err != nil {
			return r.err
		}
		if r.stdout == "" {
			return nil
		}
		_, writeErr := io.WriteString(out, r.stdout+"\n")
		return writeErr
	}
}

// pollHookAnswer makes the single long-poll. One request, not a loop: the daemon
// returns the moment a human decides, and polling would put a round trip of
// latency on a gate somebody is watching.
func pollHookAnswer(
	ctx context.Context,
	client *ipc.Client,
	envelope hookEnvelope,
) (string, error) {
	status, body, err := client.PostJSON(
		ctx,
		scopedAgentPath(envelope.Project, envelope.Repo, envelope.Workspace, "/hooks/await"),
		map[string]string{"delivery_id": envelope.DeliveryID},
	)
	if err != nil {
		// The daemon went away while we waited. Print nothing: the CLI's own dialog
		// is what the human gets, which is the pre-Crowbar behaviour.
		return "", err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "", fmt.Errorf("hook answer: daemon returned HTTP %d", status)
	}
	var answer hookAnswer
	if err := json.Unmarshal(body, &answer); err != nil {
		// A body we cannot read is not a decision. Never print a guess.
		return "", fmt.Errorf("hook answer: decode: %w", err)
	}
	return answer.Data.Stdout, nil
}

// reportAbandoned tells the daemon this prompt was decided somewhere else, so the
// chat stops showing a question nobody is asking any more.
//
// Its own context is used rather than the cancelled poll's: this call exists
// precisely because the poll was abandoned. Errors are swallowed — there is
// nowhere to report them to, and the process is about to end.
func reportAbandoned(envelope hookEnvelope, host string) {
	client, err := ipc.NewClientWithTimeout(host, abandonBudget)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), abandonBudget)
	defer cancel()
	_, _, _ = client.PostJSON(
		ctx,
		scopedAgentPath(envelope.Project, envelope.Repo, envelope.Workspace, "/hooks/abandon"),
		map[string]string{"delivery_id": envelope.DeliveryID},
	)
}
