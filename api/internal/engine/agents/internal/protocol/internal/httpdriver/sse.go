package httpdriver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type sseEvent struct {
	id, name, data string
}

// readEvents parses a Server-Sent-Events body, calling emit per dispatched event
// (and with the id seen, even when an event carries no data) until emit says stop.
func readEvents(r io.Reader, emit func(sseEvent) bool) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxResponse)
	var ev sseEvent
	var data []string
	dispatch := func() bool {
		defer func() { ev.name, data = "", nil }()
		if len(data) == 0 {
			return true
		}
		ev.data = strings.Join(data, "\n")
		if ev.name == "" {
			ev.name = "message"
		}
		return emit(ev)
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if !dispatch() {
				return nil
			}
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch {
		case line[0] == ':':
		case field == "event":
			ev.name = value
		case field == "data":
			data = append(data, value)
		case field == "id" && !strings.ContainsRune(value, 0):
			ev.id = value
		}
	}
	return sc.Err()
}

// openStream starts the stream goroutine once StreamPath has every value it needs.
func (c *Client) openStream() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started || c.closed || c.cfg.StreamPath == "" || missing(c.cfg.StreamPath, c.captured) != "" {
		return
	}
	target, err := c.expandPath(c.cfg.StreamPath, c.captured)
	if err != nil {
		return
	}
	c.started = true
	c.wg.Add(1)
	go c.stream(target)
}

// stream reconnects with Last-Event-ID until MaxReconnects consecutive failures.
func (c *Client) stream(target string) {
	defer c.wg.Done()
	defer c.end.Do(func() { close(c.frames) })
	var lastID string
	for failures := 0; ; {
		got, err := c.connect(target, &lastID)
		if c.ctx.Err() != nil {
			return
		}
		if got {
			failures = 0
		}
		failures++
		if failures > c.cfg.MaxReconnects {
			c.push(Frame{Err: fmt.Errorf("httpdriver: stream: gave up after %d reconnects: %w", failures-1, err)})
			return
		}
		wait, stop := context.WithTimeout(c.ctx, c.cfg.Backoff(failures))
		<-wait.Done()
		stop()
	}
}

// connect reads one stream connection; got reports whether it was accepted.
func (c *Client) connect(target string, lastID *string) (got bool, err error) {
	req, err := http.NewRequestWithContext(c.ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if *lastID != "" {
		req.Header.Set("Last-Event-ID", *lastID)
	}
	c.authorize(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("connect: %w", err)
	}
	defer closeBody(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("status %d", resp.StatusCode)
	}
	err = readEvents(resp.Body, func(ev sseEvent) bool {
		if ev.id != "" {
			*lastID = ev.id
		}
		return c.push(toFrame(ev))
	})
	return true, err
}

func toFrame(ev sseEvent) Frame {
	if !json.Valid([]byte(ev.data)) {
		return Frame{Method: ev.name, Err: fmt.Errorf("httpdriver: stream: event %q has non-JSON data", ev.name)}
	}
	return Frame{Method: ev.name, Params: json.RawMessage(ev.data)}
}

func (c *Client) push(f Frame) bool {
	select {
	case c.frames <- f:
		return true
	case <-c.ctx.Done():
		return false
	}
}
