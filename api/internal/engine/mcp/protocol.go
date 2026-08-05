package mcp

import (
	"context"
	"encoding/json"
)

// ProtocolVersion is the revision this server speaks. See the package doc for
// why it is not 2026-07-28.
const ProtocolVersion = "2025-11-25"

// Tool is one advertised tool. InputSchema is a raw JSON Schema object.
//
// There is deliberately no OutputSchema field: under 2025-11-25 a tool that
// declares one must return structuredContent AND should also serialize the same
// data into content, which sends the whole payload twice. Results here are text.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolSet is everything the protocol layer needs to know about Crowbar: nothing,
// beyond a list of tools and a way to call one. Call returns the text a model
// should read; an error is rendered as a TOOL error, not a transport error.
type ToolSet interface {
	Tools() []Tool
	Call(ctx context.Context, name string, args json.RawMessage) (string, error)
}

type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      serverInfo     `json:"serverInfo"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type toolsListResult struct {
	Tools []Tool `json:"tools"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type callToolResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError"`
}

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
