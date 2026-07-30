package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func git(
	dir string,
	args ...string,
) error {
	_, err := gitOutput(dir, args...)
	return err
}

// gitOutput runs git in dir and returns its stdout. Stderr is folded into the
// error because git says why it refused there, and a seed that fails silently
// on "not a git repository" is worse than no seed at all.
func gitOutput(
	dir string,
	args ...string,
) (string, error) {
	cmd := exec.Command("git", args...) //nolint:gosec // G204: args are seed-authored literals, never user input
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("seed: git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

func writeSources(
	dir string,
	files []sourceFile,
) error {
	for _, f := range files {
		if err := writeSource(dir, f); err != nil {
			return err
		}
	}
	return nil
}

func writeSource(
	dir string,
	file sourceFile,
) error {
	target := filepath.Join(dir, file.path)
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return fmt.Errorf("seed: create %s: %w", filepath.Dir(file.path), err)
	}
	if err := os.WriteFile(target, []byte(file.content), 0o644); err != nil { //nolint:gosec // G306: throwaway fixture sources, not secrets
		return fmt.Errorf("seed: write %s: %w", file.path, err)
	}
	return nil
}
