package main

import (
	"bytes"
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

const (
	answerSlack = 15 * time.Second

	abandonBudget = 2 * time.Second
)

type hookAck struct {
	Data struct {
		Await *struct {
			ChoiceID string `json:"choiceId"`
			WaitMS   int64  `json:"waitMs"`
		} `json:"await"`
	} `json:"data"`
}

type hookAnswer struct {
	Data struct {
		Stdout string `json:"stdout"`
	} `json:"data"`
}

func awaitHookAnswer(
	envelope hookEnvelope,
	host string,
	waitMS int64,
	out io.Writer,
) error {

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(signals)
	return awaitHookAnswerOn(envelope, host, waitMS, out, signals)
}

func awaitHookAnswerOn(
	envelope hookEnvelope,
	host string,
	waitMS int64,
	out io.Writer,
	signals <-chan os.Signal,
) error {
	wait := time.Duration(waitMS) * time.Millisecond
	if wait <= 0 {

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

		return "", err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "", fmt.Errorf("hook answer: daemon returned HTTP %d", status)
	}

	if len(bytes.TrimSpace(body)) == 0 {
		return "", nil
	}
	var answer hookAnswer
	if err := json.Unmarshal(body, &answer); err != nil {

		return "", fmt.Errorf("hook answer: decode: %w", err)
	}
	return answer.Data.Stdout, nil
}

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
