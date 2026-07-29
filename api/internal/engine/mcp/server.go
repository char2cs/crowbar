package mcp

import (
	"context"
	"encoding/json"
)

// Server dispatches a single JSON-RPC message against a ToolSet. It holds no
// per-connection state: every message is self-contained, which is what lets the
// transport be one HTTP POST per message (see cmd/crowbar's mcp relay).
type Server struct {
	name    string
	version string
	tools   ToolSet
}

func NewServer(name, version string, tools ToolSet) *Server {
	return &Server{name: name, version: version, tools: tools}
}

// Handle processes one inbound message. The bool reports whether anything should
// be sent back: a JSON-RPC notification is answered with silence, never with an
// error, so a client that sends notifications/initialized is not confused by a
// stray reply.
func (s *Server) Handle(ctx context.Context, raw []byte) ([]byte, bool) {
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return marshal(NewError(nil, CodeParseError, "invalid JSON")), true
	}
	if req.IsNotification() {
		return nil, false
	}

	switch req.Method {
	case "initialize":
		return marshal(s.result(req.ID, initializeResult{
			ProtocolVersion: ProtocolVersion,
			// An empty object, not nil: the tools capability must be PRESENT for a
			// client to call tools/list, and `"tools":null` does not count.
			Capabilities: map[string]any{"tools": map[string]any{}},
			ServerInfo:   serverInfo{Name: s.name, Version: s.version},
		})), true

	case "ping":
		return marshal(s.result(req.ID, struct{}{})), true

	case "tools/list":
		return marshal(s.result(req.ID, toolsListResult{Tools: s.tools.Tools()})), true

	case "tools/call":
		var p callToolParams
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			return marshal(NewError(req.ID, CodeInvalidParams, "params must carry a tool name")), true
		}
		text, err := s.tools.Call(ctx, p.Name, p.Arguments)
		if err != nil {
			// A tool failure is data the model should read and act on, so it rides
			// back as a successful result carrying isError — not as a JSON-RPC
			// error, which clients treat as a transport fault.
			return marshal(s.result(req.ID, callToolResult{
				Content: []textContent{{Type: "text", Text: err.Error()}},
				IsError: true,
			})), true
		}
		return marshal(s.result(req.ID, callToolResult{
			Content: []textContent{{Type: "text", Text: text}},
		})), true

	default:
		return marshal(NewError(req.ID, CodeMethodNotFound, "unsupported method "+req.Method)), true
	}
}

func (s *Server) result(id json.RawMessage, v any) Response {
	resp, err := NewResult(id, v)
	if err != nil {
		return NewError(id, CodeInternalError, "encode result: "+err.Error())
	}
	return resp
}

func marshal(resp Response) []byte {
	b, err := json.Marshal(resp)
	if err != nil {
		// The only way this fails is a non-marshalable result, which result()
		// already converted to an error response — this is belt and braces.
		return []byte(`{"jsonrpc":"2.0","error":{"code":-32603,"message":"encode response"}}`)
	}
	return b
}
