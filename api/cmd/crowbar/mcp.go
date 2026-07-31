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

// mcpRelayTimeout bounds ONE relayed MCP message.
//
// It is not the ipc default (5s), and the difference is the point. A hook or a
// handoff callback is a small read against local state; a TOOL CALL can be
// several git subprocesses — get_review_scope resolves the merge base and reads
// the whole branch diff, post_review_comment reads the diff geometry before it
// writes — and the daemon bounds each of those at its own GitOpTimeout of 60s,
// chosen as "far above any healthy local git command". Under the 5s default that
// ceiling was unreachable from this path: no tool call could spend more than five
// seconds of git, however large the repository.
//
// Two seconds' work is not what makes this worth fixing. The failure was that
// the relay pre-empted work the daemon was still legitimately doing, and then
// reported it as `crowbar daemon unreachable` — which is false, points the model
// at a recovery that cannot help, and (for an unkeyed write that had already
// committed) invites a retry that duplicates the finding.
//
// 120s is two of the daemon's own git ceilings: enough for a call that chains a
// ref resolution and a diff read to hit that ceiling and return the daemon's own
// error, and short enough that a genuinely wedged daemon still surfaces to the
// model inside one turn rather than hanging the CLI.
const mcpRelayTimeout = 120 * time.Second

func newMCPCmd() *cobra.Command {
	var project, repo, workspace, segment, token string
	cmd := &cobra.Command{
		Use:    "mcp",
		Short:  "Relay MCP stdio traffic to the Crowbar daemon",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			client, err := ipc.NewClientWithTimeout("unix://", mcpRelayTimeout)
			if err != nil {
				return err
			}
			post := func(path string, body any) (int, []byte, error) {
				return client.PostJSON(context.Background(), path, body)
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
		// PostJSON returns err == nil for a non-2xx: a bad request, a tool
		// surface not configured, or a stale binary hitting a route that
		// 404s. That body carries no rpc field either, which would otherwise
		// be indistinguishable from the 204 silence below — answering an
		// application error with silence hangs the client forever on a
		// request it is entitled to a reply for, so it becomes an error too.
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

// writeLine writes payload and flushes immediately. An MCP client blocks on
// stdout waiting for this reply, so a buffered response is a hang. A failure
// here (a broken pipe) is reported to stderr, never stdout — stdout is the
// protocol channel a client is parsing as JSON-RPC.
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

// transportError turns a daemon-reachability failure into a JSON-RPC error
// echoing the request's id, so the client can match the failure to its call.
//
// It returns nil — no reply at all — for a NOTIFICATION: a line that parsed
// cleanly and carried no id. JSON-RPC 2.0 forbids any response to a
// notification, and permits "id":null only for a request whose id could not be
// DETERMINED, which is the different case of a line too malformed to parse.
// Telling those apart is the whole reason the unmarshal error is inspected
// rather than discarded.
//
// The distinction is not academic: notifications/initialized is a notification
// sent during the handshake, which is the likeliest moment for a transient
// daemon failure, and answering it produced an unsolicited {"id":null,"error":…}
// on stdout that the client never asked for.
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
