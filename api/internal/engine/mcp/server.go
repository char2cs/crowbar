package mcp

import (
	"context"
	"encoding/json"
)

type Server struct {
	name    string
	version string
	tools   ToolSet
}

func NewServer(name, version string, tools ToolSet) *Server {
	return &Server{name: name, version: version, tools: tools}
}

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
			Capabilities:    map[string]any{"tools": map[string]any{}},
			ServerInfo:      serverInfo{Name: s.name, Version: s.version},
		})), true

	case "ping":
		return marshal(s.result(req.ID, struct{}{})), true

	case "tools/list":
		tools := s.tools.Tools()
		if tools == nil {
			tools = []Tool{}
		}
		return marshal(s.result(req.ID, toolsListResult{Tools: tools})), true

	case "tools/call":
		var p callToolParams
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			return marshal(NewError(req.ID, CodeInvalidParams, "params must carry a tool name")), true
		}
		text, images, err := s.call(ctx, p)
		if err != nil {
			return marshal(s.result(req.ID, callToolResult{
				Content: []any{textContent{Type: "text", Text: err.Error()}},
				IsError: true,
			})), true
		}
		content := []any{textContent{Type: "text", Text: text}}
		for _, img := range images {
			content = append(content, imageContent{Type: "image", Data: img.Data, MIMEType: img.MIMEType})
		}
		return marshal(s.result(req.ID, callToolResult{Content: content})), true

	default:
		return marshal(NewError(req.ID, CodeMethodNotFound, "unsupported method "+req.Method)), true
	}
}

func (s *Server) call(ctx context.Context, p callToolParams) (string, []Image, error) {
	if rich, ok := s.tools.(ImageToolSet); ok {
		return rich.CallWithImages(ctx, p.Name, p.Arguments)
	}
	text, err := s.tools.Call(ctx, p.Name, p.Arguments)
	return text, nil, err
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
		return []byte(`{"jsonrpc":"2.0","error":{"code":-32603,"message":"encode response"}}`)
	}
	return b
}
