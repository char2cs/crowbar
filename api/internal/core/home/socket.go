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

func maxSocketPath() int {
	if runtime.GOOS == "linux" {
		return 107
	}
	return 103
}

func socketName(
	root string,
) string {
	var digest hash.Hash = fnv.New64a()
	digest.Write([]byte(root))
	return fmt.Sprintf("crowbar-%x.sock", binary.BigEndian.Uint64(digest.Sum(nil)))
}
