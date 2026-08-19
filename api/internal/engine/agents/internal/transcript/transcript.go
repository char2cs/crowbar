// Package transcript reads the provider's OWN record of a conversation.
//
// It exists because a terminating hook reports ONE message. Claude's Stop
// carries `last_assistant_message` — literally the last one — so an agent that
// speaks, works for thirty seconds and speaks again has its first message
// dropped on the floor, and every reader of Crowbar's record (the chat, a
// handoff, get_chat_log serving a sibling agent) is told a lossy version of what
// the agent said. The provider's own file is the only place the rest exists.
//
// THREE PROPERTIES ARE LOAD-BEARING, and each of them is a test:
//
//   - IT ONLY EVER READS. The file belongs to the provider — it is under
//     ~/.claude or ~/.codex — and Crowbar has no business writing there. There is
//     exactly one syscall pair in this package that touches the path: os.Open
//     (O_RDONLY) and Stat through the handle it returns. No create, no lock, no
//     temp file, no rename, no chmod, no directory entry. A read that cannot
//     happen is not retried by trying harder.
//
//   - IT NEVER LOSES THE REPLY. Every failure — a missing file, an unreadable
//     one, a half-written line, a shape nobody expected — yields NO messages and
//     no error worth failing a hook over. The caller then records exactly what it
//     recorded before this package existed. Degrading to yesterday is always
//     available; degrading below it never is.
//
//   - IT IS INCREMENTAL AND BOUNDED. A live session's transcript reaches tens of
//     thousands of entries, so a read starts where the last one stopped, consumes
//     at most MaxReadBytes, and never lands mid-line — which is also what makes it
//     safe against a file being appended to while it is being read.
package transcript

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/payload"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

const (
	// defaultMaxReadBytes bounds one read when a descriptor names no ceiling.
	//
	// It is generous rather than tight because the cost of stopping short is a
	// message recorded one drain later, while the cost of a tight bound is a turn
	// whose messages never surface at all. Measured against a real 15 MB claude
	// transcript, the longest single line was 283 KB and a whole turn's window
	// between two reads was orders of magnitude below this.
	defaultMaxReadBytes = 4 << 20

	// defaultMaxMessageBytes bounds one message. The same real transcript's
	// longest assistant message was 4.7 KB, so this is a guard against a provider
	// changing shape, not a working limit.
	defaultMaxMessageBytes = 256 << 10

	// truncationMarker is appended to a message cut at the ceiling. A reader must
	// be able to see that there was more; a silently clipped message is a quote
	// the agent never said.
	truncationMarker = "\n\n[crowbar: message truncated]"
)

// maxLineBytes bounds ONE line handed to the JSON decoder. A line longer than
// this is SKIPPED — and only that line: decoding a megabyte of JSON to find a
// message that could not be one at that size is work a hook path should not do,
// but the lines around it are ordinary and must still be read.
const maxLineBytes = 1 << 20

// Declared reports whether this provider's transcript can be read.
func Declared(d *spec.Descriptor) bool {
	return d != nil && d.Transcript.Declared()
}

// Path returns the transcript this hook payload names, and whether the provider
// declares one at all.
//
// decoded is the hook payload the engine already parsed, so nothing re-reads it
// and no caller above the engine ever learns which field a provider puts its
// transcript in.
func Path(d *spec.Descriptor, decoded map[string]any) (string, bool) {
	if !Declared(d) {
		return "", false
	}
	path := payload.String(decoded, d.Transcript.PathField)
	if path == "" {
		return "", false
	}
	return path, true
}

// End returns the transcript's current size: the position a session Crowbar has
// only just started watching begins at.
//
// Starting at the END is what stops a resumed conversation's entire history from
// being replayed into the record as though the agent had just said all of it.
// An unreadable file answers 0, which is the same as a file that does not exist
// yet — correct for a session whose CLI has not written anything.
func End(d *spec.Descriptor, path string) int64 {
	if !Declared(d) || path == "" {
		return 0
	}
	f, err := os.Open(path) //nolint:gosec // path comes from the provider's own hook payload; opened read-only
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return 0
	}
	return info.Size()
}

