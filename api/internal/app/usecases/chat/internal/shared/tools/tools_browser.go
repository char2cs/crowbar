package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/mcp"
)

const (
	defaultWaitMs = 10_000
	maxWaitMs     = 30_000
)

// BrowserBridge is the daemon's route to the desktop app's browser tabs. Both
// methods take the CALLER'S chat id, so what an agent can touch is decided
// by who it is, never by an argument it supplies.
type BrowserBridge interface {
	// Tabs lists, as raw JSON, only the tabs that belong to chatID.
	Tabs(chatID string) json.RawMessage
	// Do runs op against a tab of chatID (or, with no tab, against the window).
	Do(ctx context.Context, chatID, op, tabID string, args json.RawMessage) (json.RawMessage, error)
}

type browserSpec struct {
	name, op, description string
	props                 string // JSON object members, besides the shared "tab"
	required              []string
	noTab                 bool // the op is not addressed to an existing tab
}

var browserSpecs = []browserSpec{
	{
		name: "browser_open", op: "open", noTab: true,
		description: "Open the built-in browser in this chat's pane, optionally at a URL. Reuses this chat's browser tab unless newTab is true. Returns the tab id.",
		props:       `"url":{"type":"string","description":"http(s) URL to load."},"newTab":{"type":"boolean","description":"Open an additional tab instead of reusing one."}`,
	},
	{
		name: "browser_navigate", op: "navigate", required: []string{"url"},
		description: "Load a URL in a browser tab.",
		props:       `"url":{"type":"string"}`,
	},
	{name: "browser_back", op: "back", description: "Go back in a browser tab's history."},
	{name: "browser_forward", op: "forward", description: "Go forward in a browser tab's history."},
	{name: "browser_reload", op: "reload", description: "Reload a browser tab."},
	{
		name: "browser_snapshot", op: "snapshot",
		description: "Read the page as an accessibility-style text tree. Every actionable element carries a ref like [e12] that browser_click, browser_type and browser_scroll accept. Refs are only valid for the latest snapshot; the output is truncated with a marker when large.",
	},
	{
		name: "browser_click", op: "click", required: []string{"ref"},
		description: "Click the element with this ref from the latest browser_snapshot.",
		props:       `"ref":{"type":"string"}`,
	},
	{
		name: "browser_type", op: "type", required: []string{"ref", "text"},
		description: "Type text into the field with this ref, replacing its value; submit presses Enter afterwards.",
		props:       `"ref":{"type":"string"},"text":{"type":"string"},"submit":{"type":"boolean"}`,
	},
	{
		name: "browser_press_key", op: "press_key", required: []string{"key"},
		description: "Press a key (e.g. Enter, Escape, Tab, ArrowDown) on the focused element.",
		props:       `"key":{"type":"string"}`,
	},
	{
		name: "browser_scroll", op: "scroll",
		description: "Scroll the page, or bring the element with ref into view.",
		props:       `"ref":{"type":"string"},"direction":{"type":"string","enum":["up","down","top","bottom"]}`,
	},
	{
		name: "browser_wait", op: "wait", required: []string{"condition"},
		description: "Wait until the page has loaded, a CSS selector exists, or text appears. Fails after timeoutMs (default 10000, max 30000).",
		props:       `"condition":{"type":"string","enum":["load","selector","text"]},"value":{"type":"string","description":"The selector or text; unused for load."},"timeoutMs":{"type":"integer"}`,
	},
	{
		name: "browser_eval", op: "eval", required: []string{"js"},
		description: "Run JavaScript in the page and return its JSON-serializable result. The code is the body of an async function: use return to give a value back.",
		props:       `"js":{"type":"string"}`,
	},
	{name: "browser_close", op: "close", description: "Close a browser tab."},
}

func browserTools(deps Deps) []toolDef {
	if deps.Browser == nil {
		return nil
	}
	defs := []toolDef{browserTabsDef(deps.Browser), browserScreenshotDef(deps.Browser)}
	for _, spec := range browserSpecs {
		defs = append(defs, spec.def(deps.Browser))
	}
	return defs
}

func (s browserSpec) schema() json.RawMessage {
	props := s.props
	if !s.noTab {
		tab := `"tab":{"type":"string","description":"Tab id from browser_tabs; optional when this chat has exactly one tab."}`
		props = strings.TrimPrefix(tab+","+props, ",")
		props = strings.TrimSuffix(props, ",")
	}
	req, _ := json.Marshal(append([]string{}, s.required...))
	return json.RawMessage(fmt.Sprintf(
		`{"type":"object","properties":{%s},"required":%s,"additionalProperties":false}`, props, req))
}

