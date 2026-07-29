package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

func newMCPCmd() *cobra.Command {
	var project, repo, workspace, segment, token string
	cmd := &cobra.Command{
		Use:    "mcp",
		Short:  "Relay MCP stdio traffic to the Crowbar daemon",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			client, err := ipc.NewClient("unix://")
			if err != nil {
				return err
			}
			post := func(path string, body any) ([]byte, error) {
				_, raw, err := client.PostJSON(context.Background(), path, body)
				return raw, err
			}
			return runMCPRelay(os.Stdin, os.Stdout, post, segment, project, repo, workspace, token)
		},
	}
	cmd.Flags().StringVar(&segment, "segment", "", "Crowbar segment id")
	cmd.Flags().StringVar(&token, "token", "", "runner token minted at spawn")
	bindScopeFlags(cmd, &project, &repo, &workspace)
	return cmd
}

// runMCPRelay is the whole relay: read one JSON-RPC message per line, hand it
// to the daemon, write back whatever the daemon says to write. It understands
// nothing about MCP itself — which methods exist, what a tool does, whether a
// message deserves a reply are all daemon-side decisions, so a stale crowbar
// binary can never disagree with the daemon it is talking to.
func runMCPRelay(
	in io.Reader, out io.Writer,
	post func(path string, body any) ([]byte, error),
	segment, project, repo, workspace, token string,
) error {
	path := scopedAgentPath(project, repo, workspace, "/runners/"+segment+"/mcp")

	scanner := bufio.NewScanner(in)
	// Tool results and chat logs routinely exceed the 64 KiB default; without
	// raising it a long line truncates into a bogus JSON parse error.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	writer := bufio.NewWriter(out)
	defer func() { _ = writer.Flush() }()

	for scanner.Scan() {
		relayLine(scanner.Text(), path, token, post, writer)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("mcp: read stdin: %w", err)
	}
	return nil
}

// relayLine forwards one JSON-RPC message to the daemon and writes its reply,
// if any. Split out of runMCPRelay's loop so neither function nests past one
// level of control flow.
func relayLine(
	line, path, token string,
	post func(path string, body any) ([]byte, error),
	writer *bufio.Writer,
) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	raw, err := post(path, map[string]any{"token": token, "rpc": json.RawMessage(line)})
	if err != nil {
		// A daemon blip must not end the session: report it against the
		// request's own id and keep serving the next line.
		writeLine(writer, transportError(line, err))
		return
	}

	var envelope struct {
		Data struct {
			RPC json.RawMessage `json:"rpc"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.Data.RPC) == 0 {
		// No rpc in the envelope is the daemon's 204: the message was a
		// notification, and JSON-RPC says the relay stays silent.
		return
	}
	writeLine(writer, envelope.Data.RPC)
}

// writeLine writes payload and flushes immediately. An MCP client blocks on
// stdout waiting for this reply, so a buffered response is a hang.
func writeLine(w *bufio.Writer, payload []byte) {
	_, _ = w.Write(payload)
	_ = w.WriteByte('\n')
	_ = w.Flush()
}

// transportError turns a daemon-reachability failure into a JSON-RPC error
// echoing the request's id, so the client can match the failure to its call.
// A line too malformed to carry an id gets "id":null, which JSON-RPC allows.
func transportError(line string, cause error) []byte {
	var request struct {
		ID json.RawMessage `json:"id"`
	}
	_ = json.Unmarshal([]byte(line), &request)
	id := request.ID
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return []byte(fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%s,"error":{"code":-32603,"message":%q}}`,
		id, "crowbar daemon unreachable: "+cause.Error()))
}