// Read returns the messages appended to path since from, and where to resume.
//
// A window larger than the ceiling is cut at the last complete line inside it and
// reported Truncated, so the remainder is read next time rather than dropped. A
// file SHORTER than from is a different file at the same path (a session rotated
// out from under us): it restarts at that file's end rather than replaying it,
// because replaying somebody else's history as this turn's speech is a worse
// failure than missing a message.
func Read(d *spec.Descriptor, path string, from int64) (models.TranscriptRead, error) {
	if !Declared(d) || path == "" {
		return models.TranscriptRead{Offset: from}, nil
	}
	if from < 0 {
		from = 0
	}
	f, err := os.Open(path) //nolint:gosec // path comes from the provider's own hook payload; opened read-only
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return models.TranscriptRead{Offset: from}, nil
		}
		return models.TranscriptRead{Offset: from}, fmt.Errorf("agents: open transcript: %w", err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return models.TranscriptRead{Offset: from}, fmt.Errorf("agents: stat transcript: %w", err)
	}
	size := info.Size()
	if size <= from {
		if size < from {
			return models.TranscriptRead{Offset: size}, nil
		}
		return models.TranscriptRead{Offset: from}, nil
	}

	limit := int64(d.Transcript.MaxReadBytes)
	if limit <= 0 {
		limit = defaultMaxReadBytes
	}
	window := size - from
	truncated := window > limit
	if truncated {
		window = limit
	}

	buf := make([]byte, window)
	n, err := f.ReadAt(buf, from)
	if err != nil && !errors.Is(err, io.EOF) {
		return models.TranscriptRead{Offset: from}, fmt.Errorf("agents: read transcript: %w", err)
	}
	buf = buf[:n]

	// Stop at the last complete line. Anything after it is a record still being
	// written, and leaving it for the next read is the whole of this package's
	// tolerance for a file that grows while it is open.
	consumed := int64(bytes.LastIndexByte(buf, '\n') + 1)
	if consumed == 0 {
		if !truncated {
			// A partial line at the end of the file: nothing to do but wait for it.
			return models.TranscriptRead{Offset: from}, nil
		}
		// A single line longer than one read is allowed to be. Consuming the window
		// leaves the next read starting mid-line, which decodes to nothing and is
		// dropped — bounded, and it can never wedge the cursor.
		return models.TranscriptRead{Offset: from + int64(len(buf)), Truncated: true}, nil
	}

	return models.TranscriptRead{
		Messages:  messages(d, buf[:consumed]),
		Offset:    from + consumed,
		Truncated: truncated,
	}, nil
}

// messages decodes whole lines and keeps the ones the descriptor calls a message.
//
// Every per-line failure is silent and LOCAL, and the locality is the point: an
// entry that does not decode, decodes to something other than an object, or is
// simply too long to be worth buffering costs that line and nothing around it. A
// provider's file is not a contract, and the offset has already moved past this
// window by the time we get here — so a bad line that could stop the loop would
// take every message after it with it, permanently.
//
// That is why the lines are cut by hand rather than by bufio.Scanner. A Scanner
// hits ErrTooLong on an oversized token and then returns false FOR GOOD, which
// silently discards the rest of the window; splitting on the newline ourselves
// cannot end early.
func messages(d *spec.Descriptor, data []byte) []models.TranscriptMessage {
	var out []models.TranscriptMessage
	for len(data) > 0 {
		line := data
		if end := bytes.IndexByte(data, '\n'); end >= 0 {
			line, data = data[:end], data[end+1:]
		} else {
			data = nil
		}
		if len(line) > maxLineBytes {
			continue
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		msg, ok := message(d, entry)
		if !ok {
			continue
		}
		out = append(out, msg)
	}
	return out
}

// message applies the descriptor's recognition rules to one decoded entry.
func message(d *spec.Descriptor, entry map[string]any) (models.TranscriptMessage, bool) {
	m := d.Transcript.Message
	if !matches(entry, m.Match) || rejects(entry, m.Reject) {
		return models.TranscriptMessage{}, false
	}

	text := entryText(entry, m)
	if text == "" {
		return models.TranscriptMessage{}, false
	}

	limit := d.Transcript.MaxMessageBytes
	if limit <= 0 {
		limit = defaultMaxMessageBytes
	}
	truncated := false
	if len(text) > limit {
		text = safeCut(text, limit) + truncationMarker
		truncated = true
	}

	at, _ := payload.Time(entry, m.Timestamp)
	return models.TranscriptMessage{Text: text, At: at, Truncated: truncated}, true
}

// entryText assembles one message's words: either the joined text of the blocks
// the descriptor selects, or a single declared leaf for a provider that does not
// split a message up.
func entryText(entry map[string]any, m spec.TranscriptMessageSpec) string {
	if m.Blocks == "" {
		return strings.TrimSpace(payload.String(entry, m.Text))
	}
	var parts []string
	for _, block := range payload.Objects(entry, m.Blocks) {
		if !matches(block, m.BlockMatch) {
			continue
		}
		part := strings.TrimSpace(payload.String(block, m.BlockText))
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, m.Join)
}

// matches reports whether every declared pair holds. An empty rule set matches
// everything, which is why Declared insists on at least one Match pair: a
// descriptor that named none would call every line in the file a message.
func matches(obj map[string]any, want map[string]string) bool {
	for path, expected := range want {
		got, ok := payload.Scalar(obj, path)
		if !ok || got != expected {
			return false
		}
	}
	return true
}

// rejects reports whether any declared pair holds — the disqualifying half.
func rejects(obj map[string]any, deny map[string]string) bool {
	for path, expected := range deny {
		if got, ok := payload.Scalar(obj, path); ok && got == expected {
			return true
		}
	}
	return false
}

// safeCut trims to at most limit bytes without splitting a rune, so a truncated
// message is still valid UTF-8 rather than ending in a replacement character.
func safeCut(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8Start(s[cut]) {
		cut--
	}
	return s[:cut]
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
