package home

import (
	"encoding/binary"
	"fmt"
	"hash"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

const socketFile = "crowbar.sock"

// Socket is where the daemon for this home listens. The default home binds inside
// itself; an overridden home binds a short name in the temp dir hashed from the
// cleaned home, so every spelling maps to one socket. A path too long for sun_path
// binds that name in the user's runtime dir, or in /tmp/crowbar-<uid>, instead.
func (h Home) Socket() string {
	path := filepath.Join(h.root, socketFile)
	if h.overridden {
		path = filepath.Join(os.TempDir(), socketName(h.root))
	}
	if len(path) <= maxSocketPath() {
		return path
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(dir) {
		if short := filepath.Join(dir, socketName(h.root)); len(short) <= maxSocketPath() {
			return short
		}
	}
	return filepath.Join("/tmp", "crowbar-"+strconv.Itoa(os.Getuid()), socketName(h.root))
}

// maxSocketPath is the longest path a unix socket binds: sun_path less its NUL,
// 108 bytes on Linux and 104 on macOS and the BSDs.
func maxSocketPath() int {
	if runtime.GOOS == "linux" {
		return 107
	}
	return 103
}

func socketName(
	root string,
) string {
	// gosec G104 exempts (hash.Hash).Write, but not the Hash64 fnv returns.
	var digest hash.Hash = fnv.New64a()
	digest.Write([]byte(root))
	return fmt.Sprintf("crowbar-%x.sock", binary.BigEndian.Uint64(digest.Sum(nil)))
}