// rejectUnknown enforces the schema's additionalProperties:false, so nothing
// the model invents (a chat id, say) is ever forwarded to the desktop.
func (s browserSpec) rejectUnknown(fields map[string]json.RawMessage) error {
	var known map[string]json.RawMessage
	if err := json.Unmarshal(s.schema(), &struct {
		Properties *map[string]json.RawMessage `json:"properties"`
	}{&known}); err != nil {
		return fmt.Errorf("agenttools: %s: %w", s.name, err)
	}
	for name := range fields {
		if _, ok := known[name]; !ok {
			return fmt.Errorf("agenttools: %s: unknown argument %q", s.name, name)
		}
	}
	return nil
}

func (s browserSpec) def(b BrowserBridge) toolDef {
	return toolDef{
		name: s.name, description: s.description, schema: s.schema(),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			raw, err := runBrowserOp(ctx, b, c, s, args)
			if err != nil {
				return "", err
			}
			return browserText(raw), nil
		},
	}
}

func runBrowserOp(
	ctx context.Context,
	b BrowserBridge,
	c Caller,
	s browserSpec,
	args json.RawMessage,
) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if err := decode(args, &fields); err != nil {
		return nil, err
	}
	if err := s.rejectUnknown(fields); err != nil {
		return nil, err
	}
	for _, name := range s.required {
		if v, ok := fields[name]; !ok || string(v) == "null" || string(v) == `""` {
			return nil, fmt.Errorf("agenttools: %s: %s is required", s.name, name)
		}
	}
	tabID := ""
	if !s.noTab {
		var err error
		if tabID, err = pickTab(b, c.ChatID, fields); err != nil {
			return nil, fmt.Errorf("agenttools: %s: %w", s.name, err)
		}
	}
	delete(fields, "tab")
	if s.op == "wait" {
		fields["timeoutMs"] = clampWait(fields["timeoutMs"])
	}
	forwarded, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("agenttools: %s: %w", s.name, err)
	}
	raw, err := b.Do(ctx, c.ChatID, s.op, tabID, forwarded)
	if err != nil {
		return nil, fmt.Errorf("agenttools: %s: %w", s.name, err)
	}
	return raw, nil
}

// pickTab resolves the tab an agent addressed. The id is still checked against
// the caller's chat by the bridge; this only supplies the default when the
// agent named none.
func pickTab(b BrowserBridge, chatID string, fields map[string]json.RawMessage) (string, error) {
	var named string
	if raw, ok := fields["tab"]; ok {
		if err := json.Unmarshal(raw, &named); err != nil {
			return "", fmt.Errorf("tab must be a string")
		}
	}
	if named != "" {
		return named, nil
	}
	ids := tabIDs(b.Tabs(chatID))
	switch len(ids) {
	case 0:
		return "", fmt.Errorf("this chat has no browser tab; call browser_open first")
	case 1:
		return ids[0], nil
	}
	return "", fmt.Errorf("this chat has several tabs (%s); pass tab", strings.Join(ids, ", "))
}

func tabIDs(raw json.RawMessage) []string {
	var tabs []struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &tabs) != nil {
		return nil
	}
	ids := make([]string, 0, len(tabs))
	for _, t := range tabs {
		ids = append(ids, t.ID)
	}
	return ids
}

func clampWait(raw json.RawMessage) json.RawMessage {
	var ms int
	if json.Unmarshal(raw, &ms) != nil || ms <= 0 {
		ms = defaultWaitMs
	}
	ms = min(ms, maxWaitMs)
	out, _ := json.Marshal(ms)
	return out
}

// browserText shows a peer's answer to the model: a JSON string as itself,
// anything else as compact JSON.
func browserText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	if len(raw) == 0 {
		return "ok"
	}
	return string(raw)
}

func browserTabsDef(b BrowserBridge) toolDef {
	return toolDef{
		name:        "browser_tabs",
		description: "List this chat's browser tabs: id, url, title, loading.",
		schema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		run: func(_ context.Context, c Caller, _ json.RawMessage) (string, error) {
			return string(b.Tabs(c.ChatID)), nil
		},
	}
}

func browserScreenshotDef(b BrowserBridge) toolDef {
	spec := browserSpec{name: "browser_screenshot", op: "screenshot"}
	return toolDef{
		name:        spec.name,
		description: "Take a PNG screenshot of a browser tab's visible viewport.",
		schema:      spec.schema(),
		runImage: func(ctx context.Context, c Caller, args json.RawMessage) (string, []mcp.Image, error) {
			raw, err := runBrowserOp(ctx, b, c, spec, args)
			if err != nil {
				return "", nil, err
			}
			var shot struct {
				PNG    string `json:"png"`
				Width  int    `json:"width"`
				Height int    `json:"height"`
			}
			if err := json.Unmarshal(raw, &shot); err != nil || shot.PNG == "" {
				return "", nil, fmt.Errorf("agenttools: browser_screenshot: the desktop app returned no image")
			}
			text := fmt.Sprintf("Screenshot %dx%d", shot.Width, shot.Height)
			return text, []mcp.Image{{MIMEType: "image/png", Data: shot.PNG}}, nil
		},
	}
}
