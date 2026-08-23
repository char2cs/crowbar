package exec

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/env"
)

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
