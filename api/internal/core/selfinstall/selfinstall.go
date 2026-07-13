package selfinstall

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Install copies the running executable to <homeDir>/bin/crowbar (0755) so the
// vendor CLIs' hooks can invoke `crowbar hook ...` by absolute path. Idempotent:
// re-copies only when the destination is missing or a different size. Best-effort
// at the call site (a failure must not stop the daemon).
func Install(homeDir string) (string, error) {
	src, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("selfinstall: executable: %w", err)
	}
	binDir := filepath.Join(homeDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("selfinstall: mkdir: %w", err)
	}
	dst := filepath.Join(binDir, "crowbar")

	si, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("selfinstall: stat src: %w", err)
	}
	if di, derr := os.Stat(dst); derr == nil && di.Size() == si.Size() {
		return dst, nil // already installed, same size
	}
	if err := copyFile(src, dst); err != nil {
		return "", err
	}
	if err := os.Chmod(dst, 0o755); err != nil {
		return "", fmt.Errorf("selfinstall: chmod: %w", err)
	}
	return dst, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("selfinstall: open src: %w", err)
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("selfinstall: create: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("selfinstall: copy: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("selfinstall: close: %w", err)
	}
	return os.Rename(tmp, dst) // atomic swap
}
