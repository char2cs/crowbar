//go:build integration || manual

package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// jsonRPCRequest is a JSON-RPC 2.0 request envelope for MCP tool calls.
type jsonRPCRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response envelope.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toolCallResult mirrors the top-level shape of a tools/call result.
type toolCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// MCPClient calls MCP tools over Streamable HTTP (JSON-RPC) on the Unix socket.
// Use WithToken to pin a specific agent run token.
type MCPClient struct {
	t          *testing.T
	sockPath   string
	token      string
	httpClient *http.Client
}

func newMCPClient(
	sockPath string,
) *MCPClient {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sockPath)
		},
	}
	return &MCPClient{
		sockPath: sockPath,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// WithToken returns a copy of MCPClient with the agent run token pinned. The
// token is sent as the X-Agent-Run-Token header on every subsequent request.
func (c *MCPClient) WithToken(
	token string,
) *MCPClient {
	cp := *c
	cp.token = token
	return &cp
}

// WithTest attaches a testing.T so tool call methods can call t.Fatalf.
func (c *MCPClient) WithTest(
	t *testing.T,
) *MCPClient {
	cp := *c
	cp.t = t
	return &cp
}

func (c *MCPClient) callTool(
	toolName string,
	args map[string]any,
) (string, bool, error) {
	payload := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", false, fmt.Errorf("mcp: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://unix/mcp", bytes.NewReader(b))
	if err != nil {
		return "", false, fmt.Errorf("mcp: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("X-Agent-Run-Token", c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("mcp: do: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("mcp: read body: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return "", false, fmt.Errorf("unauthorized")
	}
	if resp.StatusCode == http.StatusForbidden {
		return "", false, fmt.Errorf("forbidden")
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return "", false, fmt.Errorf("mcp: unexpected status %d: %s", resp.StatusCode, body)
	}

	var rpc jsonRPCResponse
	if err := json.Unmarshal(body, &rpc); err != nil {
		return "", false, fmt.Errorf("mcp: decode rpc: %w", err)
	}
	if rpc.Error != nil {
		return "", false, fmt.Errorf("mcp: rpc error %d: %s", rpc.Error.Code, rpc.Error.Message)
	}

	var result toolCallResult
	if err := json.Unmarshal(rpc.Result, &result); err != nil {
		return "", false, fmt.Errorf("mcp: decode result: %w", err)
	}

	text := ""
	if len(result.Content) > 0 {
		text = result.Content[0].Text
	}
	return text, result.IsError, nil
}

func (c *MCPClient) mustCallTool(
	toolName string,
	args map[string]any,
) string {
	c.t.Helper()
	text, isErr, err := c.callTool(toolName, args)
	if err != nil {
		c.t.Fatalf("MCPClient.%s: %v", toolName, err)
	}
	if isErr {
		c.t.Fatalf("MCPClient.%s: tool returned error: %s", toolName, text)
	}
	return text
}

// Signal calls crowbar_signal with the given event and output.
func (c *MCPClient) Signal(
	event string,
	output string,
) error {
	text, isErr, err := c.callTool("crowbar_signal", map[string]any{
		"event":  event,
		"output": output,
	})
	if err != nil {
		return err
	}
	if isErr {
		return fmt.Errorf("crowbar_signal returned error: %s", text)
	}
	return nil
}

// CreateItem calls crowbar_create_item and returns the new item's ID.
func (c *MCPClient) CreateItem(
	title string,
) (string, error) {
	text, isErr, err := c.callTool("crowbar_create_item", map[string]any{"title": title})
	if err != nil {
		return "", err
	}
	if isErr {
		return "", fmt.Errorf("crowbar_create_item returned error: %s", text)
	}
	var out struct {
		ItemID string `json:"item_id"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return "", fmt.Errorf("crowbar_create_item: decode: %w", err)
	}
	return out.ItemID, nil
}

// UpdateItemStatus calls crowbar_update_item_status.
func (c *MCPClient) UpdateItemStatus(
	itemID string,
	status string,
) error {
	_, isErr, err := c.callTool("crowbar_update_item_status", map[string]any{
		"item_id": itemID,
		"status":  status,
	})
	if err != nil {
		return err
	}
	if isErr {
		return fmt.Errorf("crowbar_update_item_status returned error")
	}
	return nil
}

// GetItems calls crowbar_get_items and returns all kanban items.
func (c *MCPClient) GetItems() ([]domain.KanbanItem, error) {
	text, isErr, err := c.callTool("crowbar_get_items", nil)
	if err != nil {
		return nil, err
	}
	if isErr {
		return nil, fmt.Errorf("crowbar_get_items returned error: %s", text)
	}
	var items []domain.KanbanItem
	if err := json.Unmarshal([]byte(text), &items); err != nil {
		return nil, fmt.Errorf("crowbar_get_items: decode: %w", err)
	}
	return items, nil
}

// OpenThread calls crowbar_open_thread and returns the new thread's ID.
func (c *MCPClient) OpenThread(
	file string,
	line int,
	content string,
) (string, error) {
	text, isErr, err := c.callTool("crowbar_open_thread", map[string]any{
		"file":    file,
		"line":    line,
		"content": content,
	})
	if err != nil {
		return "", err
	}
	if isErr {
		return "", fmt.Errorf("crowbar_open_thread returned error: %s", text)
	}
	var out struct {
		ThreadID string `json:"thread_id"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return "", fmt.Errorf("crowbar_open_thread: decode: %w", err)
	}
	return out.ThreadID, nil
}

// ReplyThread calls crowbar_reply_thread.
func (c *MCPClient) ReplyThread(
	threadID string,
	content string,
) error {
	_, isErr, err := c.callTool("crowbar_reply_thread", map[string]any{
		"thread_id": threadID,
		"content":   content,
	})
	if err != nil {
		return err
	}
	if isErr {
		return fmt.Errorf("crowbar_reply_thread returned error")
	}
	return nil
}

// GetThreads calls crowbar_get_threads and returns matching threads.
func (c *MCPClient) GetThreads(
	status string,
	phase string,
) ([]domain.ReviewThread, error) {
	args := map[string]any{}
	if status != "" {
		args["status"] = status
	}
	if phase != "" {
		args["phase"] = phase
	}
	text, isErr, err := c.callTool("crowbar_get_threads", args)
	if err != nil {
		return nil, err
	}
	if isErr {
		return nil, fmt.Errorf("crowbar_get_threads returned error: %s", text)
	}
	var threads []domain.ReviewThread
	if err := json.Unmarshal([]byte(text), &threads); err != nil {
		return nil, fmt.Errorf("crowbar_get_threads: decode: %w", err)
	}
	return threads, nil
}

// ResolveThread calls crowbar_resolve_thread.
func (c *MCPClient) ResolveThread(
	threadID string,
	emoji string,
) error {
	_, isErr, err := c.callTool("crowbar_resolve_thread", map[string]any{
		"thread_id": threadID,
		"emoji":     emoji,
	})
	if err != nil {
		return err
	}
	if isErr {
		return fmt.Errorf("crowbar_resolve_thread returned error")
	}
	return nil
}
