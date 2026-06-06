// Package protocol implements the minimal LSP JSON-RPC surface Crowbar needs:
// id-correlated request/response and publishDiagnostics notifications (10 §1).
//
// Decision (Task 1): self-implemented — no external JSON-RPC/LSP library.
// grep of go.mod/go.sum found no existing jsonrpc2/go.lsp.dev dependency.
// The protocol surface required (request/response by id + publishDiagnostics
// notifications) is small enough that a thin Content-Length framing codec plus
// these structs covers it entirely; adding a heavy dependency is unjustified.
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

// RPCError is a JSON-RPC error payload.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
