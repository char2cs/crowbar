// Package mcp implements the Model Context Protocol revision 2025-11-25 —
// the revision both vendor CLIs Crowbar drives actually speak (claude 2.1.220
// and codex 0.139.0 both pin it). The 2026-07-28 revision is deliberately NOT
// implemented here: it removes the initialize handshake, replaces
// server-initiated requests with Multi Round-Trip Requests and requires
// server/discover, none of which any client Crowbar targets can use yet.
//
// This package knows nothing about Crowbar. It speaks protocol and delegates
// every tool decision to a ToolSet.
package mcp

import "encoding/json"

// JSON-RPC 2.0 error codes (the -32000..-32099 server range is left alone).
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Request is one inbound JSON-RPC message. ID is kept as RawMessage because
// JSON-RPC permits a string, a number or null, and a response MUST echo the id
// back with the same type it arrived as — decoding into any Go scalar would
// silently rewrite it.
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
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

func NewError(id json.RawMessage, code int, message string) Response {
	return Response{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: message}}
}

func NewResult(id json.RawMessage, result any) (Response, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return Response{}, err
	}
	return Response{JSONRPC: "2.0", ID: id, Result: raw}, nil
}
