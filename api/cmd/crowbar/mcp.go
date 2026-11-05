package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

// mcpRelayTimeout bounds one relayed MCP message. A tool call can chain git
// subprocesses the daemon bounds at 60s each, so this is two of those ceilings:
// the daemon's own error surfaces first, yet a wedged daemon still fails in a turn.
const mcpRelayTimeout = 120 * time.Second

func newMCPCmd() *cobra.Command {
	var project, repo, workspace, segment, token, home, host string
	cmd := &cobra.Command{
		Use:    "mcp",
		Short:  "Relay MCP stdio traffic to the Crowbar daemon",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := applyHomeOverride(home); err != nil {
				return err
			}
			client, err := ipc.NewClientWithTimeout(host, mcpRelayTimeout)
			if err != nil {
				return err
			}
			post := func(path string, body any) (int, []byte, error) {
				return client.PostJSON(context.Background(), path, body)
			}
			return runMCPRelay(cmd.InOrStdin(), cmd.OutOrStdout(), post, segment, project, repo, workspace, token)
		},
	}
	cmd.Flags().StringVar(&segment, "segment", "", "Crowbar segment id")
	cmd.Flags().StringVar(&token, "token", "", "runner token minted at spawn")
	bindScopeFlags(cmd, &project, &repo, &workspace, &home, &host)
	return cmd
}

// runMCPRelay reads one JSON-RPC message per line, hands it to the daemon and
// writes back whatever the daemon says. All MCP semantics live daemon-side, so a
// stale crowbar binary can never disagree with its daemon.
func runMCPRelay(
	in io.Reader, out io.Writer,
	post func(path string, body any) (int, []byte, error),
	segment, project, repo, workspace, token string,
) error {
	path := scopedAgentPath(project, repo, workspace, "/runners/"+segment+"/mcp")

	scanner := bufio.NewScanner(in)
	// Tool results and chat logs routinely exceed the 64 KiB default; without
	// raising it a long line truncates into a bogus JSON parse error.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	writer := bufio.NewWriter(out)
	defer func() {
		if err := writer.Flush(); err != nil {
			reportBrokenStdout(err)
		}
	}()

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
	post func(path string, body any) (int, []byte, error),
	writer *bufio.Writer,
) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	status, raw, err := post(path, map[string]any{"token": token, "rpc": json.RawMessage(line)})
	if err != nil {
		// A daemon blip must not end the session: report it against the
		// request's own id and keep serving the next line.
		writeTransportError(writer, line, err)
		return
	}
	if status < 200 || status > 299 {
		// A non-2xx body carries no rpc field, indistinguishable from the 204 silence
		// below; a request is owed a reply, so it becomes an error instead of a hang.
		writeTransportError(writer, line, fmt.Errorf("daemon returned HTTP %d: %s", status, raw))
		return
	}

	var envelope struct {
		Data struct {
			RPC json.RawMessage `json:"rpc"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.Data.RPC) == 0 {
		// A 2xx with no rpc is the daemon's 204: the message was a
		// notification, and JSON-RPC says the relay stays silent.
		return
	}
	writeLine(writer, envelope.Data.RPC)
}

// stdoutBroken reports a broken stdout pipe at most once per process: every
// write after the pipe dies would fail the same way, and the relay has
// nothing better to do with the error than say so exactly once.
var stdoutBroken sync.Once

// writeLine writes payload and flushes immediately: the MCP client blocks on this
// reply. Failures go to stderr, since stdout is the JSON-RPC channel.
func writeLine(w *bufio.Writer, payload []byte) {
	if _, err := w.Write(payload); err != nil {
		reportBrokenStdout(err)
		return
	}
	if err := w.WriteByte('\n'); err != nil {
		reportBrokenStdout(err)
		return
	}
	if err := w.Flush(); err != nil {
		reportBrokenStdout(err)
	}
}

func reportBrokenStdout(err error) {
	stdoutBroken.Do(func() {
		fmt.Fprintf(os.Stderr, "crowbar mcp: write reply: %v\n", err)
	})
}

// writeTransportError reports cause against line's id, and writes NOTHING when
// line is a notification. Split out so both failure sites in relayLine share one
// answer to "does this message deserve a reply at all" — see transportError.
func writeTransportError(w *bufio.Writer, line string, cause error) {
	if payload := transportError(line, cause); payload != nil {
		writeLine(w, payload)
	}
}

// transportError turns a daemon-reachability failure into a JSON-RPC error echoing
// the request's id. It returns nil for a notification (parsed, no id): JSON-RPC
// forbids replying to one, and "id":null is only for an unparseable line.
func transportError(line string, cause error) []byte {
	var request struct {
		ID json.RawMessage `json:"id"`
	}
	id := json.RawMessage("null")
	if err := json.Unmarshal([]byte(line), &request); err == nil {
		if len(request.ID) == 0 {
			return nil
		}
		id = request.ID
	}
	return []byte(fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%s,"error":{"code":-32603,"message":%q}}`,
		id, "crowbar daemon unreachable: "+cause.Error()))
}
