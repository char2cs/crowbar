package httpdriver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/outbound"
)

// Request is one named HTTP call. Path and every string leaf of Body may hold
// {placeholders}, filled from the call's params and earlier captures.
type Request struct {
	Method   string
	Path     string
	Body     any
	Capture  map[string]string // var -> path into the JSON response
	OKStatus []int             // empty means any 2xx
}

var placeholder = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)

// missing returns the first placeholder in tmpl that has no value.
func missing(tmpl string, values map[string]string) string {
	for _, m := range placeholder.FindAllStringSubmatch(tmpl, -1) {
		if _, ok := values[m[1]]; !ok {
			return m[1]
		}
	}
	return ""
}

// expandPath fills tmpl with path-escaped values and joins it to the base URL.
func (c *Client) expandPath(tmpl string, values map[string]string) (string, error) {
	if name := missing(tmpl, values); name != "" {
		return "", fmt.Errorf("no value for {%s}", name)
	}
	escaped := make(map[string]string, len(values))
	for k, v := range values {
		escaped[k] = url.PathEscape(v)
	}
	return strings.TrimRight(c.cfg.BaseURL, "/") + outbound.Substitute(tmpl, escaped), nil
}

func expandTree(node any, values map[string]string) any {
	switch v := node.(type) {
	case string:
		return outbound.Substitute(v, values)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			out[k] = expandTree(val, values)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, val := range v {
			out[i] = expandTree(val, values)
		}
		return out
	default:
		return v
	}
}

// paramValues flattens a JSON object's scalar members into template values.
func paramValues(raw json.RawMessage) (map[string]string, error) {
	values := map[string]string{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return values, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, fmt.Errorf("params: %w", err)
	}
	for k, v := range obj {
		switch s := v.(type) {
		case string:
			values[k] = s
		case json.Number:
			values[k] = s.String()
		case bool:
			values[k] = fmt.Sprint(s)
		}
	}
	return values, nil
}

// do runs the named request with values layered over the remembered captures.
func (c *Client) do(ctx context.Context, name string, params map[string]string) (json.RawMessage, error) {
	req, ok := c.cfg.Requests[name]
	if !ok {
		return nil, fmt.Errorf("httpdriver: unknown request %q", name)
	}
	values := c.snapshot(params)
	target, err := c.expandPath(req.Path, values)
	if err != nil {
		return nil, fmt.Errorf("httpdriver: %s: %w", name, err)
	}
	var body io.Reader
	if req.Body != nil {
		raw, err := json.Marshal(expandTree(req.Body, values))
		if err != nil {
			return nil, fmt.Errorf("httpdriver: %s: encode body: %w", name, err)
		}
		body = bytes.NewReader(raw)
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	defer context.AfterFunc(c.ctx, cancel)() //nolint:contextcheck // Close must abort calls whose own ctx is the caller's
	hreq, err := http.NewRequestWithContext(ctx, cmpMethod(req.Method), target, body)
	if err != nil {
		return nil, fmt.Errorf("httpdriver: %s: build request: %w", name, err)
	}
	if body != nil {
		hreq.Header.Set("Content-Type", "application/json")
	}
	c.authorize(hreq)
	resp, err := c.http.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("httpdriver: %s: %w", name, err)
	}
	defer closeBody(resp.Body)
	if !statusOK(req, resp.StatusCode) {
		return nil, fmt.Errorf("httpdriver: %s: status %d", name, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return nil, fmt.Errorf("httpdriver: %s: read response: %w", name, err)
	}
	if err := c.capture(name, req, raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *Client) capture(name string, req Request, raw []byte) error {
	if len(req.Capture) == 0 {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("httpdriver: %s: parse response: %w", name, err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for field, path := range req.Capture {
		if v, ok := mapping.Scalar(decoded, mapping.Paths(path)); ok {
			c.captured[field] = v
		}
	}
	return nil
}

func cmpMethod(m string) string {
	if m == "" {
		return http.MethodPost
	}
	return m
}

func statusOK(req Request, status int) bool {
	if len(req.OKStatus) == 0 {
		return status >= 200 && status < 300
	}
	return slices.Contains(req.OKStatus, status)
}

func closeBody(b io.Closer) {
	if err := b.Close(); err != nil {
		slog.Debug("httpdriver: close body", "err", err)
	}
}
