package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/char2cs/crowbar/api/internal/app/browserbridge"
)

const (
	pointerDataPrefix = "⌖data "
	maxPointerInput   = 64 << 10
	maxPointerItems   = 10
	maxPointerLine    = maxPointerInput
)

var pointerDetails = []string{"refs", "text", "html", "styles"}

const browserResolveDescription = "Lines starting with ⌖ are browser pointers the user pasted: they name page elements. " +
	"Resolve them with this tool before acting. Pass the pasted text (one or several pointers); " +
	"returns per element a fresh ref (usable with browser_click/browser_type) or 'gone', and optionally its text, html or computed styles."

const browserResolveSchema = `{"type":"object","properties":{` +
	`"pointer":{"type":"string","description":"The pasted pointer text, including its ⌖data line(s)."},` +
	`"detail":{"type":"string","enum":["refs","text","html","styles"],"description":"What to return besides the ref; default refs."}` +
	`},"required":["pointer"],"additionalProperties":false}`

// pointer is one decoded ⌖data line. Items stay raw: the page side owns their
// shape and gets them back verbatim.
type pointer struct {
	V     int               `json:"v"`
	Tab   string            `json:"tab"`
	URL   string            `json:"url"`
	Title string            `json:"title"`
	Kind  string            `json:"kind"`
	Items []json.RawMessage `json:"items"`
	More  int               `json:"more"`
	Quote string            `json:"quote"`
}

type pointerItem struct {
	Tag  string `json:"tag"`
	Role string `json:"role"`
	Name string `json:"name"`
	Text string `json:"text"`
}

// label is the short human name of an item, for echoing the gist.
func (it pointerItem) label() string {
	name := it.Name
	if name == "" {
		name = it.Text
	}
	name = truncateRunes(name, 60)
	if name == "" {
		return it.Tag
	}
	return fmt.Sprintf("%s %q", it.Tag, name)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

type resolvedItem struct {
	Found     bool            `json:"found"`
	Ref       string          `json:"ref"`
	MatchedBy string          `json:"matchedBy"`
	Text      string          `json:"text"`
	HTML      string          `json:"html"`
	Styles    json.RawMessage `json:"styles"`
}

func parsePointers(text string) ([]pointer, error) {
	if len(text) > maxPointerInput {
		return nil, fmt.Errorf("agenttools: browser_resolve: pointer text is %d bytes; the limit is %d", len(text), maxPointerInput)
	}
	var out []pointer
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 4096), maxPointerLine+1)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		payload, ok := strings.CutPrefix(line, pointerDataPrefix)
		if !ok {
			continue
		}
		p, err := parsePointer(payload, len(out)+1)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("agenttools: browser_resolve: read pointer text: %w", err)
	}
	if len(out) == 0 {
		return nil, errors.New("agenttools: browser_resolve: no '⌖data {...}' line found in the pointer text; pass the pasted pointer in full")
	}
	return out, nil
}

func parsePointer(payload string, n int) (pointer, error) {
	var p pointer
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return pointer{}, fmt.Errorf("agenttools: browser_resolve: pointer %d: ⌖data is not valid JSON: %w", n, err)
	}
	switch {
	case p.V != 1:
		return pointer{}, fmt.Errorf("agenttools: browser_resolve: pointer %d: unsupported pointer version %d (expected 1)", n, p.V)
	case p.Tab == "":
		return pointer{}, fmt.Errorf("agenttools: browser_resolve: pointer %d: no tab id", n)
	case len(p.Items) == 0:
		return pointer{}, fmt.Errorf("agenttools: browser_resolve: pointer %d: no items", n)
	case len(p.Items) > maxPointerItems:
		return pointer{}, fmt.Errorf("agenttools: browser_resolve: pointer %d: %d items; the limit is %d", n, len(p.Items), maxPointerItems)
	}
	return p, nil
}

