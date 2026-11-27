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

const mcpRelayTimeout = 120 * time.Second

func newMCPCmd() *cobra.Command {
	var project, repo, workspace, segment, relay, tokenFile, home, host string
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
			path := mcpPath(relay, segment, project, repo, workspace)
			return runMCPRelay(cmd.InOrStdin(), cmd.OutOrStdout(), post, path, func() (string, error) {
				return readRunnerToken(tokenFile)
			})
		},
	}
	cmd.Flags().StringVar(&segment, "segment", "", "Crowbar segment id")
	cmd.Flags().StringVar(&relay, "relay", "", "relay id of the session this relay serves, in place of --segment and the scope flags")
	cmd.Flags().StringVar(&tokenFile, "token-file", "", "file holding the runner token minted at spawn")
	bindScopeFlags(cmd, &project, &repo, &workspace, &home, &host)
	return cmd
}

func mcpPath(relay, segment, project, repo, workspace string) string {
	if relay != "" {
		return "/v0/relays/" + relay + "/mcp"
	}
	return scopedAgentPath(project, repo, workspace, "/runners/"+segment+"/mcp")
}

func readRunnerToken(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	raw, err := os.ReadFile(path) //nolint:gosec // G304: the spawn names its own runner's token file
	if err != nil {
		return "", fmt.Errorf("mcp: read the runner token: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

func runMCPRelay(
	in io.Reader, out io.Writer,
	post func(path string, body any) (int, []byte, error),
	path string,
	token func() (string, error),
) error {
	scanner := bufio.NewScanner(in)
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

func relayLine(
	line, path string,
	token func() (string, error),
	post func(path string, body any) (int, []byte, error),
	writer *bufio.Writer,
) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	tok, err := token()
	if err != nil {
		writeTransportError(writer, line, err)
		return
	}
	status, raw, err := post(path, map[string]any{"token": tok, "rpc": json.RawMessage(line)})
	if err != nil {
		writeTransportError(writer, line, err)
		return
	}
	if status < 200 || status > 299 {
		writeTransportError(writer, line, fmt.Errorf("daemon returned HTTP %d: %s", status, raw))
		return
	}

	var envelope struct {
		Data struct {
			RPC json.RawMessage `json:"rpc"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.Data.RPC) == 0 {
		return
	}
	writeLine(writer, envelope.Data.RPC)
}

var stdoutBroken sync.Once

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

func writeTransportError(w *bufio.Writer, line string, cause error) {
	if payload := transportError(line, cause); payload != nil {
		writeLine(w, payload)
	}
}

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
