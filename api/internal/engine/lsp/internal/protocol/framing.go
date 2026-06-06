package protocol

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const contentLengthHeader = "Content-Length"

// WriteMessage frames payload with an LSP Content-Length header and writes it
// to w. The format is: "Content-Length: N\r\n\r\n" followed by exactly N bytes
// of payload.
func WriteMessage(
	w io.Writer,
	payload []byte,
) error {
	header := fmt.Sprintf("%s: %d\r\n\r\n", contentLengthHeader, len(payload))
	if _, err := io.WriteString(w, header); err != nil {
		return fmt.Errorf("framing: write header: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("framing: write payload: %w", err)
	}
	return nil
}

// ReadMessage reads one LSP-framed message from r. It parses headers until the
// blank line, then reads exactly Content-Length bytes and returns them.
func ReadMessage(
	r *bufio.Reader,
) ([]byte, error) {
	contentLen, err := parseHeaders(r)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, contentLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// parseHeaders reads header lines until the blank line and extracts the
// Content-Length value. It returns an error for malformed or missing headers.
func parseHeaders(
	r *bufio.Reader,
) (int, error) {
	contentLen := -1
	first := true

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if first {
				return 0, err // propagate EOF directly on the very first byte
			}
			return 0, fmt.Errorf("framing: read header: %w", err)
		}
		first = false

		line = strings.TrimRight(line, "\r\n")

		// Blank line signals end of headers.
		if line == "" {
			break
		}

		n, parseErr := parseHeaderLine(line)
		if parseErr != nil {
			return 0, parseErr
		}
		if n >= 0 {
			contentLen = n
		}
	}

	if contentLen < 0 {
		return 0, fmt.Errorf("framing: missing %s header", contentLengthHeader)
	}
	return contentLen, nil
}

// parseHeaderLine parses a single "Key: Value" header line. It returns the
// Content-Length value if the key matches, -1 for any other recognised header,
// or an error for malformed lines.
func parseHeaderLine(
	line string,
) (int, error) {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return 0, fmt.Errorf("framing: malformed header line: %q", line)
	}

	key := strings.TrimSpace(line[:idx])
	value := strings.TrimSpace(line[idx+1:])

	if !strings.EqualFold(key, contentLengthHeader) {
		return -1, nil
	}

	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("framing: invalid %s value %q: %w", contentLengthHeader, value, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("framing: negative %s: %d", contentLengthHeader, n)
	}
	return n, nil
}
