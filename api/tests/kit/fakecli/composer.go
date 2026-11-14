//go:build integration

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/rivo/uniseg"
)

const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
	// composerRow is the screen row the input box lives on.
	composerRow   = 3
	defaultPrefix = "> "
)

// composer is a TUI input box: bracketed paste on, a prefixed row, a draft, and a submit key.
type composer struct {
	out         io.Writer
	submitted   string // file one JSON string per submitted message is appended to
	prefix      string
	ghost       string
	busy        bool
	draft       string
	inPaste     bool
	unprocessed string // input held back because it may be the start of a paste marker
}

func composerFromEnv(out io.Writer, dir, seg string) *composer {
	prefix := os.Getenv("FAKECLI_COMPOSER_PREFIX")
	if prefix == "" {
		prefix = defaultPrefix
	}
	return &composer{
		out:       out,
		submitted: filepath.Join(dir, seg+".submitted"),
		prefix:    prefix,
		ghost:     os.Getenv("FAKECLI_COMPOSER_GHOST"),
		busy:      os.Getenv("FAKECLI_COMPOSER_BUSY") != "",
		draft:     os.Getenv("FAKECLI_COMPOSER_DRAFT"),
	}
}

// composeUntilStopped runs the composer on stdin and returns the exit code like echoUntilStopped.
func composeUntilStopped(stop <-chan os.Signal, c *composer) int {
	// A PTY in cooked mode would hold input until a newline and echo it; a TUI reads raw.
	raw := exec.CommandContext(context.Background(), "stty", "raw", "-echo")
	raw.Stdin = os.Stdin
	if err := raw.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "fakecli: composer needs a terminal: %v\n", err)
		return 2
	}
	if err := c.start(); err != nil {
		return 2
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 && c.feed(buf[:n]) != nil {
				return
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-done:
		return 0
	case <-stop:
		return exitStopped
	}
}

func (c *composer) start() error {
	if _, err := io.WriteString(c.out, "\x1b[?2004h"); err != nil {
		return err
	}
	return c.paint()
}

// paint redraws the composer row with the cursor right after the draft. A ghost suggestion is
// painted after the cursor and the cursor is moved back before it.
func (c *composer) paint() error {
	if c.busy {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\x1b[%d;1H\x1b[J%s%s", composerRow, c.prefix, strings.ReplaceAll(c.draft, "\n", "\r\n"))
	if c.ghost != "" && c.draft == "" {
		fmt.Fprintf(&b, "\x1b[2m%s\x1b[22m\x1b[%d;%dH", c.ghost, composerRow, uniseg.StringWidth(c.prefix)+1)
	}
	_, err := io.WriteString(c.out, b.String())
	return err
}

// feed consumes input as a byte stream: a paste is taken verbatim, a carriage return outside
// one submits, anything else is typed. Chunk boundaries are invisible.
func (c *composer) feed(chunk []byte) error {
	c.unprocessed += string(chunk)
	for c.unprocessed != "" {
		var more bool
		var err error
		if c.inPaste {
			more, err = c.stepPaste()
		} else {
			more, err = c.stepTyped()
		}
		if err != nil || !more {
			return err
		}
	}
	return nil
}

func (c *composer) stepPaste() (bool, error) {
	if i := strings.Index(c.unprocessed, pasteEnd); i >= 0 {
		c.take(c.unprocessed[:i])
		c.unprocessed = c.unprocessed[i+len(pasteEnd):]
		c.inPaste = false
		return true, c.paint()
	}
	held := partialSuffix(c.unprocessed, pasteEnd)
	cut := len(c.unprocessed) - held
	c.take(c.unprocessed[:cut])
	c.unprocessed = c.unprocessed[cut:]
	return false, nil
}

func (c *composer) stepTyped() (bool, error) {
	switch {
	case strings.HasPrefix(c.unprocessed, pasteStart):
		c.unprocessed = c.unprocessed[len(pasteStart):]
		c.inPaste = true
		return true, nil
	case strings.HasPrefix(pasteStart, c.unprocessed):
		return false, nil
	case c.unprocessed[0] == '\r':
		c.unprocessed = c.unprocessed[1:]
		return true, c.submit()
	}
	typed := c.unprocessed[:1]
	c.unprocessed = c.unprocessed[1:]
	c.take(typed)
	if c.busy {
		return true, nil
	}
	_, err := io.WriteString(c.out, typed)
	return true, err
}

func (c *composer) take(s string) {
	if !c.busy {
		c.draft += s
	}
}

func (c *composer) submit() error {
	if c.busy {
		return nil
	}
	line, err := json.Marshal(c.draft)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(c.submitted, os.O_APPEND|os.O_CREATE|os.O_WRONLY|syscall.O_CLOEXEC, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(append(line, '\n'))
	if err := errors.Join(werr, f.Close()); err != nil {
		return err
	}
	c.draft = ""
	return c.paint()
}

// partialSuffix is the length of the longest proper suffix of s that begins marker.
func partialSuffix(s, marker string) int {
	for n := min(len(marker)-1, len(s)); n > 0; n-- {
		if strings.HasSuffix(s, marker[:n]) {
			return n
		}
	}
	return 0
}
