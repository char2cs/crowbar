package mcp

import (
	"context"
	"encoding/json"
)

const ProtocolVersion = "2025-11-25"

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type ToolSet interface {
	Tools() []Tool
	Call(ctx context.Context, name string, args json.RawMessage) (string, error)
}

type Image struct {
	MIMEType string
	Data     string
}

type ImageToolSet interface {
	ToolSet
	CallWithImages(ctx context.Context, name string, args json.RawMessage) (string, []Image, error)
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

type imageContent struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MIMEType string `json:"mimeType"`
}

type callToolResult struct {
	Content []any `json:"content"`
	IsError bool  `json:"isError"`
}

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
