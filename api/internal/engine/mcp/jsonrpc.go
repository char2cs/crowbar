// Package mcp implements the Model Context Protocol revision 2025-11-25, the one every
// vendor CLI Crowbar drives pins; 2026-07-28 drops the initialize handshake and none of
// them speak it yet. It knows nothing about Crowbar and delegates tools to a ToolSet.
package mcp

import (
	"encoding/json"
	"fmt"
)

// JSON-RPC 2.0 error codes (the -32000..-32099 server range is left alone).
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Request is one inbound JSON-RPC message. ID stays a RawMessage because a response must
// echo a string, number or null id with its original type.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification reports whether the message carries no id. A notification gets
// no reply at all — not even an error — per JSON-RPC 2.0.
func (r Request) IsNotification() bool {
	return len(r.ID) == 0 || string(r.ID) == "null"
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Response struct {
	JSONRPC string `json:"jsonrpc"`
	// No omitempty: JSON-RPC 2.0 requires id present and null when it could not be
	// determined; a nil RawMessage marshals to null.
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

func NewError(id json.RawMessage, code int, message string) Response {
	return Response{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: message}}
}

func NewResult(id json.RawMessage, result any) (Response, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return Response{}, fmt.Errorf("mcp: marshal result: %w", err)
	}
	return Response{JSONRPC: "2.0", ID: id, Result: raw}, nil
}