func browserResolveDef(b BrowserBridge) toolDef {
	return toolDef{
		name:        "browser_resolve",
		description: browserResolveDescription,
		schema:      json.RawMessage(browserResolveSchema),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			var in struct {
				Pointer string `json:"pointer"`
				Detail  string `json:"detail"`
			}
			dec := json.NewDecoder(strings.NewReader(string(args)))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&in); err != nil {
				return "", fmt.Errorf("agenttools: browser_resolve: arguments are not valid: %w", err)
			}
			if in.Detail == "" {
				in.Detail = "refs"
			}
			if !slices.Contains(pointerDetails, in.Detail) {
				return "", fmt.Errorf("agenttools: browser_resolve: detail must be one of %s", strings.Join(pointerDetails, ", "))
			}
			ptrs, err := parsePointers(in.Pointer)
			if err != nil {
				return "", err
			}
			parts := make([]string, 0, len(ptrs))
			for i, p := range ptrs {
				text, err := resolvePointer(ctx, b, c, p, in.Detail)
				if err != nil {
					return "", err
				}
				parts = append(parts, fmt.Sprintf("pointer %d: %s", i+1, text))
			}
			return strings.Join(parts, "\n\n"), nil
		},
	}
}

func resolvePointer(ctx context.Context, b BrowserBridge, c Caller, p pointer, detail string) (string, error) {
	var head strings.Builder
	head.WriteString(pointerSummary(p))
	forwarded, err := json.Marshal(map[string]any{"items": p.Items, "detail": detail})
	if err != nil {
		return "", fmt.Errorf("agenttools: browser_resolve: %w", err)
	}
	raw, err := b.Do(ctx, c.ChatID, "resolve", p.Tab, forwarded)
	if errors.Is(err, browserbridge.ErrUnknownTab) {
		return fmt.Sprintf("%s\n%s\nit pointed at: %s", head.String(), browserbridge.ErrUnknownTab.Error(), itemLabels(p)), nil
	}
	if err != nil {
		return "", fmt.Errorf("agenttools: browser_resolve: %w", err)
	}
	var res struct {
		Items []resolvedItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", fmt.Errorf("agenttools: browser_resolve: the desktop app returned an unreadable answer: %w", err)
	}
	if cur, ok := currentTabURL(b.Tabs(c.ChatID), p.Tab); ok && cur != p.URL {
		fmt.Fprintf(&head, "\nnote: the tab has navigated since the pick; now at %s", cur)
	}
	return head.String() + "\n" + renderResolved(p, res.Items, detail), nil
}

func pointerSummary(p pointer) string {
	s := fmt.Sprintf("%s · %s · tab %s", p.Kind, p.URL, p.Tab)
	if p.Title != "" {
		s += fmt.Sprintf(" · %q", truncateRunes(p.Title, 60))
	}
	if p.Quote != "" {
		s += fmt.Sprintf("\nquote: %q", truncateRunes(p.Quote, 200))
	}
	return s
}

func itemLabels(p pointer) string {
	labels := make([]string, 0, len(p.Items))
	for _, raw := range p.Items {
		var it pointerItem
		_ = json.Unmarshal(raw, &it) // a malformed item only loses its label
		labels = append(labels, it.label())
	}
	return strings.Join(labels, "; ")
}

func currentTabURL(tabs json.RawMessage, id string) (string, bool) {
	var list []struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if json.Unmarshal(tabs, &list) != nil {
		return "", false
	}
	for _, t := range list {
		if t.ID == id {
			return t.URL, true
		}
	}
	return "", false
}

func renderResolved(p pointer, got []resolvedItem, detail string) string {
	lines := make([]string, 0, len(p.Items)+1)
	for i, raw := range p.Items {
		var it pointerItem
		_ = json.Unmarshal(raw, &it) // a malformed item only loses its label
		line := fmt.Sprintf("%d %s: ", i+1, it.label())
		switch {
		case i >= len(got):
			line += "no answer"
		case !got[i].Found:
			line += "gone"
		default:
			line += foundText(got[i], detail)
		}
		lines = append(lines, line)
	}
	if p.More > 0 {
		lines = append(lines, fmt.Sprintf("(+%d more items were picked but not included)", p.More))
	}
	return strings.Join(lines, "\n")
}

func foundText(r resolvedItem, detail string) string {
	how := r.MatchedBy
	if how == "" {
		how = "selector"
	}
	s := fmt.Sprintf("found %s (%s)", r.Ref, how)
	switch detail {
	case "text":
		s += "\n  text: " + r.Text
	case "html":
		s += "\n  html: " + r.HTML
	case "styles":
		if len(r.Styles) > 0 {
			s += "\n  styles: " + string(r.Styles)
		}
	}
	return s
}
