// Package protocol implements the minimal LSP JSON-RPC surface Crowbar needs:
// id-correlated request/response and publishDiagnostics notifications. It is a thin
// Content-Length codec rather than an external LSP library, as the surface is small.
package protocol

import "encoding/json"

// Request is an outbound JSON-RPC request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Notification is a JSON-RPC notification (no id).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is an inbound JSON-RPC response or server-initiated message.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Reply answers a request the server sent to the client. ID echoes the
// request's id verbatim (JSON-RPC allows numbers or strings); exactly one of
// Result (at least `null`) and Error is set.
type Reply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC error payload.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
