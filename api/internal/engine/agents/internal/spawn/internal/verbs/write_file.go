package verbs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
)

// writeFile materialises a config file under the per-spawn tmp dir.
//
// A missing `from:` source is tolerated with an empty destination rather than
// failing the spawn: the source is optional config, and refusing to start a CLI
// because an optional file is absent trades a degraded session for none at all.
func writeFile(args Args, _ *models.SpawnPlan) error {
	path := args.String("path")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("agents: write_file mkdir: %w", err)
	}
	from := args.String("from")
	if from == "" {
		return os.WriteFile(path, []byte(args.String("content")), 0o600)
	}
	src := expandHome(from)
	if _, err := os.Stat(src); err != nil {
		return os.WriteFile(path, nil, 0o600)
	}
	return copyFile(src, path)
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src) //nolint:gosec // descriptor-declared path, daemon-owned
	if err != nil {
		return fmt.Errorf("agents: copy open: %w", err)
	}
	defer func() { err = closeBoth(err, in.Close()) }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) //nolint:gosec // destination is under the per-spawn tmp dir
	if err != nil {
		return fmt.Errorf("agents: copy create: %w", err)
	}
	defer func() { err = closeBoth(err, out.Close()) }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("agents: copy: %w", err)
	}
	return nil
}

// closeBoth keeps a close failure from being swallowed while never masking the
// original error. A dropped close on the destination hides a full disk.
func closeBoth(primary, closeErr error) error {
	if primary != nil {
		return primary
	}
	if closeErr != nil {
		return fmt.Errorf("agents: close: %w", closeErr)
	}
	return nil
}
