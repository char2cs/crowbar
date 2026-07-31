package loopback

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Credentials is the published description of the daemon's auxiliary loopback
// TCP listener: where to reach it, and the bearer token every request to it must
// carry. It is the ON-DISK CONTRACT read by the native client and by whatever
// bootstraps an embedded webview, so the JSON field names are load-bearing and
// must not be renamed without updating both readers.
//
// Version is bumped whenever a field's meaning changes, so a client built against
// an older daemon can refuse loudly instead of misreading the file. PID lets a
// reader tell a live publication from one a crashed daemon left behind.
type Credentials struct {
	Version int    `json:"version"`
	Scheme  string `json:"scheme"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	URL     string `json:"url"`
	Token   string `json:"token"`
	PID     int    `json:"pid"`
}

// Publish writes the credentials into stateDir as loopback.json, owner-readable
// only (0600), and returns the path it wrote.
//
// The write is atomic: the token lands in a temp file created 0600 from the
// start and is then renamed over the destination, so a reader racing the daemon's
// startup sees either the whole previous publication or the whole new one, never
// a half-written token — and the secret is never momentarily world-readable, which
// an in-place O_TRUNC write onto an existing file would allow (O_CREATE does not
// re-apply the mode to a file that already exists).
func (c *Credentials) Publish(
	stateDir string,
) (string, error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return "", fmt.Errorf("loopback: state dir %s: %w", stateDir, err)
	}
	encoded, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", fmt.Errorf("loopback: encode credentials: %w", err)
	}
	path := filepath.Join(stateDir, FileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(encoded, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("loopback: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("loopback: publish %s: %w", path, err)
	}
	return path, nil
}

// String renders the credentials WITHOUT the token. It exists so that a stray
// %v/%s/%+v of this struct — in a log line, an error, a panic dump — cannot be
// the thing that leaks the daemon's bearer credential into a file every local
// process can read.
func (c Credentials) String() string {
	return fmt.Sprintf("loopback{url:%s pid:%d token:REDACTED}", c.URL, c.PID)
}

// LogValue is the slog counterpart of String: structured logging renders the
// redacted form too, rather than reflecting over the fields.
func (c Credentials) LogValue() slog.Value {
	return slog.StringValue(c.String())
}
