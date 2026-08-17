package exec

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/env"
)

// Executable resolves a provider command name against the environment the CHILD
// will run with.
//
// Go resolves argv[0] against the PARENT's PATH before applying cmd.Env, so a
// launchd-started daemon whose own PATH is minimal would fail to find a provider
// it is about to hand a repaired login PATH to. Searching the effective child
// PATH explicitly closes that gap; binpath.Resolve is the final fallback and also
// probes ~/.local/bin and Homebrew, which is where these CLIs actually install.
func Executable(name string, environ []string) string {
	if strings.ContainsRune(name, filepath.Separator) {
		return name
	}
	if path, ok := env.Lookup(environ, "PATH"); ok {
		for _, dir := range filepath.SplitList(path) {
			candidate := filepath.Join(dir, name)
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return candidate
			}
		}
	}
	return binpath.Resolve(name)
}
